package store

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/benoitpetit/mira/types"
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

func TestGetEmbeddingModels(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Initially empty
	models, err := store.GetEmbeddingModels()
	if err != nil {
		t.Fatalf("GetEmbeddingModels() error = %v", err)
	}
	if len(models) != 0 {
		t.Errorf("Expected 0 models initially, got %d", len(models))
	}

	// Register some models
	model1 := &types.EmbeddingModel{
		ModelHash: "hash1abc123",
		ModelName: "model-test-1",
		Dimension: 384,
		CreatedAt: time.Now(),
		Metadata:  map[string]any{"source": "test"},
	}
	model2 := &types.EmbeddingModel{
		ModelHash: "hash2def456",
		ModelName: "model-test-2",
		Dimension: 768,
		CreatedAt: time.Now(),
		Metadata:  map[string]any{"source": "test2"},
	}

	if err := store.RegisterModel(model1); err != nil {
		t.Fatalf("RegisterModel() error = %v", err)
	}
	if err := store.RegisterModel(model2); err != nil {
		t.Fatalf("RegisterModel() error = %v", err)
	}

	// Verify
	models, err = store.GetEmbeddingModels()
	if err != nil {
		t.Fatalf("GetEmbeddingModels() error = %v", err)
	}
	if len(models) != 2 {
		t.Errorf("Expected 2 models, got %d", len(models))
	}

	// Check that hashes are present
	hashMap := make(map[string]bool)
	for _, m := range models {
		hashMap[m] = true
	}
	if !hashMap["hash1abc123"] || !hashMap["hash2def456"] {
		t.Error("Expected model hashes not found")
	}
}

func TestRegisterModelDuplicate(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	model := &types.EmbeddingModel{
		ModelHash: "duplicate-hash",
		ModelName: "model-test",
		Dimension: 384,
		CreatedAt: time.Now(),
	}

	// First registration
	if err := store.RegisterModel(model); err != nil {
		t.Fatalf("First RegisterModel() error = %v", err)
	}

	// Second registration (should be ignored due to INSERT OR IGNORE)
	if err := store.RegisterModel(model); err != nil {
		t.Fatalf("Second RegisterModel() error = %v", err)
	}

	models, err := store.GetEmbeddingModels()
	if err != nil {
		t.Fatalf("GetEmbeddingModels() error = %v", err)
	}
	if len(models) != 1 {
		t.Errorf("Expected 1 model (duplicate ignored), got %d", len(models))
	}
}

func TestGetVerbatimNotFound(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// UUID that doesn't exist
	nonExistentID := uuid.MustParse("99999999-9999-9999-9999-999999999999")

	_, err := store.GetVerbatim(nonExistentID)
	if err == nil {
		t.Error("Expected error for non-existent verbatim")
	}
	if err.Error() != "verbatim not found" {
		t.Errorf("Expected 'verbatim not found' error, got %q", err.Error())
	}
}

func TestGetEmbeddingNotFound(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	nonExistentID := uuid.MustParse("99999999-9999-9999-9999-999999999999")

	_, err := store.GetEmbedding(nonExistentID)
	if err == nil {
		t.Error("Expected error for non-existent embedding")
	}
	if err.Error() != "embedding not found" {
		t.Errorf("Expected 'embedding not found' error, got %q", err.Error())
	}
}

