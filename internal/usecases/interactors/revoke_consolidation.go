package interactors

import (
	"context"
	"fmt"
	"strings"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// RevokeConsolidation reactivates the source memories of a synthesis, marks the
// synthesis contested, then rebuilds the derived vector index. It never deletes evidence.
type RevokeConsolidation struct {
	repository  ports.Repository
	vectorStore ports.VectorStore
}

func NewRevokeConsolidation(repository ports.Repository, vectorStores ...ports.VectorStore) *RevokeConsolidation {
	var vectorStore ports.VectorStore
	if len(vectorStores) > 0 {
		vectorStore = vectorStores[0]
	}
	return &RevokeConsolidation{repository: repository, vectorStore: vectorStore}
}

func (uc *RevokeConsolidation) Execute(ctx context.Context, synthesizedID uuid.UUID) error {
	if uc.vectorStore == nil {
		return fmt.Errorf("vector store is required for reliable consolidation revocation")
	}
	if _, ok := uc.vectorStore.(ports.VectorStoreRepairer); !ok {
		return fmt.Errorf("vector store does not support rebuild required for consolidation revocation")
	}
	writer, ok := uc.repository.(transactionalLifecycleWriter)
	if !ok {
		return fmt.Errorf("repository does not support reversible consolidation")
	}
	synthesis, err := uc.repository.GetVerbatimByID(ctx, synthesizedID)
	if err != nil {
		return err
	}
	ids, err := parseConsolidationSourceIDs(synthesis.Metadata["consolidated_from"])
	if err != nil {
		return fmt.Errorf("invalid consolidation source metadata: %w", err)
	}
	if len(ids) == 0 {
		return fmt.Errorf("consolidation source metadata is empty")
	}
	sourceBeliefs := make(map[uuid.UUID]*entities.Belief)
	for _, sourceID := range ids {
		if sourceID == synthesizedID {
			return fmt.Errorf("synthesis cannot reference itself as a source")
		}
		verbatim, err := uc.repository.GetVerbatimByID(ctx, sourceID)
		if err != nil {
			return fmt.Errorf("load consolidation source %s: %w", sourceID, err)
		}
		fingerprint, err := uc.repository.GetFingerprintByVerbatimID(ctx, sourceID)
		if err != nil {
			return fmt.Errorf("load consolidation source fingerprint %s: %w", sourceID, err)
		}
		sourceBeliefs[sourceID] = deriveBeliefFromFingerprint(fingerprint, verbatim)
	}
	tx, err := uc.repository.Begin()
	if err != nil {
		return fmt.Errorf("begin consolidation revocation: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // intentional: no-op after commit
	for _, raw := range ids {
		if err := writer.SetVerbatimLifecycleTx(ctx, tx, raw, entities.LifecycleActive, nil); err != nil {
			return err
		}
		if beliefRepo, ok := uc.repository.(ports.BeliefSourceRepository); ok {
			if err := beliefRepo.SyncBeliefSourceTx(ctx, tx, raw, sourceBeliefs[raw]); err != nil {
				return fmt.Errorf("restore belief support for source %s: %w", raw, err)
			}
		}
	}
	if err := writer.SetVerbatimLifecycleTx(ctx, tx, synthesizedID, entities.LifecycleContested, nil); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit consolidation revocation: %w", err)
	}
	if err := repairVectorStore(ctx, uc.vectorStore); err != nil {
		return fmt.Errorf("consolidation revocation committed but vector index repair failed: %w", err)
	}
	return nil
}

func parseConsolidationSourceIDs(value any) ([]uuid.UUID, error) {
	var result []uuid.UUID
	seen := make(map[uuid.UUID]struct{})
	parse := func(raw string) error {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			return fmt.Errorf("source ID %q is not a UUID", raw)
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("duplicate source ID %s", id)
		}
		seen[id] = struct{}{}
		result = append(result, id)
		return nil
	}
	if values, ok := value.([]any); ok {
		for _, item := range values {
			raw, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("source ID has type %T, want string", item)
			}
			if err := parse(raw); err != nil {
				return nil, err
			}
		}
	} else if values, ok := value.([]string); ok {
		for _, item := range values {
			if err := parse(item); err != nil {
				return nil, err
			}
		}
	} else {
		return nil, fmt.Errorf("source list has type %T, want string array", value)
	}
	return result, nil
}
