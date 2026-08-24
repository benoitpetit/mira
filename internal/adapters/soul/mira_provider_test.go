// Package soul provides adapters that bridge MIRA to SOUL.
package soul

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/adapters/storage"
	"github.com/google/uuid"
)

// setupProviderTestDB creates a temporary SQLite database with the full MIRA
// schema (via SQLiteRepository migrations) plus the soul_mira_links table.
func setupProviderTestDB(t *testing.T) (*MiraProvider, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "mira_soul_provider_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	repo, err := storage.NewSQLiteRepository(tmpFile.Name(), storage.DefaultSQLiteOptions())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to create repository: %v", err)
	}

	db := repo.DB()

	// Create the soul_mira_links table (normally created by SOUL's initSchema).
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS soul_mira_links (
			identity_id TEXT NOT NULL,
			memory_id   TEXT NOT NULL,
			linked_at   DATETIME NOT NULL,
			PRIMARY KEY (identity_id, memory_id)
		)
	`)
	if err != nil {
		repo.Close()
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to create soul_mira_links: %v", err)
	}

	provider := NewMiraProvider(db, nil)

	cleanup := func() {
		repo.Close()
		os.Remove(tmpFile.Name())
	}
	return provider, cleanup
}

// rawInsertVerbatimFingerprint inserts a minimal verbatim + fingerprint row
// into the database and returns the verbatim UUID.
func rawInsertVerbatimFingerprint(t *testing.T, provider *MiraProvider, content, wing string) uuid.UUID {
	t.Helper()

	vid := uuid.New()
	fid := uuid.New()
	ctx := context.Background()
	now := float64(time.Now().Unix())

	_, err := provider.db.ExecContext(ctx,
		`INSERT INTO verbatim (id, content, token_count, created_at, wing, room, metadata, metrics)
		 VALUES (?, ?, ?, ?, ?, NULL, '{}', '{}')`,
		vid[:], content, len(content)/4, now, wing,
	)
	if err != nil {
		t.Fatalf("failed to insert verbatim: %v", err)
	}

	_, err = provider.db.ExecContext(ctx,
		`INSERT INTO fingerprints (id, verbatim_id, ftype, extracted_at, data, fact_count, token_estimate)
		 VALUES (?, ?, 'fact', ?, '{"custom":{}}', 1, 10)`,
		fid[:], vid[:], now,
	)
	if err != nil {
		t.Fatalf("failed to insert fingerprint: %v", err)
	}

	return vid
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestGetMiraMemories_ReturnsResults verifies that a matching verbatim+fingerprint
// is found by the LIKE search in GetMiraMemories.
func TestGetMiraMemories_ReturnsResults(t *testing.T) {
	provider, cleanup := setupProviderTestDB(t)
	defer cleanup()

	rawInsertVerbatimFingerprint(t, provider, "the quick brown fox jumps", "test-wing")

	ctx := context.Background()
	results, err := provider.GetMiraMemories(ctx, "agent-1", "quick", 10)
	if err != nil {
		t.Fatalf("GetMiraMemories returned error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result, got none")
	}
	if results[0].MemoryID == uuid.Nil {
		t.Error("expected non-nil MemoryID")
	}
	if results[0].Wing == "" {
		t.Error("expected non-empty Wing")
	}
	if results[0].Timestamp.IsZero() {
		t.Error("expected non-zero Timestamp")
	}
}

// TestGetMiraMemories_EmptyResult verifies that a query with no match returns
// an empty (non-nil) slice.
func TestGetMiraMemories_EmptyResult(t *testing.T) {
	provider, cleanup := setupProviderTestDB(t)
	defer cleanup()

	ctx := context.Background()
	results, err := provider.GetMiraMemories(ctx, "agent-1", "xyzzy_nonexistent", 10)
	if err != nil {
		t.Fatalf("GetMiraMemories returned unexpected error: %v", err)
	}
	if results == nil {
		t.Fatal("expected non-nil slice, got nil")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// TestGetMiraMemories_WingCoalesce verifies that wing is correctly returned via
// COALESCE and that the UUID is properly decoded from BLOB.
func TestGetMiraMemories_WingCoalesce(t *testing.T) {
	provider, cleanup := setupProviderTestDB(t)
	defer cleanup()

	rawInsertVerbatimFingerprint(t, provider, "coalesce wing test content", "my-wing")

	ctx := context.Background()
	results, err := provider.GetMiraMemories(ctx, "agent-1", "coalesce", 10)
	if err != nil {
		t.Fatalf("GetMiraMemories returned error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected one result")
	}
	if results[0].Wing != "my-wing" {
		t.Errorf("expected wing 'my-wing', got %q", results[0].Wing)
	}
}

// TestGetLinkedMemories_Empty verifies that GetLinkedMemories returns a nil or
// empty (non-error) slice when there are no soul_mira_links for the given identity.
func TestGetLinkedMemories_Empty(t *testing.T) {
	provider, cleanup := setupProviderTestDB(t)
	defer cleanup()

	ctx := context.Background()
	identityID := uuid.New()
	results, err := provider.GetLinkedMemories(ctx, identityID)
	if err != nil {
		t.Fatalf("GetLinkedMemories returned error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// TestLinkIdentityToMemory verifies that a soul_mira_links entry is created and
// that GetLinkedMemories subsequently returns the linked memory, and that repeated
// calls are idempotent (INSERT OR IGNORE).
func TestLinkIdentityToMemory(t *testing.T) {
	provider, cleanup := setupProviderTestDB(t)
	defer cleanup()

	ctx := context.Background()
	identityID := uuid.New()
	memoryID := rawInsertVerbatimFingerprint(t, provider, "linked memory content", "link-wing")

	if err := provider.LinkIdentityToMemory(ctx, identityID, memoryID); err != nil {
		t.Fatalf("LinkIdentityToMemory returned error: %v", err)
	}

	linked, err := provider.GetLinkedMemories(ctx, identityID)
	if err != nil {
		t.Fatalf("GetLinkedMemories returned error: %v", err)
	}
	if len(linked) == 0 {
		t.Fatal("expected one linked memory, got none")
	}
	if linked[0].MemoryID != memoryID {
		t.Errorf("expected memory ID %v, got %v", memoryID, linked[0].MemoryID)
	}

	// Idempotency: calling LinkIdentityToMemory again must not error (INSERT OR IGNORE).
	if err := provider.LinkIdentityToMemory(ctx, identityID, memoryID); err != nil {
		t.Errorf("second call to LinkIdentityToMemory should not error (idempotent), got: %v", err)
	}
}

// TestNotifyMiraOfIdentityChange_NoError verifies that the function returns without
// error. The current implementation uses INSERT OR IGNORE without token_count, so
// in SQLite STRICT mode the NOT NULL constraint fires and the row is silently
// discarded — this is a known bug tracked as Phase 2.7.
func TestNotifyMiraOfIdentityChange_NoError(t *testing.T) {
	provider, cleanup := setupProviderTestDB(t)
	defer cleanup()

	ctx := context.Background()
	if err := provider.NotifyMiraOfIdentityChange(ctx, "agent-1", "model_swap"); err != nil {
		t.Errorf("NotifyMiraOfIdentityChange returned unexpected error: %v", err)
	}
}
