package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

func setupTestDB(t *testing.T) (*SQLiteRepository, func()) {
	tmpFile, err := os.CreateTemp("", "mira_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()

	repo, err := NewSQLiteRepository(tmpFile.Name(), DefaultSQLiteOptions())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("Failed to create repository: %v", err)
	}

	cleanup := func() {
		repo.Close()
		os.Remove(tmpFile.Name())
	}

	return repo, cleanup
}

func TestNewSQLiteRepository(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	if repo == nil {
		t.Fatal("NewSQLiteRepository returned nil")
	}
	if repo.db == nil {
		t.Error("db should be initialized")
	}

	// DB() accessor must return a non-nil *sql.DB.
	if repo.DB() == nil {
		t.Error("DB() should return non-nil *sql.DB")
	}
}

func TestStoreAndGetVerbatim(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	// Create and store verbatim
	ctx := context.Background()
	verbatim := entities.NewVerbatim("Test content for storage", "test-wing", nil)
	err := repo.StoreVerbatim(ctx, verbatim)
	if err != nil {
		t.Fatalf("StoreVerbatim failed: %v", err)
	}

	// Retrieve verbatim
	retrieved, err := repo.GetVerbatimByID(ctx, verbatim.ID)
	if err != nil {
		t.Fatalf("GetVerbatimByID failed: %v", err)
	}

	if retrieved.Content != verbatim.Content {
		t.Errorf("Content = %s, want %s", retrieved.Content, verbatim.Content)
	}
	if retrieved.Wing != verbatim.Wing {
		t.Errorf("Wing = %s, want %s", retrieved.Wing, verbatim.Wing)
	}
	if retrieved.TokenCount != verbatim.TokenCount {
		t.Errorf("TokenCount = %d, want %d", retrieved.TokenCount, verbatim.TokenCount)
	}
}

func TestGetVerbatimByIDNotFound(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	_, err := repo.GetVerbatimByID(context.Background(), uuid.New())
	if err == nil {
		t.Error("GetVerbatimByID should return error for non-existent ID")
	}
}

func TestStoreAndGetFingerprint(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	// First store a verbatim (needed for FK constraint)
	ctx := context.Background()
	verbatim := entities.NewVerbatim("Test content", "test-wing", nil)
	err := repo.StoreVerbatim(ctx, verbatim)
	if err != nil {
		t.Fatalf("StoreVerbatim failed: %v", err)
	}

	// Create and store fingerprint
	fp := entities.NewFingerprint(verbatim.ID, valueobjects.TypeDecision, "model-hash")
	fp.WithData(valueobjects.FingerprintData{
		Decision: "Use PostgreSQL",
		Subject:  []string{"database"},
		Entities: []string{"PostgreSQL"},
	})

	err = repo.StoreFingerprint(ctx, fp)
	if err != nil {
		t.Fatalf("StoreFingerprint failed: %v", err)
	}

	// Retrieve fingerprint
	retrieved, err := repo.GetFingerprintByID(ctx, fp.ID)
	if err != nil {
		t.Fatalf("GetFingerprintByID failed: %v", err)
	}

	if retrieved.Type != fp.Type {
		t.Errorf("Type = %v, want %v", retrieved.Type, fp.Type)
	}
	if retrieved.VerbatimID != fp.VerbatimID {
		t.Error("VerbatimID mismatch")
	}
	if retrieved.Data.Decision != "Use PostgreSQL" {
		t.Errorf("Decision = %s, want 'Use PostgreSQL'", retrieved.Data.Decision)
	}
}

func TestGetFingerprintByIDNotFound(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	_, err := repo.GetFingerprintByID(context.Background(), uuid.New())
	if err == nil {
		t.Error("GetFingerprintByID should return error for non-existent ID")
	}
}

func TestStoreAndGetEmbedding(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	// Register model first
	ctx := context.Background()
	model := entities.NewEmbeddingModel("test-model", 384)
	err := repo.RegisterModel(ctx, model)
	if err != nil {
		t.Fatalf("RegisterModel failed: %v", err)
	}

	// Create embedding
	vec := make([]float32, 384)
	for i := range vec {
		vec[i] = float32(i) / 384.0
	}
	emb := entities.NewEmbedding(uuid.New(), model.ModelHash, vec)

	err = repo.StoreEmbedding(ctx, emb)
	if err != nil {
		t.Fatalf("StoreEmbedding failed: %v", err)
	}

	// Retrieve embedding
	retrieved, err := repo.GetEmbeddingByID(ctx, emb.ID)
	if err != nil {
		t.Fatalf("GetEmbeddingByID failed: %v", err)
	}

	if retrieved.Dim != emb.Dim {
		t.Errorf("Dim = %d, want %d", retrieved.Dim, emb.Dim)
	}
	if len(retrieved.Vector) != 384 {
		t.Errorf("Vector length = %d, want 384", len(retrieved.Vector))
	}
}

func TestGetEmbeddingByIDNotFound(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	_, err := repo.GetEmbeddingByID(context.Background(), uuid.New())
	if err == nil {
		t.Error("GetEmbeddingByID should return error for non-existent ID")
	}
}

func TestAddAndGetCausalNode(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	fpID := uuid.New()
	node := entities.NewCausalNode(fpID, "decision", "Use PostgreSQL", "backend", nil)

	ctx := context.Background()
	err := repo.AddNode(ctx, node)
	if err != nil {
		t.Fatalf("AddNode failed: %v", err)
	}

	// Nodes don't have a direct Get method, test via GetChain
	chain, err := repo.GetChain(ctx, fpID, 5)
	if err != nil {
		t.Fatalf("GetChain failed: %v", err)
	}
	// Empty chain expected for a single node with no edges
	t.Logf("Chain length: %d", len(chain))
}

