package interactors

import (
	"context"
	"fmt"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// RevokeConsolidation reactivates the source memories of a synthesis and
// marks the synthesis contested. It is idempotent and never deletes evidence.
type RevokeConsolidation struct{ repository ports.Repository }

func NewRevokeConsolidation(repository ports.Repository) *RevokeConsolidation {
	return &RevokeConsolidation{repository: repository}
}

func (uc *RevokeConsolidation) Execute(ctx context.Context, synthesizedID uuid.UUID) error {
	writer, ok := uc.repository.(lifecycleWriter)
	if !ok {
		return fmt.Errorf("repository does not support reversible consolidation")
	}
	synthesis, err := uc.repository.GetVerbatimByID(ctx, synthesizedID)
	if err != nil {
		return err
	}
	for _, raw := range sourceIDs(synthesis.Metadata["consolidated_from"]) {
		if err := writer.SetVerbatimLifecycle(ctx, raw, entities.LifecycleActive, nil); err != nil {
			return err
		}
	}
	return writer.SetVerbatimLifecycle(ctx, synthesizedID, entities.LifecycleContested, nil)
}

func sourceIDs(value any) []uuid.UUID {
	var result []uuid.UUID
	if values, ok := value.([]any); ok {
		for _, item := range values {
			if parsed, err := uuid.Parse(fmt.Sprint(item)); err == nil {
				result = append(result, parsed)
			}
		}
	}
	if values, ok := value.([]string); ok {
		for _, item := range values {
			if parsed, err := uuid.Parse(item); err == nil {
				result = append(result, parsed)
			}
		}
	}
	return result
}
