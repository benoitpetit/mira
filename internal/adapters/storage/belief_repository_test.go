package storage

import (
	"context"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/google/uuid"
)

func TestSQLiteBeliefRepositoryPersistsResolvesAndCalibrates(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	sourceID := uuid.New()
	now := time.Now().UTC()
	belief := &entities.Belief{
		ID:         uuid.New(),
		Subject:    "MIRA",
		Predicate:  "uses",
		Value:      "SQLite",
		ValidFrom:  &now,
		Confidence: 0.8,
		Sources:    []uuid.UUID{sourceID},
		Status:     entities.BeliefActive,
		UpdatedAt:  now,
	}
	if err := repo.UpsertBelief(ctx, belief); err != nil {
		t.Fatalf("UpsertBelief: %v", err)
	}
	resolved, err := repo.ResolveBelief(ctx, "mira", "uses", now)
	if err != nil {
		t.Fatalf("ResolveBelief: %v", err)
	}
	if resolved.ID != belief.ID || resolved.Value != "SQLite" {
		t.Fatalf("resolved belief = %+v, want %+v", resolved, belief)
	}
	if err := repo.RecordBeliefFeedback(ctx, belief.ID, entities.FeedbackUseful); err != nil {
		t.Fatalf("RecordBeliefFeedback: %v", err)
	}
	if err := repo.RecordBeliefFeedback(ctx, belief.ID, entities.FeedbackStale); err != nil {
		t.Fatalf("RecordBeliefFeedback stale: %v", err)
	}
	calibration := repo.CalibrationForSource(sourceID)
	if calibration <= 0.75 || calibration >= 1.2 {
		t.Fatalf("calibration = %.3f, want bounded interior value", calibration)
	}
	if err := repo.RetractBelief(ctx, belief.ID, true); err != nil {
		t.Fatalf("RetractBelief: %v", err)
	}
	if _, err := repo.ResolveBelief(ctx, "mira", "uses", now); err == nil {
		t.Fatal("contested belief should not resolve as active")
	}
}