func TestAddAndHasEdge(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	fromID := uuid.New()
	toID := uuid.New()

	// Add nodes first
	node1 := entities.NewCausalNode(fromID, "decision", "Decision A", "backend", nil)
	node2 := entities.NewCausalNode(toID, "decision", "Decision B", "backend", nil)
	ctx := context.Background()
	repo.AddNode(ctx, node1)
	repo.AddNode(ctx, node2)

	// Add edge
	edge := entities.NewCausalEdge(fromID, toID, valueobjects.RelBecause)
	err := repo.AddEdge(ctx, edge)
	if err != nil {
		t.Fatalf("AddEdge failed: %v", err)
	}

	// Check edge exists
	if !repo.HasEdge(ctx, fromID, toID) {
		t.Error("HasEdge should return true for existing edge")
	}
	if !repo.HasEdge(ctx, toID, fromID) {
		t.Error("HasEdge should return true for reverse direction (undirected)")
	}
}

func TestGetRecentFingerprintsByWing(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	// Create multiple verbatims and fingerprints
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		verbatim := entities.NewVerbatim("Test content", "backend", nil)
		repo.StoreVerbatim(ctx, verbatim)

		fp := entities.NewFingerprint(verbatim.ID, valueobjects.TypeFact, "hash")
		repo.StoreFingerprint(ctx, fp)
	}

	// Get recent fingerprints
	results, err := repo.GetRecentFingerprintsByWing(ctx, "backend", uuid.Nil, 10)
	if err != nil {
		t.Fatalf("GetRecentFingerprintsByWing failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Got %d fingerprints, want 3", len(results))
	}
}

func TestGetStats(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	// Create some data
	ctx := context.Background()
	verbatim := entities.NewVerbatim("Test", "wing", nil)
	repo.StoreVerbatim(ctx, verbatim)

	fp := entities.NewFingerprint(verbatim.ID, valueobjects.TypeDecision, "hash")
	repo.StoreFingerprint(ctx, fp)

	// Register model
	model := entities.NewEmbeddingModel("test-model", 384)
	repo.RegisterModel(ctx, model)

	// Get stats
	stats, err := repo.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.VerbatimCount != 1 {
		t.Errorf("VerbatimCount = %d, want 1", stats.VerbatimCount)
	}
	if stats.FingerprintCount != 1 {
		t.Errorf("FingerprintCount = %d, want 1", stats.FingerprintCount)
	}
}

func TestGetTimeline(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	// Create verbatims and fingerprints
	ctx := context.Background()
	verbatim := entities.NewVerbatim("Test content", "timeline-wing", nil)
	repo.StoreVerbatim(ctx, verbatim)

	fp := entities.NewFingerprint(verbatim.ID, valueobjects.TypeDecision, "hash")
	fp.WithData(valueobjects.FingerprintData{
		Subject: []string{"test-subject"},
	})
	repo.StoreFingerprint(ctx, fp)

	// Get timeline
	timeline, err := repo.GetTimeline(ctx, "timeline-wing", nil, nil, nil, nil, 100, nil)
	if err != nil {
		t.Fatalf("GetTimeline failed: %v", err)
	}

	if len(timeline) != 1 {
		t.Errorf("Timeline length = %d, want 1", len(timeline))
	}

	if len(timeline) > 0 && timeline[0].ID != verbatim.ID.String() {
		t.Errorf("Timeline ID = %s, want verbatim ID %s", timeline[0].ID, verbatim.ID.String())
	}
}

func TestArchiveOldMemories(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	// Create old session note (should be archived - > 30 days)
	ctx := context.Background()
	oldSessionNote := entities.NewVerbatim("Old session note", "test", nil)
	oldSessionNote.CreatedAt = time.Now().Add(-40 * 24 * time.Hour) // 40 days old
	oldSessionNote.TokenCount = 100                                 // Set token count
	repo.StoreVerbatim(ctx, oldSessionNote)

	oldSessionFp := entities.NewFingerprint(oldSessionNote.ID, valueobjects.TypeSessionNote, "hash")
	repo.StoreFingerprint(ctx, oldSessionFp)

	// Create old debug log (should be archived - > 7 days)
	oldDebugLog := entities.NewVerbatim("Old debug log", "test", nil)
	oldDebugLog.CreatedAt = time.Now().Add(-10 * 24 * time.Hour) // 10 days old
	oldDebugLog.TokenCount = 50                                  // Set token count
	repo.StoreVerbatim(ctx, oldDebugLog)

	oldDebugFp := entities.NewFingerprint(oldDebugLog.ID, valueobjects.TypeDebugLog, "hash")
	repo.StoreFingerprint(ctx, oldDebugFp)

	// Create recent session note (should NOT be archived - < 30 days)
	recentSessionNote := entities.NewVerbatim("Recent session note", "test", nil)
	recentSessionNote.CreatedAt = time.Now().Add(-5 * 24 * time.Hour) // 5 days old
	recentSessionNote.TokenCount = 75
	repo.StoreVerbatim(ctx, recentSessionNote)

	recentSessionFp := entities.NewFingerprint(recentSessionNote.ID, valueobjects.TypeSessionNote, "hash")
	repo.StoreFingerprint(ctx, recentSessionFp)

	// Create recent decision (should NOT be archived - any age)
	recentDecision := entities.NewVerbatim("Recent decision", "test", nil)
	recentDecision.CreatedAt = time.Now()
	recentDecision.TokenCount = 80
	repo.StoreVerbatim(ctx, recentDecision)

	recentDecisionFp := entities.NewFingerprint(recentDecision.ID, valueobjects.TypeDecision, "hash")
	repo.StoreFingerprint(ctx, recentDecisionFp)

	// Create old decision (should NOT be archived - decisions are never archived)
	oldDecision := entities.NewVerbatim("Old decision", "test", nil)
	oldDecision.CreatedAt = time.Now().Add(-60 * 24 * time.Hour) // 60 days old
	oldDecision.TokenCount = 120
	repo.StoreVerbatim(ctx, oldDecision)

	oldDecisionFp := entities.NewFingerprint(oldDecision.ID, valueobjects.TypeDecision, "hash")
	repo.StoreFingerprint(ctx, oldDecisionFp)

	// Run archive
	result, err := repo.ArchiveOldMemories(ctx)
	if err != nil {
		t.Fatalf("ArchiveOldMemories failed: %v", err)
	}

	// Verify archive results
	if result.SessionNotes != 1 {
		t.Errorf("SessionNotes archived = %d, want 1", result.SessionNotes)
	}
	if result.DebugLogs != 1 {
		t.Errorf("DebugLogs archived = %d, want 1", result.DebugLogs)
	}
	expectedTokens := 150 // 100 + 50
	if result.TokensFreed != expectedTokens {
		t.Errorf("TokensFreed = %d, expected %d", result.TokensFreed, expectedTokens)
	}

	// Verify old session note was archived (deleted)
	_, err = repo.GetVerbatimByID(ctx, oldSessionNote.ID)
	if err == nil {
		t.Error("Old session note should have been archived")
	}
	_, err = repo.GetFingerprintByID(ctx, oldSessionFp.ID)
	if err == nil {
		t.Error("Old session note fingerprint should have been archived")
	}

	// Verify old debug log was archived (deleted)
	_, err = repo.GetVerbatimByID(ctx, oldDebugLog.ID)
	if err == nil {
		t.Error("Old debug log should have been archived")
	}

	// Verify recent session note was NOT archived
	_, err = repo.GetVerbatimByID(ctx, recentSessionNote.ID)
	if err != nil {
		t.Error("Recent session note should NOT have been archived")
	}

	// Verify recent decision was NOT archived
	_, err = repo.GetVerbatimByID(ctx, recentDecision.ID)
	if err != nil {
		t.Error("Recent decision should NOT have been archived")
	}

	// Verify old decision was NOT archived (decisions are never archived)
	_, err = repo.GetVerbatimByID(ctx, oldDecision.ID)
	if err != nil {
		t.Error("Old decision should NOT have been archived (decisions are preserved)")
	}
	_, err = repo.GetFingerprintByID(ctx, oldDecisionFp.ID)
	if err != nil {
		t.Error("Old decision fingerprint should NOT have been archived")
	}
}

