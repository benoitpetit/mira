package store

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"mira/types"
)

func setupTestDB(t *testing.T) (*Store, func()) {
	tmpDir, err := os.MkdirTemp("", "mira-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := tmpDir + "/test.db"
	store, err := New(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create store: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

func TestStoreVerbatim(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	verbatim := &types.Verbatim{
		ID:        uuid.New(),
		Content:   "Test content for verbatim storage",
		TokenCount: 10,
		CreatedAt: time.Now(),
		Wing:      "test-wing",
		Room:      strPtr("test-room"),
		Metadata:  map[string]any{"key": "value"},
	}

	tx, err := store.BeginTx()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	if err := store.StoreVerbatimTx(tx, verbatim); err != nil {
		tx.Rollback()
		t.Fatalf("Failed to store verbatim: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Retrieve and verify
	retrieved, err := store.GetVerbatim(verbatim.ID)
	if err != nil {
		t.Fatalf("Failed to get verbatim: %v", err)
	}

	if retrieved.ID != verbatim.ID {
		t.Errorf("ID mismatch: got %v, want %v", retrieved.ID, verbatim.ID)
	}
	if retrieved.Content != verbatim.Content {
		t.Errorf("Content mismatch: got %v, want %v", retrieved.Content, verbatim.Content)
	}
	if retrieved.Wing != verbatim.Wing {
		t.Errorf("Wing mismatch: got %v, want %v", retrieved.Wing, verbatim.Wing)
	}
}

func TestStoreFingerprint(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Register model first
	model := &types.EmbeddingModel{
		ModelHash: "test-model",
		ModelName: "test",
		Dimension: 384,
		CreatedAt: time.Now(),
	}
	store.RegisterModel(model)

	// Store verbatim, fingerprint and embedding together
	verbatim := &types.Verbatim{
		ID:        uuid.New(),
		Content:   "Test decision content",
		TokenCount: 15,
		CreatedAt: time.Now(),
		Wing:      "backend",
	}

	fp := &types.Fingerprint{
		ID:            verbatim.ID,
		VerbatimID:    verbatim.ID,
		Type:          types.TypeDecision,
		ExtractedAt:   time.Now(),
		Entities:      []string{"PostgreSQL", "API"},
		Subjects:      []string{"database"},
		Decision:      strPtr("Use PostgreSQL"),
		Data: types.FingerprintData{
			ID:          verbatim.ID.String(),
			Type:        "decision",
			Date:        time.Now().Format(time.RFC3339),
			Entities:    []string{"PostgreSQL"},
			Subject:     []string{"database"},
			Decision:    "Use PostgreSQL",
			VerbatimRef: "T0:" + verbatim.ID.String(),
		},
		FactCount:     3,
		TokenEstimate: 50,
		ModelHash:     "test-model",
	}

	vector := make([]float32, 384)
	for i := range vector {
		vector[i] = float32(i) / 384.0
	}
	emb := &types.Embedding{
		ID:         verbatim.ID,
		ModelHash:  "test-model",
		Dim:        384,
		Vector:     vector,
		Normalized: true,
		CreatedAt:  time.Now(),
	}

	tx, _ := store.BeginTx()
	store.StoreVerbatimTx(tx, verbatim)
	store.StoreFingerprintTx(tx, fp)
	store.StoreEmbeddingTx(tx, emb)
	tx.Commit()

	// Verify via SearchCandidates
	candidates, err := store.SearchCandidates(strPtr("backend"), nil, 10)
	if err != nil {
		t.Fatalf("Failed to search candidates: %v", err)
	}

	if len(candidates) != 1 {
		t.Errorf("Expected 1 candidate, got %d", len(candidates))
	}
}

func TestStoreEmbedding(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Register model first
	model := &types.EmbeddingModel{
		ModelHash: "test-model-123",
		ModelName: "test-model",
		Dimension: 384,
		CreatedAt: time.Now(),
	}
	if err := store.RegisterModel(model); err != nil {
		t.Fatalf("Failed to register model: %v", err)
	}

	// Store verbatim
	verbatim := &types.Verbatim{
		ID:        uuid.New(),
		Content:   "Test content",
		TokenCount: 5,
		CreatedAt: time.Now(),
		Wing:      "test",
	}

	tx, _ := store.BeginTx()
	store.StoreVerbatimTx(tx, verbatim)
	tx.Commit()

	// Store embedding
	vector := make([]float32, 384)
	for i := range vector {
		vector[i] = float32(i) / 384.0
	}

	emb := &types.Embedding{
		ID:         verbatim.ID,
		ModelHash:  model.ModelHash,
		Dim:        384,
		Vector:     vector,
		Normalized: true,
		CreatedAt:  time.Now(),
	}

	tx, _ = store.BeginTx()
	if err := store.StoreEmbeddingTx(tx, emb); err != nil {
		tx.Rollback()
		t.Fatalf("Failed to store embedding: %v", err)
	}
	tx.Commit()

	// Retrieve and verify
	retrieved, err := store.GetEmbedding(verbatim.ID)
	if err != nil {
		t.Fatalf("Failed to get embedding: %v", err)
	}

	if retrieved.ID != emb.ID {
		t.Errorf("ID mismatch")
	}
	if retrieved.Dim != emb.Dim {
		t.Errorf("Dim mismatch: got %d, want %d", retrieved.Dim, emb.Dim)
	}
}

func TestGetStats(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Initially empty
	stats, err := store.GetStats()
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	if stats.VerbatimCount != 0 {
		t.Errorf("Expected 0 verbatims, got %d", stats.VerbatimCount)
	}

	// Add some data
	for i := 0; i < 5; i++ {
		verbatim := &types.Verbatim{
			ID:        uuid.New(),
			Content:   "Test content",
			TokenCount: 10,
			CreatedAt: time.Now(),
			Wing:      "test-wing",
		}
		tx, _ := store.BeginTx()
		store.StoreVerbatimTx(tx, verbatim)
		tx.Commit()
	}

	stats, _ = store.GetStats()
	if stats.VerbatimCount != 5 {
		t.Errorf("Expected 5 verbatims, got %d", stats.VerbatimCount)
	}
}

func TestArchiveOldMemories(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Add old session_note (should be archived)
	oldSessionNote := &types.Verbatim{
		ID:        uuid.New(),
		Content:   "Old session note",
		TokenCount: 5,
		CreatedAt: time.Now().AddDate(0, 0, -31), // 31 days ago
		Wing:      "test",
	}

	tx, _ := store.BeginTx()
	store.StoreVerbatimTx(tx, oldSessionNote)
	tx.Commit()

	// Add fingerprint for session_note
	fp := &types.Fingerprint{
		ID:            oldSessionNote.ID,
		VerbatimID:    oldSessionNote.ID,
		Type:          types.TypeSessionNote,
		ExtractedAt:   time.Now().AddDate(0, 0, -31),
		Data:          types.FingerprintData{VerbatimRef: "T0:" + oldSessionNote.ID.String()},
		TokenEstimate: 5,
		ModelHash:     "abc",
	}
	tx, _ = store.BeginTx()
	store.StoreFingerprintTx(tx, fp)
	tx.Commit()

	// Archive
	result, err := store.ArchiveOldMemories()
	if err != nil {
		t.Fatalf("Failed to archive: %v", err)
	}

	if result.SessionNotes != 1 {
		t.Errorf("Expected 1 session note archived, got %d", result.SessionNotes)
	}
}

func BenchmarkStoreVerbatim(b *testing.B) {
	store, cleanup := setupTestDB(&testing.T{})
	defer cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		verbatim := &types.Verbatim{
			ID:        uuid.New(),
			Content:   "Benchmark content for verbatim storage testing",
			TokenCount: 10,
			CreatedAt: time.Now(),
			Wing:      "benchmark",
		}

		tx, _ := store.BeginTx()
		store.StoreVerbatimTx(tx, verbatim)
		tx.Commit()
	}
}

func BenchmarkStoreCompletePipeline(b *testing.B) {
	store, cleanup := setupTestDB(&testing.T{})
	defer cleanup()

	// Register model
	model := &types.EmbeddingModel{
		ModelHash: "bench-model",
		ModelName: "bench",
		Dimension: 384,
		CreatedAt: time.Now(),
	}
	store.RegisterModel(model)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx, _ := store.BeginTx()

		verbatim := &types.Verbatim{
			ID:        uuid.New(),
			Content:   "Complete pipeline benchmark test",
			TokenCount: 10,
			CreatedAt: time.Now(),
			Wing:      "benchmark",
		}
		store.StoreVerbatimTx(tx, verbatim)

		fp := &types.Fingerprint{
			ID:            verbatim.ID,
			VerbatimID:    verbatim.ID,
			Type:          types.TypeSessionNote,
			ExtractedAt:   time.Now(),
			Entities:      []string{"test"},
			Data:          types.FingerprintData{VerbatimRef: "T0:" + verbatim.ID.String()},
			TokenEstimate: 10,
			ModelHash:     "bench-model",
		}
		store.StoreFingerprintTx(tx, fp)

		vector := make([]float32, 384)
		emb := &types.Embedding{
			ID:         verbatim.ID,
			ModelHash:  "bench-model",
			Dim:        384,
			Vector:     vector,
			Normalized: true,
			CreatedAt:  time.Now(),
		}
		store.StoreEmbeddingTx(tx, emb)

		tx.Commit()
	}
}

func strPtr(s string) *string {
	return &s
}
