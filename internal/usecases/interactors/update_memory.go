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
	repo        ports.Repository
	extractor   ports.FingerprintExtractor
	vectorStore ports.VectorStore
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

	// 2. Update content
	verbatim.Content = input.Content

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
		// Fallback path (no transaction support): keep original best-effort order.
		if err := uc.repo.DeleteVerbatimByID(ctx, input.ID); err != nil {
			return nil, fmt.Errorf("failed to delete old verbatim: %w", err)
		}
		_ = uc.repo.StoreVerbatim(ctx, verbatim)
		_ = uc.repo.StoreFingerprint(ctx, fp)
		_ = uc.repo.StoreEmbedding(ctx, emb)
	}

	// 5. Update vector store (outside the DB transaction — it is a separate store).
	_ = uc.vectorStore.Delete(ctx, input.ID)
	candidate := entities.NewCandidate(fp, verbatim, emb.Vector)
	_ = uc.vectorStore.AddCandidate(ctx, candidate)

	return &UpdateMemoryOutput{Verbatim: verbatim}, nil
}
