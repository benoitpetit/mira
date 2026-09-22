// UpdateMemory use case
package interactors

import (
	"context"
	"fmt"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// UpdateMemoryInput contains the input for updating a memory
type UpdateMemoryInput struct {
	ID      uuid.UUID
	Content string
}

// UpdateMemoryOutput contains the output of updating a memory
type UpdateMemoryOutput struct {
	Verbatim *entities.Verbatim
}

// UpdateMemory implements the update memory use case
type UpdateMemory struct {
	repo           ports.Repository
	extractor      ports.FingerprintExtractor
	vectorStore    ports.VectorStore
	causalDetector ports.CausalRelationDetector
}

// WithCausalDetector enables causal relation rebuilding after updates while
// keeping the constructor compatible with lightweight embedders and tests.
func (uc *UpdateMemory) WithCausalDetector(detector ports.CausalRelationDetector) *UpdateMemory {
	uc.causalDetector = detector
	return uc
}

// NewUpdateMemory creates a new update memory interactor
func NewUpdateMemory(repo ports.Repository, extractor ports.FingerprintExtractor, vectorStore ports.VectorStore) *UpdateMemory {
	return &UpdateMemory{
		repo:        repo,
		extractor:   extractor,
		vectorStore: vectorStore,
	}
}

// Execute updates a verbatim's content and regenerates its fingerprint and embedding.
// The delete and re-insert happen inside a single transaction so that a storage
// failure during re-insert leaves the original verbatim intact (no data loss).
func (uc *UpdateMemory) Execute(ctx context.Context, input UpdateMemoryInput) (*UpdateMemoryOutput, error) {
	// 1. Load existing verbatim
	verbatim, err := uc.repo.GetVerbatimByID(ctx, input.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load verbatim: %w", err)
	}
	if err := (StoreMemoryInput{
		Content: input.Content, Wing: verbatim.Wing, Room: verbatim.Room,
		ValidFrom: verbatim.ValidFrom, ValidUntil: verbatim.ValidUntil,
	}).Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Work on a copy so an extractor or transaction failure cannot mutate a
	// repository/mock object before the replacement is committed.
	updated := *verbatim
	updated.Content = input.Content
	verbatim = &updated

	// 3. Regenerate fingerprint and embedding
	fp, emb, err := uc.extractor.ExtractPipeline(ctx, verbatim, nil)
	if err != nil {
		return nil, fmt.Errorf("extraction failed: %w", err)
	}

	// 4. Atomically delete old records and insert new ones in a single transaction.
	//    If the insert fails, the transaction rolls back and the original verbatim
	//    is preserved — unlike the previous code which deleted first, then inserted.
	tx, err := uc.repo.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	if tx != nil {
		if err := uc.repo.DeleteVerbatimByIDTx(ctx, tx, input.ID); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("failed to delete old verbatim: %w", err)
		}
		if err := uc.repo.StoreVerbatimTx(ctx, tx, verbatim); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("failed to store verbatim: %w", err)
		}
		if err := uc.repo.StoreFingerprintTx(ctx, tx, fp); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("failed to store fingerprint: %w", err)
		}
		if err := uc.repo.StoreEmbeddingTx(ctx, tx, emb); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("failed to store embedding: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("failed to commit transaction: %w", err)
		}
	} else {
		// Fallback path (no transaction support): surface each write failure so a
		// partial replacement cannot be reported as a successful update.
		if err := uc.repo.DeleteVerbatimByID(ctx, input.ID); err != nil {
			return nil, fmt.Errorf("failed to delete old verbatim: %w", err)
		}
		if err := uc.repo.StoreVerbatim(ctx, verbatim); err != nil {
			return nil, fmt.Errorf("failed to store updated verbatim: %w", err)
		}
		if err := uc.repo.StoreFingerprint(ctx, fp); err != nil {
			return nil, fmt.Errorf("failed to store updated fingerprint: %w", err)
		}
		if err := uc.repo.StoreEmbedding(ctx, emb); err != nil {
			return nil, fmt.Errorf("failed to store updated embedding: %w", err)
		}
	}

	// 5. Update vector store (outside the DB transaction — it is a separate store).
	// If synchronization fails, rebuild from the authoritative repository.
	vectorErr := uc.vectorStore.Delete(ctx, input.ID)
	if vectorErr == nil {
		vectorErr = uc.vectorStore.AddCandidate(ctx, entities.NewCandidate(fp, verbatim, emb.Vector))
	}
	if vectorErr != nil {
		if repairErr := repairVectorStore(ctx, uc.vectorStore); repairErr != nil {
			return nil, fmt.Errorf("memory updated but vector index repair failed: %w", repairErr)
		}
	}

	// Rebuild all derived indexes from the replacement fingerprint.
	reindexMemoryDerivedData(ctx, uc.repo, fp, verbatim, input.Content, uc.causalDetector, nil)

	return &UpdateMemoryOutput{Verbatim: verbatim}, nil
}
