package storage

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
	_ "github.com/benoitpetit/go-sqlcipher/v4"
)

// closedRepo returns a repo whose underlying DB has already been closed,
// so any Begin()/BeginTx() call returns an error immediately.
func closedRepo(t *testing.T) *SQLiteRepository {
	t.Helper()
	repo, cleanup := setupTestDB(t)
	cleanup() // closes the db and removes the temp file
	return repo
}

// ──────────────────────────────────────────────────────────────────────────────
// Begin() error paths
// ──────────────────────────────────────────────────────────────────────────────

func TestStoreVerbatim_BeginError(t *testing.T) {
	r := closedRepo(t)
	v := entities.NewVerbatim("hello", "wing", nil)
	if err := r.StoreVerbatim(context.Background(), v); err == nil {
		t.Error("expected error from closed db")
	}
}

func TestDeleteVerbatimByID_BeginError(t *testing.T) {
	r := closedRepo(t)
	if err := r.DeleteVerbatimByID(context.Background(), uuid.New()); err == nil {
		t.Error("expected error from closed db")
	}
}

func TestStoreFingerprint_BeginError(t *testing.T) {
	r := closedRepo(t)
	fp := &entities.Fingerprint{
		ID:          uuid.New(),
		VerbatimID:  uuid.New(),
		Type:        valueobjects.MemoryType("fact"),
		ExtractedAt: time.Now(),
	}
	if err := r.StoreFingerprint(context.Background(), fp); err == nil {
		t.Error("expected error from closed db")
	}
}

func TestGetRecentFingerprintsByWing_BeginError(t *testing.T) {
	r := closedRepo(t)
	_, err := r.GetRecentFingerprintsByWing(context.Background(), "wing", uuid.New(), 10)
	if err == nil {
		t.Error("expected error from closed db")
	}
}

func TestStoreEmbedding_BeginError(t *testing.T) {
	r := closedRepo(t)
	emb := &entities.Embedding{
		ID:        uuid.New(),
		ModelHash: "test",
		Dim:       3,
		Vector:    []float32{0.1, 0.2, 0.3},
		CreatedAt: time.Now(),
	}
	if err := r.StoreEmbedding(context.Background(), emb); err == nil {
		t.Error("expected error from closed db")
	}
}

func TestAddNode_BeginError(t *testing.T) {
	r := closedRepo(t)
	node := entities.NewCausalNode(uuid.New(), "fact", "summary", "wing", nil)
	if err := r.AddNode(context.Background(), node); err == nil {
		t.Error("expected error from closed db")
	}
}

func TestAddEdge_BeginError(t *testing.T) {
	r := closedRepo(t)
	edge := &entities.CausalEdge{
		FromID:     uuid.New(),
		ToID:       uuid.New(),
		Relation:   valueobjects.RelBecause,
		Weight:     1.0,
		DetectedAt: time.Now(),
	}
	if err := r.AddEdge(context.Background(), edge); err == nil {
		t.Error("expected error from closed db")
	}
}

func TestStoreTags_BeginError(t *testing.T) {
	r := closedRepo(t)
	// Non-empty tags list so we reach the Begin() call.
	if err := r.StoreTags(context.Background(), uuid.New(), []string{"tag1"}, "entity"); err == nil {
		t.Error("expected error from closed db")
	}
}

func TestStoreTags_EmptyTags(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	// Empty slice → early return nil.
	if err := repo.StoreTags(context.Background(), uuid.New(), []string{}, "entity"); err != nil {
		t.Errorf("expected nil for empty tags, got: %v", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// QueryContext error paths (closed DB)
// ──────────────────────────────────────────────────────────────────────────────

func TestGetChain_ClosedDB(t *testing.T) {
	r := closedRepo(t)
	_, err := r.GetChain(context.Background(), uuid.New(), 3)
	if err == nil {
		t.Error("expected error from closed db")
	}
}

func TestGetChain_ZeroMaxDepth(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	// maxDepth <= 0 → default to 5; returns empty list (no nodes).
	nodes, err := repo.GetChain(context.Background(), uuid.New(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected empty chain, got %d", len(nodes))
	}
}

func TestGetConsequences_ClosedDB(t *testing.T) {
	r := closedRepo(t)
	_, err := r.GetConsequences(context.Background(), uuid.New(), 3)
	if err == nil {
		t.Error("expected error from closed db")
	}
}

func TestGetConsequences_ZeroMaxDepth(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	nodes, err := repo.GetConsequences(context.Background(), uuid.New(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected empty consequences, got %d", len(nodes))
	}
}

func TestGetAllEmbeddings_ClosedDB(t *testing.T) {
	r := closedRepo(t)
	_, err := r.GetAllEmbeddings(context.Background())
	if err == nil {
		t.Error("expected error from closed db")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// NewSQLiteRepository — invalid path (db.Ping error)
// ──────────────────────────────────────────────────────────────────────────────

func TestNewSQLiteRepository_InvalidPath(t *testing.T) {
	_, err := NewSQLiteRepository("/nonexistent-dir/mira_test.db", DefaultSQLiteOptions())
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// parseMigrationVersion
// ──────────────────────────────────────────────────────────────────────────────

func TestParseMigrationVersion_InvalidName(t *testing.T) {
	_, err := parseMigrationVersion("nounderscorehere.up.sql")
	if err == nil {
		t.Error("expected error for name without underscore separator")
	}
}

func TestParseMigrationVersion_ValidName(t *testing.T) {
	v, err := parseMigrationVersion("001_initial.up.sql")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 1 {
		t.Errorf("expected version 1, got %d", v)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// runMigrations error path — closed DB triggers the CREATE TABLE failure
// ──────────────────────────────────────────────────────────────────────────────

func TestRunMigrations_ClosedDB(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.Close() // closed → all Exec calls fail
	if err := runMigrations(db); err == nil {
		t.Error("expected error for closed db")
	}
}
