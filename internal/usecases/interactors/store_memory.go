// StoreMemory use case
package interactors

import (
	"context"
	"fmt"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
)

// StoreMemoryInput contains the input for storing a memory
type StoreMemoryInput struct {
	Content string
	Wing    string
	Room    *string
	Type    *valueobjects.MemoryType
}

// StoreMemoryOutput contains the output of storing a memory
type StoreMemoryOutput struct {
	FingerprintID string
	Type          string
	FactCount     int
	TokenCount    int
	ModelHash     string
}

// StoreMemory implements the store memory use case
type StoreMemory struct {
	repository       ports.Repository
	extractor        ports.FingerprintExtractor
	causalDetector   ports.CausalRelationDetector
	vectorStore      ports.VectorStore
	metricsCollector ports.MetricsCollector
	logger           ports.Logger
}

// NewStoreMemory creates a new store memory interactor
func NewStoreMemory(
	repository ports.Repository,
	extractor ports.FingerprintExtractor,
	causalDetector ports.CausalRelationDetector,
	vectorStore ports.VectorStore,
	metricsCollector ports.MetricsCollector,
	logger ports.Logger,
) *StoreMemory {
	return &StoreMemory{
		repository:       repository,
		extractor:        extractor,
		causalDetector:   causalDetector,
		vectorStore:      vectorStore,
		metricsCollector: metricsCollector,
		logger:           logger,
	}
}

// Execute stores a memory with full extraction (atomic transaction)
func (uc *StoreMemory) Execute(ctx context.Context, input StoreMemoryInput) (*StoreMemoryOutput, error) {
	start := time.Now()

	// 1. Create verbatim
	verbatim := entities.NewVerbatim(input.Content, input.Wing, input.Room)

	// 2. Extract T1 and T2
	fp, emb, err := uc.extractor.ExtractPipeline(ctx, verbatim, input.Type)
	if err != nil {
		return nil, fmt.Errorf("extraction failed: %w", err)
	}

	// 3. Atomic transaction for T0, T1, T2 storage
	tx, err := uc.repository.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Handle nil tx (for testing) - use non-tx methods
	if tx == nil {
		if err := uc.repository.StoreVerbatim(ctx, verbatim); err != nil {
			return nil, fmt.Errorf("failed to store verbatim: %w", err)
		}
		if err := uc.repository.StoreFingerprint(ctx, fp); err != nil {
			return nil, fmt.Errorf("failed to store fingerprint: %w", err)
		}
		if err := uc.repository.StoreEmbedding(ctx, emb); err != nil {
			return nil, fmt.Errorf("failed to store embedding: %w", err)
		}
	} else {
		// Store T0
		if err := uc.repository.StoreVerbatimTx(ctx, tx, verbatim); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to store verbatim: %w", err)
		}

		// Store T1
		if err := uc.repository.StoreFingerprintTx(ctx, tx, fp); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to store fingerprint: %w", err)
		}

		// Store T2
		if err := uc.repository.StoreEmbeddingTx(ctx, tx, emb); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to store embedding: %w", err)
		}

		// Commit transaction
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("failed to commit transaction: %w", err)
		}
	}

	// 4. Add to vector store (non-fatal)
	candidate := entities.NewCandidate(fp, verbatim, emb.Vector)
	if err := uc.vectorStore.AddCandidate(ctx, candidate); err != nil {
		// Non-fatal: continue with SQLite only
		if uc.logger != nil {
			uc.logger.Warn("Failed to add candidate to vector store, continuing with SQLite only",
				"error", err,
				"fingerprint_id", fp.ID.String(),
			)
		}
	}

	// 5. Create causal node (non-fatal)
	node := entities.NewCausalNode(fp.ID, string(fp.Type), fp.Data.Subject[0], input.Wing, input.Room)
	if err := uc.repository.AddNode(ctx, node); err != nil {
		// Non-fatal: continue without causal node
		if uc.logger != nil {
			uc.logger.Warn("Failed to create causal node, continuing without",
				"error", err,
				"fingerprint_id", fp.ID.String(),
			)
		}
	}

	// 6. Detect causal relations (non-fatal)
	recentFps, err := uc.repository.GetRecentFingerprintsByWing(ctx, input.Wing, fp.ID, 50)
	if err != nil {
		if uc.logger != nil {
			uc.logger.Warn("Failed to get recent fingerprints for causal detection",
				"error", err,
				"wing", input.Wing,
			)
		}
	} else if len(recentFps) > 0 && uc.causalDetector != nil {
		edges, err := uc.causalDetector.DetectCausalRelations(ctx, fp, recentFps, input.Content)
		if err != nil {
			if uc.logger != nil {
				uc.logger.Warn("Failed to detect causal relations",
					"error", err,
					"fingerprint_id", fp.ID.String(),
				)
			}
		} else {
			for _, edge := range edges {
				if err := uc.repository.AddEdge(ctx, edge); err != nil {
					if uc.logger != nil {
						uc.logger.Warn("Failed to add causal edge",
							"error", err,
							"from_id", edge.FromID.String(),
							"to_id", edge.ToID.String(),
							"relation", string(edge.Relation),
						)
					}
				}
			}
		}
	}

	// Record metrics if collector is available
	if uc.metricsCollector != nil {
		uc.metricsCollector.RecordStore(time.Since(start))
	}

	return &StoreMemoryOutput{
		FingerprintID: fp.ID.String(),
		Type:          string(fp.Type),
		FactCount:     fp.FactCount,
		TokenCount:    verbatim.TokenCount,
		ModelHash:     fp.ModelHash,
	}, nil
}
