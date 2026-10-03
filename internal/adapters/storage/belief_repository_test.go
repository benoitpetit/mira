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
	source := entities.NewVerbatim("Mira prefers SQLite", "belief-test", nil)
	source.ID = sourceID
	if err := repo.StoreVerbatim(ctx, source); err != nil {
		t.Fatal(err)
	}
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

func TestSQLiteBeliefSupportTracksOnlyActiveSourcesAndCanBeReactivated(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UTC()

	sources := []*entities.Verbatim{
		entities.NewVerbatim("Mira prefers concise answers", "belief-support", nil),
		entities.NewVerbatim("Mira prefers concise answers", "belief-support", nil),
	}
	for _, source := range sources {
		if err := repo.StoreVerbatim(ctx, source); err != nil {
			t.Fatal(err)
		}
	}
	belief := &entities.Belief{ID: uuid.New(), Subject: "Mira", Predicate: "prefers", Value: "concise answers", Confidence: .8, Status: entities.BeliefActive, UpdatedAt: now}
	for i, source := range sources {
		tx, err := repo.Begin()
		if err != nil {
			t.Fatal(err)
		}
		sourceBelief := *belief
		if i > 0 {
			sourceBelief.ID = uuid.New() // legacy/source-specific IDs still reconcile by assertion identity
		}
		if err := repo.SyncBeliefSourceTx(ctx, tx, source.ID, &sourceBelief); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}

	resolved, err := repo.ResolveBelief(ctx, "Mira", "prefers", now)
	if err != nil || len(resolved.Sources) != 2 {
		t.Fatalf("resolved belief sources = %#v, err=%v; want both active sources", resolved, err)
	}
	if err := repo.SetVerbatimLifecycle(ctx, sources[0].ID, entities.LifecycleArchived, nil); err != nil {
		t.Fatal(err)
	}
	resolved, err = repo.ResolveBelief(ctx, "Mira", "prefers", now)
	if err != nil || len(resolved.Sources) != 1 || resolved.Sources[0] != sources[1].ID {
		t.Fatalf("belief after archiving one source = %#v, err=%v; want remaining active support", resolved, err)
	}
	if err := repo.SetVerbatimLifecycle(ctx, sources[1].ID, entities.LifecycleArchived, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ResolveBelief(ctx, "Mira", "prefers", now); err == nil {
		t.Fatal("belief resolved after all supporting sources were archived")
	}
	if err := repo.SetVerbatimLifecycle(ctx, sources[0].ID, entities.LifecycleActive, nil); err != nil {
		t.Fatal(err)
	}
	resolved, err = repo.ResolveBelief(ctx, "Mira", "prefers", now)
	if err != nil || len(resolved.Sources) != 1 || resolved.Sources[0] != sources[0].ID {
		t.Fatalf("belief after source restore = %#v, err=%v; want restored support", resolved, err)
	}
}

func TestSQLiteDeletingBeliefSourceRemovesOnlyThatSupport(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UTC()
	sources := []*entities.Verbatim{
		entities.NewVerbatim("source one", "belief-delete", nil),
		entities.NewVerbatim("source two", "belief-delete", nil),
	}
	belief := &entities.Belief{ID: uuid.New(), Subject: "Mira", Predicate: "prefers", Value: "concise", Confidence: .7, Status: entities.BeliefActive, UpdatedAt: now}
	for _, source := range sources {
		if err := repo.StoreVerbatim(ctx, source); err != nil {
			t.Fatal(err)
		}
		tx, err := repo.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.SyncBeliefSourceTx(ctx, tx, source.ID, belief); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.DeleteVerbatimByID(ctx, sources[0].ID); err != nil {
		t.Fatal(err)
	}
	resolved, err := repo.ResolveBelief(ctx, "Mira", "prefers", now)
	if err != nil || len(resolved.Sources) != 1 || resolved.Sources[0] != sources[1].ID {
		t.Fatalf("belief after deleting one source = %#v, err=%v; want surviving source only", resolved, err)
	}
	if _, err := repo.ClearByIDs(ctx, []uuid.UUID{sources[1].ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ResolveBelief(ctx, "Mira", "prefers", now); err == nil {
		t.Fatal("belief resolved after deleting every support source")
	}
}

func TestSQLiteBeliefSupportMigrationDropsMissingAndMalformedSources(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	active := entities.NewVerbatim("active", "migration", nil)
	archived := entities.NewVerbatim("archived", "migration", nil)
	deleted := entities.NewVerbatim("deleted", "migration", nil)
	for _, source := range []*entities.Verbatim{active, archived, deleted} {
		if err := repo.StoreVerbatim(ctx, source); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.SetVerbatimLifecycle(ctx, archived.ID, entities.LifecycleArchived, nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteVerbatimByID(ctx, deleted.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().Exec(`DROP TABLE belief_sources`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().Exec(`DROP TABLE belief_source_migration_diagnostics`); err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		sources string
		want    string
	}{
		{sources: `[` + `"` + active.ID.String() + `"` + `]`, want: "active"},
		{sources: `[` + `"` + archived.ID.String() + `"` + `]`, want: "revoked"},
		{sources: `[` + `"` + deleted.ID.String() + `"` + `]`, want: "revoked"},
		{sources: `not-json`, want: "revoked"},
		{sources: `5`, want: "revoked"},
		{sources: `["not-a-uuid"]`, want: "revoked"},
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		id := uuid.New()
		ids = append(ids, id)
		if _, err := repo.DB().Exec(`INSERT INTO beliefs(id,subject,predicate,value,sources,status,updated_at) VALUES(?,?,?,?,?,?,?)`, id.String(), "Mira", "prefers", id.String(), row.sources, "active", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}
	migration, err := migrationsFS.ReadFile("migrations/018_belief_sources.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().Exec(string(migration)); err != nil {
		t.Fatalf("apply belief support migration: %v", err)
	}
	for i, expected := range rows {
		var status string
		if err := repo.DB().QueryRow(`SELECT status FROM beliefs WHERE id=?`, ids[i].String()).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != expected.want {
			t.Errorf("belief %d status = %q, want %q", i, status, expected.want)
		}
	}
	var supportCount, activeCount, discarded int
	if err := repo.DB().QueryRow(`SELECT COUNT(*), SUM(active) FROM belief_sources`).Scan(&supportCount, &activeCount); err != nil {
		t.Fatal(err)
	}
	if supportCount != 2 || activeCount != 1 {
		t.Errorf("backfilled supports = %d, active = %d; want 2 and 1", supportCount, activeCount)
	}
	if err := repo.DB().QueryRow(`SELECT discarded_source_references FROM belief_source_migration_diagnostics WHERE migration_version=18`).Scan(&discarded); err != nil {
		t.Fatal(err)
	}
	if discarded != 4 {
		t.Errorf("discarded source diagnostic = %d, want 4", discarded)
	}
}
