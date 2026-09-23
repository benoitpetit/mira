// Package interactors provides the application use cases.
// ArchiveMemories use case
package interactors

import (
	"context"
	"fmt"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
)

// ArchiveMemoriesOutput contains the output of archiving
type ArchiveMemoriesOutput struct {
	Result *valueobjects.ArchiveResult `json:"result"`
}

// ArchiveMemories implements the archive memories use case
type ArchiveMemories struct {
	statsRepo   ports.StatsRepository
	vectorStore ports.VectorStore
}

// NewArchiveMemories creates a new archive memories interactor
func NewArchiveMemories(statsRepo ports.StatsRepository, vectorStores ...ports.VectorStore) *ArchiveMemories {
	uc := &ArchiveMemories{statsRepo: statsRepo}
	if len(vectorStores) > 0 {
		uc.vectorStore = vectorStores[0]
	}
	return uc
}

// Execute archives old memories
func (uc *ArchiveMemories) Execute(ctx context.Context) (*ArchiveMemoriesOutput, error) {
	result, err := uc.statsRepo.ArchiveOldMemories(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to archive memories: %w", err)
	}
	if uc.vectorStore != nil {
		if err := repairVectorStore(ctx, uc.vectorStore); err != nil {
			return nil, fmt.Errorf("archive committed but vector index repair failed: %w", err)
		}
	}

	return &ArchiveMemoriesOutput{
		Result: result,
	}, nil
}