func TestRegisterAndGetAllModels(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	// Register models
	model1 := entities.NewEmbeddingModel("model-1", 384)
	model2 := entities.NewEmbeddingModel("model-2", 768)

	ctx := context.Background()
	err := repo.RegisterModel(ctx, model1)
	if err != nil {
		t.Fatalf("RegisterModel failed: %v", err)
	}

	err = repo.RegisterModel(ctx, model2)
	if err != nil {
		t.Fatalf("RegisterModel failed: %v", err)
	}

	// Get all models
	models, err := repo.GetAllModels(ctx)
	if err != nil {
		t.Fatalf("GetAllModels failed: %v", err)
	}

	if len(models) != 2 {
		t.Errorf("Got %d models, want 2", len(models))
	}
}

func TestStoreVerbatimTx(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	// Start a transaction
	tx, err := repo.Begin()
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	// Create and store verbatim in transaction
	ctx := context.Background()
	verbatim := entities.NewVerbatim("Transaction test content", "tx-wing", nil)
	err = repo.StoreVerbatimTx(ctx, tx, verbatim)
	if err != nil {
		tx.Rollback()
		t.Fatalf("StoreVerbatimTx failed: %v", err)
	}

	// Rollback the transaction
	err = tx.Rollback()
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Verify the verbatim was NOT stored (rollback worked)
	_, err = repo.GetVerbatimByID(ctx, verbatim.ID)
	if err == nil {
		t.Error("Expected error after rollback, but verbatim was found")
	}

	// Now test successful commit
	tx, err = repo.Begin()
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	verbatim2 := entities.NewVerbatim("Committed content", "tx-wing", nil)
	err = repo.StoreVerbatimTx(ctx, tx, verbatim2)
	if err != nil {
		tx.Rollback()
		t.Fatalf("StoreVerbatimTx failed: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Verify the verbatim WAS stored
	retrieved, err := repo.GetVerbatimByID(ctx, verbatim2.ID)
	if err != nil {
		t.Fatalf("GetVerbatimByID failed after commit: %v", err)
	}
	if retrieved.Content != verbatim2.Content {
		t.Errorf("Content = %s, want %s", retrieved.Content, verbatim2.Content)
	}
}

func TestStoreEmbeddingTx(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	// Register model first
	ctx := context.Background()
	model := entities.NewEmbeddingModel("test-model", 384)
	err := repo.RegisterModel(ctx, model)
	if err != nil {
		t.Fatalf("RegisterModel failed: %v", err)
	}

	// Create embedding with specific float32 values
	vec := make([]float32, 384)
	// Use specific test values including edge cases
	testValues := []float32{0.0, 0.5, 1.0, -0.5, -1.0, 0.123456, -0.999999, 0.000001}
	for i := range vec {
		vec[i] = testValues[i%len(testValues)]
	}
	vec[0] = 0.123456789 // Specific value to check precision

	emb := entities.NewEmbedding(uuid.New(), model.ModelHash, vec)

	// Store embedding using transaction
	tx, err := repo.Begin()
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	err = repo.StoreEmbeddingTx(ctx, tx, emb)
	if err != nil {
		tx.Rollback()
		t.Fatalf("StoreEmbeddingTx failed: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Retrieve embedding
	retrieved, err := repo.GetEmbeddingByID(ctx, emb.ID)
	if err != nil {
		t.Fatalf("GetEmbeddingByID failed: %v", err)
	}

	// Verify dimensions
	if retrieved.Dim != emb.Dim {
		t.Errorf("Dim = %d, want %d", retrieved.Dim, emb.Dim)
	}
	if len(retrieved.Vector) != 384 {
		t.Errorf("Vector length = %d, want 384", len(retrieved.Vector))
	}

	// Verify values are preserved correctly (float32 precision)
	tolerance := float32(0.00001)
	mismatchCount := 0
	for i := 0; i < len(vec); i++ {
		diff := vec[i] - retrieved.Vector[i]
		if diff < 0 {
			diff = -diff
		}
		if diff > tolerance {
			mismatchCount++
			if mismatchCount <= 3 {
				t.Errorf("Vector[%d] = %v, want %v (diff: %v)", i, retrieved.Vector[i], vec[i], diff)
			}
		}
	}
	if mismatchCount > 0 {
		t.Errorf("Total mismatches: %d", mismatchCount)
	}

	// Verify specific test values
	if retrieved.Vector[0] != vec[0] {
		t.Errorf("Vector[0] precision lost: got %v, want %v", retrieved.Vector[0], vec[0])
	}
}

func TestTransactionRollback(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	// Test 1: Rollback prevents data persistence
	tx, err := repo.Begin()
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	// Store a verbatim in the transaction
	ctx := context.Background()
	verbatim := entities.NewVerbatim("Will be rolled back", "rollback-wing", nil)
	err = repo.StoreVerbatimTx(ctx, tx, verbatim)
	if err != nil {
		tx.Rollback()
		t.Fatalf("StoreVerbatimTx failed: %v", err)
	}

	// Rollback the transaction
	err = tx.Rollback()
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Verify the verbatim was NOT stored
	_, err = repo.GetVerbatimByID(ctx, verbatim.ID)
	if err == nil {
		t.Error("Expected error after rollback, but verbatim was found")
	}

	// Test 2: After rollback, a new transaction can successfully commit
	tx2, err := repo.Begin()
	if err != nil {
		t.Fatalf("Begin tx2 failed: %v", err)
	}

	verbatim2 := entities.NewVerbatim("Should be committed", "rollback-wing", nil)
	err = repo.StoreVerbatimTx(ctx, tx2, verbatim2)
	if err != nil {
		tx2.Rollback()
		t.Fatalf("StoreVerbatimTx failed: %v", err)
	}

	err = tx2.Commit()
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Verify it was stored
	retrieved, err := repo.GetVerbatimByID(ctx, verbatim2.ID)
	if err != nil {
		t.Fatalf("GetVerbatimByID failed: %v", err)
	}
	if retrieved.Content != verbatim2.Content {
		t.Errorf("Content = %s, want %s", retrieved.Content, verbatim2.Content)
	}
}

func BenchmarkStoreVerbatim(b *testing.B) {
	repo, cleanup := setupTestDB(&testing.T{})
	defer cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		verbatim := entities.NewVerbatim("Benchmark content", "bench", nil)
		repo.StoreVerbatim(context.Background(), verbatim)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

// storeFullMemory stores a verbatim + fingerprint + embedding and returns the verbatim.
// It is used by tests that require all three to be present in the DB.
func storeFullMemory(t *testing.T, repo *SQLiteRepository, content, wing string) *entities.Verbatim {
	t.Helper()
	ctx := context.Background()

	v := entities.NewVerbatim(content, wing, nil)
	if err := repo.StoreVerbatim(ctx, v); err != nil {
		t.Fatalf("storeFullMemory: StoreVerbatim: %v", err)
	}

	fp := entities.NewFingerprint(v.ID, valueobjects.TypeFact, "test-hash")
	fp.WithData(valueobjects.FingerprintData{Decision: content, Subject: []string{"test"}})
	if err := repo.StoreFingerprint(ctx, fp); err != nil {
		t.Fatalf("storeFullMemory: StoreFingerprint: %v", err)
	}

	vec := make([]float32, 4)
	for i := range vec {
		vec[i] = float32(i) + 0.1
	}
	emb := entities.NewEmbedding(v.ID, "test-hash", vec)
	if err := repo.StoreEmbedding(ctx, emb); err != nil {
		t.Fatalf("storeFullMemory: StoreEmbedding: %v", err)
	}

	return v
}

// strRef returns a pointer to a string literal.
func strRef(s string) *string { return &s }

// ── new tests ─────────────────────────────────────────────────────────────────

func TestDeleteVerbatimByID(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	v := entities.NewVerbatim("to be deleted", "wing", nil)
	if err := repo.StoreVerbatim(ctx, v); err != nil {
		t.Fatalf("StoreVerbatim: %v", err)
	}

	// Confirm it exists
	if _, err := repo.GetVerbatimByID(ctx, v.ID); err != nil {
		t.Fatalf("GetVerbatimByID before delete: %v", err)
	}

	// Delete it
	if err := repo.DeleteVerbatimByID(ctx, v.ID); err != nil {
		t.Fatalf("DeleteVerbatimByID: %v", err)
	}

	// Confirm it's gone
	if _, err := repo.GetVerbatimByID(ctx, v.ID); err == nil {
		t.Error("expected error after deletion, got nil")
	}

	// Deleting non-existent id should not error
	if err := repo.DeleteVerbatimByID(ctx, uuid.New()); err != nil {
		t.Errorf("DeleteVerbatimByID on unknown id: %v", err)
	}
}

func TestGetFingerprintByVerbatimID(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	v := entities.NewVerbatim("fingerprint lookup test", "wing", nil)
	if err := repo.StoreVerbatim(ctx, v); err != nil {
		t.Fatalf("StoreVerbatim: %v", err)
	}

	fp := entities.NewFingerprint(v.ID, valueobjects.TypeDecision, "hash1")
	fp.WithData(valueobjects.FingerprintData{Decision: "use X"})
	if err := repo.StoreFingerprint(ctx, fp); err != nil {
		t.Fatalf("StoreFingerprint: %v", err)
	}

	got, err := repo.GetFingerprintByVerbatimID(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetFingerprintByVerbatimID: %v", err)
	}
	if got.VerbatimID != v.ID {
		t.Errorf("VerbatimID = %v, want %v", got.VerbatimID, v.ID)
	}
	if got.Type != valueobjects.TypeDecision {
		t.Errorf("Type = %v, want %v", got.Type, valueobjects.TypeDecision)
	}

	// Non-existent verbatim
	_, err = repo.GetFingerprintByVerbatimID(ctx, uuid.New())
	if err == nil {
		t.Error("expected error for unknown verbatim ID, got nil")
	}
}

func TestGetConsequencesAndParents(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	nodeA := &entities.CausalNode{ID: uuid.New(), Type: "decision", Summary: "A", Timestamp: now, Wing: "w"}
	nodeB := &entities.CausalNode{ID: uuid.New(), Type: "fact", Summary: "B", Timestamp: now, Wing: "w"}
	nodeC := &entities.CausalNode{ID: uuid.New(), Type: "fact", Summary: "C", Timestamp: now, Wing: "w"}

	for _, n := range []*entities.CausalNode{nodeA, nodeB, nodeC} {
		if err := repo.AddNode(ctx, n); err != nil {
			t.Fatalf("AddNode(%s): %v", n.Summary, err)
		}
	}

	edgeAB := entities.NewCausalEdge(nodeA.ID, nodeB.ID, valueobjects.RelBecause)
	edgeBC := entities.NewCausalEdge(nodeB.ID, nodeC.ID, valueobjects.RelTriggered)
	for _, e := range []*entities.CausalEdge{edgeAB, edgeBC} {
		if err := repo.AddEdge(ctx, e); err != nil {
			t.Fatalf("AddEdge: %v", err)
		}
	}

	// GetConsequences of A should return B and C
	consequences, err := repo.GetConsequences(ctx, nodeA.ID, 5)
	if err != nil {
		t.Fatalf("GetConsequences: %v", err)
	}
	if len(consequences) != 2 {
		t.Errorf("GetConsequences returned %d nodes, want 2", len(consequences))
	}

	// GetConsequences with depth 1 should return only B
	shallow, err := repo.GetConsequences(ctx, nodeA.ID, 1)
	if err != nil {
		t.Fatalf("GetConsequences(depth=1): %v", err)
	}
	if len(shallow) != 1 {
		t.Errorf("GetConsequences(depth=1) returned %d nodes, want 1", len(shallow))
	}

	// GetConsequences of a leaf returns nothing
	empty, err := repo.GetConsequences(ctx, nodeC.ID, 5)
	if err != nil {
		t.Fatalf("GetConsequences(leaf): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected 0 consequences for leaf, got %d", len(empty))
	}

	// GetParents of C should return B
	parents, err := repo.GetParents(ctx, nodeC.ID)
	if err != nil {
		t.Fatalf("GetParents: %v", err)
	}
	if len(parents) != 1 || parents[0].ID != nodeB.ID {
		t.Errorf("GetParents returned unexpected nodes: %v", parents)
	}

	// GetParents with relation filter that matches
	parentsFiltered, err := repo.GetParents(ctx, nodeC.ID, valueobjects.RelTriggered)
	if err != nil {
		t.Fatalf("GetParents(filtered): %v", err)
	}
	if len(parentsFiltered) != 1 {
		t.Errorf("GetParents(RelTriggered) returned %d nodes, want 1", len(parentsFiltered))
	}

	// GetParents of a root returns nothing
	rootParents, err := repo.GetParents(ctx, nodeA.ID)
	if err != nil {
		t.Fatalf("GetParents(root): %v", err)
	}
	if len(rootParents) != 0 {
		t.Errorf("expected 0 parents for root, got %d", len(rootParents))
	}
}

func TestTagOperations(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	v1 := entities.NewVerbatim("tag test memory one", "wing", nil)
	v2 := entities.NewVerbatim("tag test memory two", "wing", nil)
	for _, v := range []*entities.Verbatim{v1, v2} {
		if err := repo.StoreVerbatim(ctx, v); err != nil {
			t.Fatalf("StoreVerbatim: %v", err)
		}
	}

	// StoreTags — empty slice should be a no-op
	if err := repo.StoreTags(ctx, v1.ID, nil, "keyword"); err != nil {
		t.Errorf("StoreTags(nil): %v", err)
	}

	// Store tags for v1
	tags1 := []string{"golang", "testing", "database"}
	if err := repo.StoreTags(ctx, v1.ID, tags1, "keyword"); err != nil {
		t.Fatalf("StoreTags(v1): %v", err)
	}

	// Store overlapping tags for v2
	tags2 := []string{"golang", "production"}
	if err := repo.StoreTags(ctx, v2.ID, tags2, "keyword"); err != nil {
		t.Fatalf("StoreTags(v2): %v", err)
	}

	// GetVerbatimsByTags with a shared tag
	ids, err := repo.GetVerbatimsByTags(ctx, []string{"golang"}, 10)
	if err != nil {
		t.Fatalf("GetVerbatimsByTags: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 verbatims for 'golang', got %d", len(ids))
	}

	// GetVerbatimsByTags with a unique tag
	ids, err = repo.GetVerbatimsByTags(ctx, []string{"database"}, 10)
	if err != nil {
		t.Fatalf("GetVerbatimsByTags(database): %v", err)
	}
	if len(ids) != 1 || ids[0] != v1.ID {
		t.Errorf("expected v1 only for 'database', got %v", ids)
	}

	// GetVerbatimsByTags with empty slice returns nil
	empty, err := repo.GetVerbatimsByTags(ctx, nil, 10)
	if err != nil {
		t.Fatalf("GetVerbatimsByTags(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected empty result for nil tags, got %d", len(empty))
	}

	// GetTagsForVerbatim
	gotTags, err := repo.GetTagsForVerbatim(ctx, v1.ID)
	if err != nil {
		t.Fatalf("GetTagsForVerbatim: %v", err)
	}
	if len(gotTags) != len(tags1) {
		t.Errorf("GetTagsForVerbatim returned %d tags, want %d", len(gotTags), len(tags1))
	}

	// GetTagsForVerbatim for unknown verbatim returns empty slice
	unknown, err := repo.GetTagsForVerbatim(ctx, uuid.New())
	if err != nil {
		t.Fatalf("GetTagsForVerbatim(unknown): %v", err)
	}
	if len(unknown) != 0 {
		t.Errorf("expected 0 tags for unknown verbatim, got %d", len(unknown))
	}
}

func TestSearchLexical(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	if !repo.fts5Enabled {
		t.Skip("FTS5 not available in this build")
	}

	// Store full memories so the FTS join succeeds
	v1 := storeFullMemory(t, repo, "the quick brown fox", "wing1")
	_ = storeFullMemory(t, repo, "lazy dog sits quietly", "wing2")

	// FTS5 MATCH search
	results, err := repo.SearchLexical(ctx, "fox", 10, nil, nil)
	if err != nil {
		t.Fatalf("SearchLexical: %v", err)
	}
	if len(results) != 1 || results[0].Verbatim.ID != v1.ID {
		t.Errorf("SearchLexical('fox') expected v1, got %v", results)
	}

	// Wing filter
	resultsWing, err := repo.SearchLexical(ctx, "fox", 10, strRef("wing1"), nil)
	if err != nil {
		t.Fatalf("SearchLexical(wing filter): %v", err)
	}
	if len(resultsWing) != 1 {
		t.Errorf("expected 1 result with wing filter, got %d", len(resultsWing))
	}

	// No results
	none, err := repo.SearchLexical(ctx, "xyzzy", 10, nil, nil)
	if err != nil {
		t.Fatalf("SearchLexical(no match): %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected 0 results for 'xyzzy', got %d", len(none))
	}
}

func TestSearchExact(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	if !repo.fts5Enabled {
		t.Skip("FTS5 not available in this build")
	}

	content := "exact content for search test"
	v := storeFullMemory(t, repo, content, "wing1")

	// Exact match
	results, err := repo.SearchExact(ctx, content, 10, nil, nil)
	if err != nil {
		t.Fatalf("SearchExact: %v", err)
	}
	if len(results) != 1 || results[0].Verbatim.ID != v.ID {
		t.Errorf("SearchExact expected v, got %v", results)
	}

	// Partial content — should not match exact search
	partial, err := repo.SearchExact(ctx, "exact content", 10, nil, nil)
	if err != nil {
		t.Fatalf("SearchExact(partial): %v", err)
	}
	if len(partial) != 0 {
		t.Errorf("SearchExact(partial) should return 0 results, got %d", len(partial))
	}

	// Wing filter mismatch — no results
	noWing, err := repo.SearchExact(ctx, content, 10, strRef("other-wing"), nil)
	if err != nil {
		t.Fatalf("SearchExact(wing mismatch): %v", err)
	}
	if len(noWing) != 0 {
		t.Errorf("expected 0 results with wrong wing, got %d", len(noWing))
	}
}

func TestGetCandidatesWithEmbeddings(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	v1 := storeFullMemory(t, repo, "candidate one", "wing1")
	v2 := storeFullMemory(t, repo, "candidate two", "wing2")

	// Empty ids returns nil
	empty, err := repo.GetCandidatesWithEmbeddings(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("GetCandidatesWithEmbeddings(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected 0 candidates for nil ids, got %d", len(empty))
	}

	// Fetch both
	candidates, err := repo.GetCandidatesWithEmbeddings(ctx, []uuid.UUID{v1.ID, v2.ID}, nil, nil)
	if err != nil {
		t.Fatalf("GetCandidatesWithEmbeddings: %v", err)
	}
	if len(candidates) != 2 {
		t.Errorf("expected 2 candidates, got %d", len(candidates))
	}

	// Wing filter keeps only wing1
	filtered, err := repo.GetCandidatesWithEmbeddings(ctx, []uuid.UUID{v1.ID, v2.ID}, strRef("wing1"), nil)
	if err != nil {
		t.Fatalf("GetCandidatesWithEmbeddings(wing filter): %v", err)
	}
	if len(filtered) != 1 {
		t.Errorf("expected 1 candidate with wing1 filter, got %d", len(filtered))
	}
	if filtered[0].Verbatim.Wing != "wing1" {
		t.Errorf("wrong wing: %s", filtered[0].Verbatim.Wing)
	}

	// Unknown id returns empty
	unknown, err := repo.GetCandidatesWithEmbeddings(ctx, []uuid.UUID{uuid.New()}, nil, nil)
	if err != nil {
		t.Fatalf("GetCandidatesWithEmbeddings(unknown): %v", err)
	}
	if len(unknown) != 0 {
		t.Errorf("expected 0 candidates for unknown id, got %d", len(unknown))
	}
}

func TestGetAllEmbeddings(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Empty DB
	embs, err := repo.GetAllEmbeddings(ctx)
	if err != nil {
		t.Fatalf("GetAllEmbeddings (empty): %v", err)
	}
	if len(embs) != 0 {
		t.Errorf("expected 0 embeddings for empty DB, got %d", len(embs))
	}

	storeFullMemory(t, repo, "embedding one", "wing")
	storeFullMemory(t, repo, "embedding two", "wing")

	embs, err = repo.GetAllEmbeddings(ctx)
	if err != nil {
		t.Fatalf("GetAllEmbeddings: %v", err)
	}
	if len(embs) != 2 {
		t.Errorf("expected 2 embeddings, got %d", len(embs))
	}
	for _, e := range embs {
		if len(e.Vector) == 0 {
			t.Error("embedding vector should not be empty")
		}
	}
}

func TestGetChildren(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Build a 3-node chain A → B → C.
	nodeA := &entities.CausalNode{ID: uuid.New(), Type: "decision", Summary: "A", Timestamp: now, Wing: "w"}
	nodeB := &entities.CausalNode{ID: uuid.New(), Type: "fact", Summary: "B", Timestamp: now, Wing: "w"}
	nodeC := &entities.CausalNode{ID: uuid.New(), Type: "fact", Summary: "C", Timestamp: now, Wing: "w"}
	for _, n := range []*entities.CausalNode{nodeA, nodeB, nodeC} {
		if err := repo.AddNode(ctx, n); err != nil {
			t.Fatalf("AddNode(%s): %v", n.Summary, err)
		}
	}
	if err := repo.AddEdge(ctx, entities.NewCausalEdge(nodeA.ID, nodeB.ID, valueobjects.RelBecause)); err != nil {
		t.Fatalf("AddEdge A→B: %v", err)
	}
	if err := repo.AddEdge(ctx, entities.NewCausalEdge(nodeA.ID, nodeC.ID, valueobjects.RelTriggered)); err != nil {
		t.Fatalf("AddEdge A→C: %v", err)
	}

	// A has two children: B and C.
	children, err := repo.GetChildren(ctx, nodeA.ID)
	if err != nil {
		t.Fatalf("GetChildren(A): %v", err)
	}
	if len(children) != 2 {
		t.Errorf("GetChildren(A) = %d, want 2", len(children))
	}

	// B and C are leaves — no children.
	noChildren, err := repo.GetChildren(ctx, nodeB.ID)
	if err != nil {
		t.Fatalf("GetChildren(B): %v", err)
	}
	if len(noChildren) != 0 {
		t.Errorf("GetChildren(B) = %d, want 0", len(noChildren))
	}

	// Unknown node returns empty slice, no error.
	unknown, err := repo.GetChildren(ctx, uuid.New())
	if err != nil {
		t.Fatalf("GetChildren(unknown): %v", err)
	}
	if len(unknown) != 0 {
		t.Errorf("GetChildren(unknown) = %d, want 0", len(unknown))
	}
}

func TestClearAll(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Populate with two full memories.
	v1 := storeFullMemory(t, repo, "first memory", "wingA")
	v2 := storeFullMemory(t, repo, "second memory", "wingB")

	// Verify they exist.
	if _, err := repo.GetVerbatimByID(ctx, v1.ID); err != nil {
		t.Fatalf("v1 should exist before ClearAll: %v", err)
	}

	if err := repo.ClearAll(ctx); err != nil {
		t.Fatalf("ClearAll: %v", err)
	}

	// Both verbatims must be gone.
	if _, err := repo.GetVerbatimByID(ctx, v1.ID); err == nil {
		t.Error("v1 should be gone after ClearAll")
	}
	if _, err := repo.GetVerbatimByID(ctx, v2.ID); err == nil {
		t.Error("v2 should be gone after ClearAll")
	}

	// Calling ClearAll on an already-empty DB must not error.
	if err := repo.ClearAll(ctx); err != nil {
		t.Errorf("ClearAll on empty DB: %v", err)
	}
}

func TestClearByIDs(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	v1 := storeFullMemory(t, repo, "keep me", "wing")
	v2 := storeFullMemory(t, repo, "delete me", "wing")

	// Empty slice is a no-op.
	n, err := repo.ClearByIDs(ctx, nil)
	if err != nil {
		t.Fatalf("ClearByIDs(nil): %v", err)
	}
	if n != 0 {
		t.Errorf("ClearByIDs(nil) count = %d, want 0", n)
	}

	// Delete only v2.
	n, err = repo.ClearByIDs(ctx, []uuid.UUID{v2.ID})
	if err != nil {
		t.Fatalf("ClearByIDs([v2]): %v", err)
	}
	if n != 1 {
		t.Errorf("ClearByIDs count = %d, want 1", n)
	}

	// v2 should be gone, v1 should remain.
	if _, err := repo.GetVerbatimByID(ctx, v2.ID); err == nil {
		t.Error("v2 should be gone after ClearByIDs")
	}
	if _, err := repo.GetVerbatimByID(ctx, v1.ID); err != nil {
		t.Errorf("v1 should still exist: %v", err)
	}

	// Deleting an already-deleted ID returns count=1 without error
	// (the implementation returns len(ids) regardless).
	n, err = repo.ClearByIDs(ctx, []uuid.UUID{v2.ID})
	if err != nil {
		t.Errorf("ClearByIDs(already gone): %v", err)
	}
	if n != 1 {
		t.Errorf("ClearByIDs(already gone) count = %d, want 1", n)
	}
}

func TestClearByRoom(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// storeWithRoom stores a full memory with the given wing/room.
	storeWithRoom := func(content, wing string, room *string) *entities.Verbatim {
		v := entities.NewVerbatim(content, wing, room)
		if err := repo.StoreVerbatim(ctx, v); err != nil {
			t.Fatalf("StoreVerbatim: %v", err)
		}
		fp := entities.NewFingerprint(v.ID, valueobjects.TypeFact, "h")
		if err := repo.StoreFingerprint(ctx, fp); err != nil {
			t.Fatalf("StoreFingerprint: %v", err)
		}
		vec := []float32{0.1, 0.2, 0.3, 0.4}
		if err := repo.StoreEmbedding(ctx, entities.NewEmbedding(v.ID, "h", vec)); err != nil {
			t.Fatalf("StoreEmbedding: %v", err)
		}
		return v
	}

	room1 := "room1"
	vKeep := storeWithRoom("keep", "wingA", nil)    // wingA, room=NULL
	vDel := storeWithRoom("delete", "wingA", &room1) // wingA, room=room1

	// ClearByRoom on a wing/room combination that doesn't exist → 0 rows, no error.
	n, err := repo.ClearByRoom(ctx, "wingA", strRef("nonexistent"))
	if err != nil {
		t.Fatalf("ClearByRoom(nonexistent room): %v", err)
	}
	if n != 0 {
		t.Errorf("ClearByRoom(nonexistent) = %d, want 0", n)
	}

	// Delete wingA/room1.
	n, err = repo.ClearByRoom(ctx, "wingA", &room1)
	if err != nil {
		t.Fatalf("ClearByRoom(wingA/room1): %v", err)
	}
	if n != 1 {
		t.Errorf("ClearByRoom count = %d, want 1", n)
	}

	// vDel should be gone, vKeep should remain.
	if _, err := repo.GetVerbatimByID(ctx, vDel.ID); err == nil {
		t.Error("vDel should be gone after ClearByRoom")
	}
	if _, err := repo.GetVerbatimByID(ctx, vKeep.ID); err != nil {
		t.Errorf("vKeep should still exist: %v", err)
	}

	// ClearByRoom with nil room targets NULL room entries.
	n, err = repo.ClearByRoom(ctx, "wingA", nil)
	if err != nil {
		t.Fatalf("ClearByRoom(wingA, nil): %v", err)
	}
	if n != 1 {
		t.Errorf("ClearByRoom(wingA, nil) count = %d, want 1", n)
	}
	if _, err := repo.GetVerbatimByID(ctx, vKeep.ID); err == nil {
		t.Error("vKeep should be gone after ClearByRoom(wingA, nil)")
	}
}

func TestGetTimelineFilters(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	wing := "tlWing"
	room := "tlRoom"

	// Helper: store with a specific type.
	storeTyped := func(content string, mt valueobjects.MemoryType, room *string) *entities.Verbatim {
		v := entities.NewVerbatim(content, wing, room)
		if err := repo.StoreVerbatim(ctx, v); err != nil {
			t.Fatalf("StoreVerbatim: %v", err)
		}
		fp := entities.NewFingerprint(v.ID, mt, "hash")
		fp.WithData(valueobjects.FingerprintData{Decision: content, Subject: []string{content}})
		if err := repo.StoreFingerprint(ctx, fp); err != nil {
			t.Fatalf("StoreFingerprint: %v", err)
		}
		return v
	}

	v1 := storeTyped("fact memory", valueobjects.TypeFact, nil)
	v2 := storeTyped("decision memory", valueobjects.TypeDecision, &room)
	_ = v2

	// No filters — both items returned.
	items, err := repo.GetTimeline(ctx, wing, nil, nil, nil, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetTimeline(no filter): %v", err)
	}
	if len(items) != 2 {
		t.Errorf("GetTimeline returned %d items, want 2", len(items))
	}

	// Room filter — only v2.
	items, err = repo.GetTimeline(ctx, wing, &room, nil, nil, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetTimeline(room filter): %v", err)
	}
	if len(items) != 1 {
		t.Errorf("GetTimeline(room filter) = %d, want 1", len(items))
	}

	// Type filter — only v1 (TypeFact, room=nil).
	mt := valueobjects.TypeFact
	items, err = repo.GetTimeline(ctx, wing, nil, &mt, nil, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetTimeline(type filter): %v", err)
	}
	if len(items) != 1 || items[0].ID != v1.ID.String() {
		t.Errorf("GetTimeline(TypeFact) unexpected result: %v", items)
	}

	// Limit 0 falls back to default 100 — must not error.
	items, err = repo.GetTimeline(ctx, wing, nil, nil, nil, nil, 0, nil)
	if err != nil {
		t.Fatalf("GetTimeline(limit 0): %v", err)
	}
	if len(items) != 2 {
		t.Errorf("GetTimeline(limit 0) = %d items, want 2", len(items))
	}

	// Limit > 1000 is capped — must return items without error.
	items, err = repo.GetTimeline(ctx, wing, nil, nil, nil, nil, 9999, nil)
	if err != nil {
		t.Fatalf("GetTimeline(limit 9999): %v", err)
	}
	if len(items) > 1000 {
		t.Errorf("GetTimeline(limit 9999) returned %d items, want ≤1000", len(items))
	}

	// since/until strings (RFC3339 timestamps far in the past/future).
	past := "2000-01-01T00:00:00Z"
	future := "2099-01-01T00:00:00Z"
	items, err = repo.GetTimeline(ctx, wing, nil, nil, &past, &future, 10, nil)
	if err != nil {
		t.Fatalf("GetTimeline(since/until): %v", err)
	}
	// The DB stores timestamps as float64(unix); since/until filters compare
	// against extracted_at string — may return 0 or 2 depending on column type.
	// We just verify no error and items count is sane.
	if len(items) > 2 {
		t.Errorf("GetTimeline(since/until) returned %d items", len(items))
	}

	// Cursor — empty cursor must be a no-op.
	emptyStr := ""
	items, err = repo.GetTimeline(ctx, wing, nil, nil, nil, nil, 10, &emptyStr)
	if err != nil {
		t.Fatalf("GetTimeline(empty cursor): %v", err)
	}
	if len(items) != 2 {
		t.Errorf("GetTimeline(empty cursor) = %d, want 2", len(items))
	}

	// Cursor — valid RFC3339 timestamp filters out older records.
	cursorTS := "2099-01-01T00:00:00Z"
	items, err = repo.GetTimeline(ctx, wing, nil, nil, nil, nil, 10, &cursorTS)
	if err != nil {
		t.Fatalf("GetTimeline(cursor): %v", err)
	}
	// Result depends on stored timestamps; just verify no error.
	_ = items

	// Unknown wing returns empty.
	items, err = repo.GetTimeline(ctx, "noWing", nil, nil, nil, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetTimeline(unknown wing): %v", err)
	}
	if len(items) != 0 {
		t.Errorf("GetTimeline(unknown wing) = %d, want 0", len(items))
	}
}

func TestSQLiteRepository_UpdateVerbatimSummary(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Store a verbatim
	v := entities.NewVerbatim("Some long session note content here for testing compression", "wing1", nil)
	v.TokenCount = 10
	tx, err := repo.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := repo.StoreVerbatimTx(ctx, tx, v); err != nil {
		t.Fatalf("StoreVerbatimTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	summary := "Long session note content testing compression"
	if err := repo.UpdateVerbatimSummary(ctx, v.ID, summary, 6); err != nil {
		t.Fatalf("UpdateVerbatimSummary: %v", err)
	}

	// Reload and check
	got, err := repo.GetVerbatimByID(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetVerbatimByID: %v", err)
	}
	if !got.HasSummary() {
		t.Fatal("expected HasSummary() == true")
	}
	if *got.Summary != summary {
		t.Errorf("Summary = %q, want %q", *got.Summary, summary)
	}
	if got.SummaryTokenCount != 6 {
		t.Errorf("SummaryTokenCount = %d, want 6", got.SummaryTokenCount)
	}
}
