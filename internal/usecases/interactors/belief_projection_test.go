package interactors

import (
	"testing"

	"github.com/benoitpetit/mira/internal/domain/entities"
)

func TestBeliefStatusForLifecycleDoesNotReactivateArchivedMemory(t *testing.T) {
	if got := beliefStatusForLifecycle(entities.LifecycleArchived); got != entities.BeliefRevoked {
		t.Fatalf("archived lifecycle mapped to %q, want revoked", got)
	}
}
