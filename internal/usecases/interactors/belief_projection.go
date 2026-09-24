package interactors

import (
	"strings"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

// deriveBeliefFromFingerprint creates a conservative, deterministic belief
// projection. Only explicit decisions/preferences are promoted; raw prose is
// never guessed into an assertion.
func deriveBeliefFromFingerprint(fp *entities.Fingerprint, verbatim *entities.Verbatim) *entities.Belief {
	if fp == nil || verbatim == nil || fp.Data.Negated {
		return nil
	}

	subject := ""
	for _, candidate := range fp.Data.Subject {
		if strings.TrimSpace(candidate) != "" {
			subject = strings.TrimSpace(candidate)
			break
		}
	}
	value := strings.TrimSpace(fp.Data.Decision)
	if subject == "" || value == "" {
		return nil
	}

	predicate := "asserts"
	if fp.Type == valueobjects.TypePreference {
		predicate = "prefers"
	}
	key := verbatim.ID.String() + "|" + subject + "|" + predicate + "|" + value
	confidence := 0.5
	if raw, ok := fp.Data.Custom["extraction_confidence"].(float64); ok {
		confidence = raw
	}
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}
	now := time.Now().UTC()
	return &entities.Belief{
		ID:         uuid.NewSHA1(uuid.Nil, []byte(key)),
		Subject:    subject,
		Predicate:  predicate,
		Value:      value,
		ValidFrom:  verbatim.ValidFrom,
		ValidUntil: verbatim.ValidUntil,
		Confidence: confidence,
		Sources:    []uuid.UUID{verbatim.ID},
		Status:     beliefStatusForLifecycle(verbatim.LifecycleState),
		UpdatedAt:  now,
	}
}

func beliefStatusForLifecycle(lifecycle string) entities.BeliefStatus {
	switch lifecycle {
	case entities.LifecycleSuperseded:
		return entities.BeliefSuperseded
	case entities.LifecycleContested:
		return entities.BeliefContested
	case entities.LifecycleArchived:
		return entities.BeliefRevoked
	default:
		return entities.BeliefActive
	}
}
