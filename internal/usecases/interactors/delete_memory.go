// DeleteMemory use case
package interactors

import (
	"context"
	"fmt"

	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// DeleteMemoryInput contains the input for deleting a memory
type DeleteMemoryInput struct {
	ID uuid.UUID
}

// DeleteMemory implements the delete memory use case
type DeleteMemory struct {
	repo        ports.Repository
	vectorStore ports.VectorStore
}

// NewDeleteMemory creates a new delete memory interactor
func NewDeleteMemory(repo ports.Repository, vectorStore ports.VectorStore) *DeleteMemory {
	return &DeleteMemory{
		repo:        repo,
		vectorStore: vectorStore,
	}
}

// Execute deletes a memory by ID from the repository and vector index.
func (uc *DeleteMemory) Execute(ctx context.Context, input DeleteMemoryInput) error {
	if err := uc.repo.DeleteVerbatimByID(ctx, input.ID); err != nil {
		return fmt.Errorf("failed to delete verbatim: %w", err)
	}
	if err := uc.vectorStore.Delete(ctx, input.ID); err != nil {
		// Non-fatal: vector store may be unavailable or index not ready
	}
	return nil
}
