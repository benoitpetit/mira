// ClearMemory use case
package interactors

import (
	"context"
	"fmt"

	"github.com/benoitpetit/mira/internal/usecases/ports"
)

// ClearMemoryInput contains the input for clearing memories
type ClearMemoryInput struct {
	Mode string
	Wing string
	Room *string
}

// ClearMemoryOutput contains the result of clearing memories
type ClearMemoryOutput struct {
	DeletedCount int
	Mode         string
}

// ClearMemory implements the clear memory use case
type ClearMemory struct {
	repo        ports.StatsRepository
	vectorStore ports.VectorStore
}

// NewClearMemory creates a new clear memory interactor
func NewClearMemory(repo ports.StatsRepository, vectorStore ports.VectorStore) *ClearMemory {
	return &ClearMemory{
		repo:        repo,
		vectorStore: vectorStore,
	}
}

// Execute clears memories according to the specified mode
func (uc *ClearMemory) Execute(ctx context.Context, input ClearMemoryInput) (*ClearMemoryOutput, error) {
	switch input.Mode {
	case "global":
		if err := uc.repo.ClearAll(ctx); err != nil {
			return nil, fmt.Errorf("failed to clear all memories: %w", err)
		}
		if err := uc.vectorStore.ClearAll(ctx); err != nil {
			return nil, fmt.Errorf("failed to clear vector index: %w", err)
		}
		return &ClearMemoryOutput{DeletedCount: 0, Mode: "global"}, nil
	case "room":
		count, err := uc.repo.ClearByRoom(ctx, input.Wing, input.Room)
		if err != nil {
			return nil, fmt.Errorf("failed to clear room memories: %w", err)
		}
		if err := uc.vectorStore.ClearByRoom(ctx, input.Wing, input.Room); err != nil {
			return nil, fmt.Errorf("failed to clear vector index: %w", err)
		}
		return &ClearMemoryOutput{DeletedCount: count, Mode: "room"}, nil
	default:
		return nil, fmt.Errorf("invalid mode: %s (must be 'global' or 'room')", input.Mode)
	}
}
