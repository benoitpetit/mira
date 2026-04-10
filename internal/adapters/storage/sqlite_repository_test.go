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
	timeline, err := repo.GetTimeline(ctx, "timeline-wing", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("GetTimeline failed: %v", err)
	}

	if len(timeline) != 1 {
		t.Errorf("Timeline length = %d, want 1", len(timeline))
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
