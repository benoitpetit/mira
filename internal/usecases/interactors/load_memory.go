// LoadMemory use case
package interactors

import (
	"context"
	"fmt"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// LoadMemoryInput contains the input for loading a memory
type LoadMemoryInput struct {
	ID uuid.UUID
}

// LoadMemoryOutput contains the output of loading a memory
type LoadMemoryOutput struct {
	Verbatim *entities.Verbatim
}

// LoadMemory implements the load memory use case
type LoadMemory struct {
	verbatimRepo   ports.VerbatimRepository
	fingerprintRepo ports.FingerprintRepository
}

// NewLoadMemory creates a new load memory interactor
func NewLoadMemory(verbatimRepo ports.VerbatimRepository, fingerprintRepo ports.FingerprintRepository) *LoadMemory {
	return &LoadMemory{
		verbatimRepo:    verbatimRepo,
		fingerprintRepo: fingerprintRepo,
	}
}

// Execute loads a verbatim by ID. It accepts either a verbatim ID or a fingerprint ID.
func (uc *LoadMemory) Execute(ctx context.Context, input LoadMemoryInput) (*LoadMemoryOutput, error) {
	verbatim, err := uc.verbatimRepo.GetVerbatimByID(ctx, input.ID)
	if err != nil && uc.fingerprintRepo != nil {
		// Try resolving as a fingerprint ID
		fp, fpErr := uc.fingerprintRepo.GetFingerprintByID(ctx, input.ID)
		if fpErr == nil && fp != nil {
			verbatim, err = uc.verbatimRepo.GetVerbatimByID(ctx, fp.VerbatimID)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load verbatim: %w", err)
	}

	return &LoadMemoryOutput{
		Verbatim: verbatim,
	}, nil
}