func TestSearchCandidatesFilters(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Register model
	model := &types.EmbeddingModel{
		ModelHash: "test-model",
		ModelName: "test",
		Dimension: 384,
		CreatedAt: time.Now(),
	}
	store.RegisterModel(model)

	// Create verbatims in different wings/rooms
	wings := []string{"backend", "frontend", "devops"}
	rooms := []*string{strPtr("team-a"), strPtr("team-b"), nil}

	for _, wing := range wings {
		for _, room := range rooms {
			v := &types.Verbatim{
				ID:        uuid.New(),
				Content:   "Content for " + wing,
				TokenCount: 10,
				CreatedAt: time.Now(),
				Wing:      wing,
				Room:      room,
			}

			fp := &types.Fingerprint{
				ID:            v.ID,
				VerbatimID:    v.ID,
				Type:          types.TypeDecision,
				ExtractedAt:   time.Now(),
				Data:          types.FingerprintData{VerbatimRef: "T0:" + v.ID.String()},
				TokenEstimate: 10,
				ModelHash:     "test-model",
			}

			vector := make([]float32, 384)
			emb := &types.Embedding{
				ID:         v.ID,
				ModelHash:  "test-model",
				Dim:        384,
				Vector:     vector,
				Normalized: true,
				CreatedAt:  time.Now(),
			}

			tx, _ := store.BeginTx()
			store.StoreVerbatimTx(tx, v)
			store.StoreFingerprintTx(tx, fp)
			store.StoreEmbeddingTx(tx, emb)
			tx.Commit()
		}
	}

	tests := []struct {
		name      string
		wing      *string
		room      *string
		wantCount int
	}{
		{
			name:      "no filters",
			wing:      nil,
			room:      nil,
			wantCount: 9,
		},
		{
			name:      "filter by wing backend",
			wing:      strPtr("backend"),
			room:      nil,
			wantCount: 3,
		},
		{
			name:      "filter by wing frontend",
			wing:      strPtr("frontend"),
			room:      nil,
			wantCount: 3,
		},
		{
			name:      "filter by non-existent wing",
			wing:      strPtr("nonexistent"),
			room:      nil,
			wantCount: 0,
		},
		{
			name:      "filter by wing and room",
			wing:      strPtr("backend"),
			room:      strPtr("team-a"),
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidates, err := store.SearchCandidates(tt.wing, tt.room, 100)
			if err != nil {
				t.Fatalf("SearchCandidates() error = %v", err)
			}
			if len(candidates) != tt.wantCount {
				t.Errorf("Expected %d candidates, got %d", tt.wantCount, len(candidates))
			}
		})
	}
}

func TestSearchCandidatesLimit(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Register model
	model := &types.EmbeddingModel{
		ModelHash: "test-model",
		ModelName: "test",
		Dimension: 384,
		CreatedAt: time.Now(),
	}
	store.RegisterModel(model)

	// Create 10 verbatims
	for i := 0; i < 10; i++ {
		v := &types.Verbatim{
			ID:        uuid.New(),
			Content:   "Content",
			TokenCount: 10,
			CreatedAt: time.Now().Add(-time.Duration(i) * time.Minute),
			Wing:      "test",
		}

		fp := &types.Fingerprint{
			ID:            v.ID,
			VerbatimID:    v.ID,
			Type:          types.TypeDecision,
			ExtractedAt:   v.CreatedAt,
			Data:          types.FingerprintData{VerbatimRef: "T0:" + v.ID.String()},
			TokenEstimate: 10,
			ModelHash:     "test-model",
		}

		vector := make([]float32, 384)
		emb := &types.Embedding{
			ID:         v.ID,
			ModelHash:  "test-model",
			Dim:        384,
			Vector:     vector,
			Normalized: true,
			CreatedAt:  time.Now(),
		}

		tx, _ := store.BeginTx()
		store.StoreVerbatimTx(tx, v)
		store.StoreFingerprintTx(tx, fp)
		store.StoreEmbeddingTx(tx, emb)
		tx.Commit()
	}

	// Test with different limits
	tests := []struct {
		limit     int
		wantCount int
	}{
		{5, 5},
		{3, 3},
		{20, 10}, // More than available
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("limit_%d", tt.limit), func(t *testing.T) {
			candidates, err := store.SearchCandidates(nil, nil, tt.limit)
			if err != nil {
				t.Fatalf("SearchCandidates() error = %v", err)
			}
			if len(candidates) != tt.wantCount {
				t.Errorf("Expected %d candidates with limit %d, got %d", tt.wantCount, tt.limit, len(candidates))
			}
		})
	}
}

func TestDB(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	db := store.DB()
	if db == nil {
		t.Error("DB() should return non-nil database")
	}

	// Verify connection is valid
	if err := db.Ping(); err != nil {
		t.Errorf("DB().Ping() error = %v", err)
	}
}

func TestTransactionRollback(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Begin transaction
	tx, err := store.BeginTx()
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}

	// Insert data
	v := &types.Verbatim{
		ID:        uuid.New(),
		Content:   "Test content",
		TokenCount: 10,
		CreatedAt: time.Now(),
		Wing:      "test",
	}
	if err := store.StoreVerbatimTx(tx, v); err != nil {
		t.Fatalf("StoreVerbatimTx() error = %v", err)
	}

	// Rollback instead of commit
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	// Verify data was not inserted
	stats, err := store.GetStats()
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if stats.VerbatimCount != 0 {
		t.Error("Expected 0 verbatims after rollback")
	}
}

