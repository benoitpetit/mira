package interactors

import (
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

func TestBeliefStatusForLifecycleDoesNotReactivateArchivedMemory(t *testing.T) {
	if got := beliefStatusForLifecycle(entities.LifecycleArchived); got != entities.BeliefRevoked {
		t.Fatalf("archived lifecycle mapped to %q, want revoked", got)
	}
}

func TestDerivedBeliefIdentityIsSharedAcrossEquivalentActiveSources(t *testing.T) {
	first := entities.NewVerbatim("Mira prefers concise answers", "belief", nil)
	second := entities.NewVerbatim("Mira prefers concise answers", "belief", nil)
	validFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	first.ValidFrom, second.ValidFrom = &validFrom, &validFrom
	firstFP := entities.NewFingerprint(first.ID, valueobjects.TypePreference, "model")
	secondFP := entities.NewFingerprint(second.ID, valueobjects.TypePreference, "model")
	data := valueobjects.FingerprintData{Subject: []string{"Mira"}, Decision: "concise answers"}
	firstFP.WithData(data)
	secondFP.WithData(data)

	firstBelief := deriveBeliefFromFingerprint(firstFP, first)
	secondBelief := deriveBeliefFromFingerprint(secondFP, second)
	if firstBelief == nil || secondBelief == nil || firstBelief.ID != secondBelief.ID {
		t.Fatalf("equivalent sources produced belief IDs %v and %v; want one shared assertion identity", idOrNil(firstBelief), idOrNil(secondBelief))
	}
}

func idOrNil(belief *entities.Belief) uuid.UUID {
	if belief == nil {
		return uuid.Nil
	}
	return belief.ID
}
