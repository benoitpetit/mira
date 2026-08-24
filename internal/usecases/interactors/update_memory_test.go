package interactors

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/benoitpetit/mira/internal/adapters/storage"
	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func setupUpdateTestDB(t *testing.T) (*storage.SQLiteRepository, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "mira_update_test_*.db")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	repo, err := storage.NewSQLiteRepository(f.Name(), storage.DefaultSQLiteOptions())
	if err != nil {
		os.Remove(f.Name())
		t.Fatalf("repo: %v", err)
	}
	return repo, func() {
		repo.Close()
		os.Remove(f.Name())
	}
}

// mockUpdateExtractor returns a deterministic fingerprint + embedding for any verbatim.
type mockUpdateExtractor struct{}

func (m *mockUpdateExtractor) ExtractPipeline(ctx context.Context, v *entities.Verbatim, _ *valueobjects.MemoryType) (*entities.Fingerprint, *entities.Embedding, error) {
	fp := entities.NewFingerprint(v.ID, valueobjects.TypeFact, "test-hash")
	fp.FactCount = 1
	fp.TokenEstimate = 4
	emb := entities.NewEmbedding(v.ID, "test-hash", make([]float32, 4))
	return fp, emb, nil
}

func (m *mockUpdateExtractor) ModelHash() string { return "test-hash" }

// mockUpdateVectorStore discards all vector operations.
type mockUpdateVectorStore struct{}

func (m *mockUpdateVectorStore) Search(_ context.Context, _ []float32, _ int, _, _ *string) ([]*entities.Candidate, error) {
	return nil, nil
}
func (m *mockUpdateVectorStore) SearchLexical(_ context.Context, _ string, _ int, _, _ *string) ([]*entities.Candidate, error) {
	return nil, nil
}
func (m *mockUpdateVectorStore) AddCandidate(_ context.Context, _ *entities.Candidate) error {
	return nil
}
func (m *mockUpdateVectorStore) Delete(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockUpdateVectorStore) ClearAll(_ context.Context) error             { return nil }
func (m *mockUpdateVectorStore) ClearByRoom(_ context.Context, _ string, _ *string) error {
	return nil
}

// insertTestVerbatimForUpdate stores a verbatim with a trivial fingerprint + embedding.
func insertTestVerbatimForUpdate(t *testing.T, repo *storage.SQLiteRepository, content, wing string) *entities.Verbatim {
	t.Helper()
	ctx := context.Background()

	v := entities.NewVerbatim(content, wing, nil)
	v.TokenCount = 4

	fp := entities.NewFingerprint(v.ID, valueobjects.TypeFact, "test-hash")
	fp.FactCount = 1
	fp.TokenEstimate = 4

	emb := entities.NewEmbedding(v.ID, "test-hash", make([]float32, 4))

	tx, err := repo.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := repo.StoreVerbatimTx(ctx, tx, v); err != nil {
		tx.Rollback()
		t.Fatalf("store verbatim: %v", err)
	}
	if err := repo.StoreFingerprintTx(ctx, tx, fp); err != nil {
		tx.Rollback()
		t.Fatalf("store fingerprint: %v", err)
	}
	if err := repo.StoreEmbeddingTx(ctx, tx, emb); err != nil {
		tx.Rollback()
		t.Fatalf("store embedding: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return v
}

// ---------------------------------------------------------------------------
// failingStoreRepo wraps a real repo and makes StoreVerbatimTx always fail.
// Used to simulate an I/O error that occurs after DeleteVerbatimByIDTx has
// already been called within the same transaction — verifying that the
// rollback preserves the original verbatim.
// ---------------------------------------------------------------------------

type failingStoreRepo struct {
	*storage.SQLiteRepository
}

// StoreVerbatimTx always returns an error, simulating an I/O failure.
func (f *failingStoreRepo) StoreVerbatimTx(_ context.Context, _ *sql.Tx, _ *entities.Verbatim) error {
	return errors.New("simulated store failure")
}

// Ensure failingStoreRepo satisfies ports.Repository at compile time.
var _ ports.Repository = (*failingStoreRepo)(nil)

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestUpdateMemory_Success verifies the happy path: after a successful Execute,
// the verbatim's content is updated in the database.
func TestUpdateMemory_Success(t *testing.T) {
	repo, cleanup := setupUpdateTestDB(t)
	defer cleanup()

	ctx := context.Background()
	original := insertTestVerbatimForUpdate(t, repo, "original content", "test-wing")

	uc := NewUpdateMemory(repo, &mockUpdateExtractor{}, &mockUpdateVectorStore{})
	out, err := uc.Execute(ctx, UpdateMemoryInput{
		ID:      original.ID,
		Content: "updated content",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out == nil || out.Verbatim == nil {
		t.Fatal("expected non-nil output verbatim")
	}
	if out.Verbatim.Content != "updated content" {
		t.Errorf("content mismatch: want %q, got %q", "updated content", out.Verbatim.Content)
	}

	// Confirm the new content is persisted in the DB.
	stored, err := repo.GetVerbatimByID(ctx, original.ID)
	if err != nil {
		t.Fatalf("GetVerbatimByID after update: %v", err)
	}
	if stored.Content != "updated content" {
		t.Errorf("DB content mismatch: want %q, got %q", "updated content", stored.Content)
	}
}

// TestUpdateMemory_RollbackOnStoreFailure verifies that when StoreVerbatimTx fails
// the DELETE is rolled back and the original verbatim is preserved in the database.
// This documents the fix introduced in Phase 2.1: DELETE and INSERT now share a
// single transaction, so a store failure cannot cause data loss.
func TestUpdateMemory_RollbackOnStoreFailure(t *testing.T) {
	realRepo, cleanup := setupUpdateTestDB(t)
	defer cleanup()

	ctx := context.Background()
	original := insertTestVerbatimForUpdate(t, realRepo, "original content", "test-wing")

	// Wrap the repo so that StoreVerbatimTx always fails.
	failRepo := &failingStoreRepo{SQLiteRepository: realRepo}

	uc := NewUpdateMemory(failRepo, &mockUpdateExtractor{}, &mockUpdateVectorStore{})
	_, err := uc.Execute(ctx, UpdateMemoryInput{
		ID:      original.ID,
		Content: "should not be stored",
	})
	if err == nil {
		t.Fatal("expected Execute to return an error, got nil")
	}

	// The original verbatim must still be retrievable — the transaction was rolled back.
	stored, err := realRepo.GetVerbatimByID(ctx, original.ID)
	if err != nil {
		t.Fatalf("original verbatim is gone after rollback: %v", err)
	}
	if stored.Content != "original content" {
		t.Errorf("expected %q, got %q — original data was not preserved", "original content", stored.Content)
	}
}