func TestGetTimeline(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Register model
	model := &types.EmbeddingModel{
		ModelHash: "test-model",
		ModelName: "test",
		Dimension: 384,
		CreatedAt: time.Now(),
	}
	store.RegisterModel(model)

	wing := "backend"
	room := "db-team"

	now := time.Now()

	// Create multiple fingerprints
	for i := 0; i < 5; i++ {
		v := &types.Verbatim{
			ID:        uuid.New(),
			Content:   "Test content",
			TokenCount: 10,
			CreatedAt: now.Add(-time.Duration(i) * time.Hour),
			Wing:      wing,
			Room:      &room,
		}

		fp := &types.Fingerprint{
			ID:            v.ID,
			VerbatimID:    v.ID,
			Type:          types.TypeDecision,
			ExtractedAt:   v.CreatedAt,
			Data: types.FingerprintData{
				Subject:     []string{"Topic " + string(rune('A'+i))},
				Decision:    "Decision " + string(rune('A'+i)),
				VerbatimRef: "T0:" + v.ID.String(),
			},
			TokenEstimate: 10,
			ModelHash:     "test-model",
		}

		vector := make([]float32, 384)
		emb := &types.Embedding{
			ID:         v.ID,
			ModelHash:  "test-model",
			Dim:        384,
			Vector:     vector,
			Normalized: true,
			CreatedAt:  now,
		}

		tx, _ := store.BeginTx()
		store.StoreVerbatimTx(tx, v)
		store.StoreFingerprintTx(tx, fp)
		store.StoreEmbeddingTx(tx, emb)
		tx.Commit()
	}

	// Test: Timeline without filters
	t.Run("no filters", func(t *testing.T) {
		items, err := store.GetTimeline(wing, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("GetTimeline() error = %v", err)
		}
		if len(items) != 5 {
			t.Errorf("Expected 5 items, got %d", len(items))
		}
		// Check order (DESC)
		for i := 1; i < len(items); i++ {
			if items[i].Timestamp.After(items[i-1].Timestamp) {
				t.Error("Items should be sorted by timestamp DESC")
			}
		}
	})

	// Test: Filter by room
	t.Run("filter by room", func(t *testing.T) {
		wrongRoom := "wrong-room"
		items, err := store.GetTimeline(wing, &wrongRoom, nil, nil, nil)
		if err != nil {
			t.Fatalf("GetTimeline() error = %v", err)
		}
		if len(items) != 0 {
			t.Errorf("Expected 0 items for wrong room, got %d", len(items))
		}

		items, err = store.GetTimeline(wing, &room, nil, nil, nil)
		if err != nil {
			t.Fatalf("GetTimeline() error = %v", err)
		}
		if len(items) != 5 {
			t.Errorf("Expected 5 items for correct room, got %d", len(items))
		}
	})

	// Test: Filter by type
	t.Run("filter by type", func(t *testing.T) {
		memType := string(types.TypeDecision)
		items, err := store.GetTimeline(wing, nil, &memType, nil, nil)
		if err != nil {
			t.Fatalf("GetTimeline() error = %v", err)
		}
		if len(items) != 5 {
			t.Errorf("Expected 5 items of type decision, got %d", len(items))
		}
	})

	// Test: Filter by time range
	t.Run("filter by time range", func(t *testing.T) {
		since := now.Add(-3 * time.Hour)
		until := now.Add(-1 * time.Hour)

		items, err := store.GetTimeline(wing, nil, nil, &since, &until)
		if err != nil {
			t.Fatalf("GetTimeline() error = %v", err)
		}
		// Should have 3 items (1h, 2h and 3h ago - boundary is inclusive)
		if len(items) != 3 {
			t.Errorf("Expected 3 items in time range, got %d", len(items))
		}
	})
}

func TestClose(t *testing.T) {
	store, _ := setupTestDB(&testing.T{})
	// Don't use cleanup - we close manually

	// Close the store
	if err := store.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Operations after close should fail
	_, err := store.GetStats()
	if err == nil {
		t.Error("Expected error when using closed store")
	}
}
