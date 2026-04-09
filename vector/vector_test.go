package vector

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/benoitpetit/mira/store"
	"github.com/benoitpetit/mira/types"
	"github.com/google/uuid"
)

// --- CosineSimilarity ---

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float64
		tol      float64
	}{
		{
			name:     "identical vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 1.0,
			tol:      0.0001,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{0, 1, 0},
			expected: 0.0,
			tol:      0.0001,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{-1, 0, 0},
			expected: -1.0,
			tol:      0.0001,
		},
		{
			name:     "45 degree angle",
			a:        []float32{1, 0, 0},
			b:        []float32{0.7071, 0.7071, 0},
			expected: 0.7071,
			tol:      0.001,
		},
		{
			name:     "different lengths",
			a:        []float32{1, 0},
			b:        []float32{1, 0, 0},
			expected: 0.0,
			tol:      0.0001,
		},
		{
			name:     "zero vector a",
			a:        []float32{0, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 0.0,
			tol:      0.0001,
		},
		{
			name:     "both zero vectors",
			a:        []float32{0, 0, 0},
			b:        []float32{0, 0, 0},
			expected: 0.0,
			tol:      0.0001,
		},
		{
			name:     "empty vectors",
			a:        []float32{},
			b:        []float32{},
			expected: 0.0,
			tol:      0.0001,
		},
		{
			name:     "nil vectors",
			a:        nil,
			b:        nil,
			expected: 0.0,
			tol:      0.0001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cosineSimilarity(tt.a, tt.b)
			if math.Abs(result-tt.expected) > tt.tol {
				t.Errorf("cosineSimilarity() = %v, want %v (tol %v)", result, tt.expected, tt.tol)
			}
		})
	}
}

// --- Helper: create a test store with schema ---

func setupTestStore(t *testing.T) (*store.Store, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "mira-vector-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	dbPath := tmpDir + "/test.db"

	st, err := store.NewWithOptions(dbPath, store.StoreOptions{})
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create store: %v", err)
	}

	// Register a test model
	model := &types.EmbeddingModel{
		ModelHash: "test-hash-1234",
		ModelName: "test-model",
		Dimension: 3,
		CreatedAt: time.Now(),
	}
	if err := st.RegisterModel(model); err != nil {
		t.Logf("Warning: failed to register model: %v", err)
	}

	cleanup := func() {
		st.Close()
		os.RemoveAll(tmpDir)
	}
	return st, cleanup
}

// storeTestData inserts a verbatim + fingerprint + embedding via transactions.
func storeTestData(t *testing.T, st *store.Store, wing string, room *string, vec []float32) uuid.UUID {
	t.Helper()
	id := uuid.New()

	v := &types.Verbatim{
		ID:         id,
		Content:    "Test content for " + id.String()[:8],
		TokenCount: 10,
		Wing:       wing,
		Room:       room,
		CreatedAt:  time.Now(),
	}

	fp := &types.Fingerprint{
		ID:            id,
		VerbatimID:    id,
		Type:          types.TypeFact,
		ExtractedAt:   time.Now(),
		Entities:      []string{"test"},
		Subjects:      []string{"testing"},
		Data:          types.FingerprintData{ID: id.String(), Type: "fact"},
		FactCount:     1,
		TokenEstimate: 5,
		ModelHash:     "test-hash-1234",
	}

	emb := &types.Embedding{
		ID:         id,
		ModelHash:  "test-hash-1234",
		Dim:        len(vec),
		Vector:     vec,
		Normalized: true,
		CreatedAt:  time.Now(),
	}

	tx, err := st.BeginTx()
	if err != nil {
		t.Fatalf("BeginTx() error: %v", err)
	}
	defer tx.Rollback()

	if err := st.StoreVerbatimTx(tx, v); err != nil {
		t.Fatalf("StoreVerbatimTx() error: %v", err)
	}
	if err := st.StoreFingerprintTx(tx, fp); err != nil {
		t.Fatalf("StoreFingerprintTx() error: %v", err)
	}
	if err := st.StoreEmbeddingTx(tx, emb); err != nil {
		t.Fatalf("StoreEmbeddingTx() error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error: %v", err)
	}

	return id
}

func strPtr(s string) *string { return &s }

// --- SQLiteAdapter.Search ---

func TestSQLiteAdapter_Search_Basic(t *testing.T) {
	st, cleanup := setupTestStore(t)
	defer cleanup()

	// Insert test data with known vectors
	storeTestData(t, st, "backend", nil, []float32{1.0, 0.0, 0.0})
	storeTestData(t, st, "backend", nil, []float32{0.9, 0.1, 0.0})
	storeTestData(t, st, "frontend", nil, []float32{0.0, 1.0, 0.0})

	adapter := NewSQLiteAdapter(st)

	// Search with query vector close to first two
	results, err := adapter.Search([]float32{1.0, 0.0, 0.0}, 10, strPtr("backend"), nil)
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Expected at least one result")
	}

	// All results should be from "backend" wing (via Verbatim)
	for _, r := range results {
		if r.Verbatim == nil {
			t.Error("Expected Verbatim to be populated")
			continue
		}
		if r.Verbatim.Wing != "backend" {
			t.Errorf("Expected wing 'backend', got '%s'", r.Verbatim.Wing)
		}
	}
}

func TestSQLiteAdapter_Search_WingFilter(t *testing.T) {
	st, cleanup := setupTestStore(t)
	defer cleanup()

	storeTestData(t, st, "backend", nil, []float32{1.0, 0.0, 0.0})
	storeTestData(t, st, "frontend", nil, []float32{1.0, 0.0, 0.0})

	adapter := NewSQLiteAdapter(st)

	// Search only backend
	backendResults, err := adapter.Search([]float32{1.0, 0.0, 0.0}, 10, strPtr("backend"), nil)
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	// Search only frontend
	frontendResults, err := adapter.Search([]float32{1.0, 0.0, 0.0}, 10, strPtr("frontend"), nil)
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	// Each filtered set should have results
	if len(backendResults) == 0 {
		t.Error("Expected backend results")
	}
	if len(frontendResults) == 0 {
		t.Error("Expected frontend results")
	}
}

func TestSQLiteAdapter_Search_RoomFilter(t *testing.T) {
	st, cleanup := setupTestStore(t)
	defer cleanup()

	storeTestData(t, st, "backend", strPtr("auth"), []float32{1.0, 0.0, 0.0})
	storeTestData(t, st, "backend", strPtr("db"), []float32{1.0, 0.0, 0.0})

	adapter := NewSQLiteAdapter(st)

	results, err := adapter.Search([]float32{1.0, 0.0, 0.0}, 10, strPtr("backend"), strPtr("auth"))
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Expected at least one result for room filter")
	}
}

func TestSQLiteAdapter_Search_Limit(t *testing.T) {
	st, cleanup := setupTestStore(t)
	defer cleanup()

	for i := 0; i < 10; i++ {
		v := float32(i) / 10.0
		storeTestData(t, st, "test", nil, []float32{1.0 - v, v, 0.0})
	}

	adapter := NewSQLiteAdapter(st)

	results, err := adapter.Search([]float32{1.0, 0.0, 0.0}, 3, strPtr("test"), nil)
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	if len(results) > 3 {
		t.Errorf("Expected at most 3 results, got %d", len(results))
	}
}

func TestSQLiteAdapter_Search_EmptyStore(t *testing.T) {
	st, cleanup := setupTestStore(t)
	defer cleanup()

	adapter := NewSQLiteAdapter(st)

	results, err := adapter.Search([]float32{1.0, 0.0, 0.0}, 10, nil, nil)
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results from empty store, got %d", len(results))
	}
}

func TestSQLiteAdapter_Search_SortedBySimilarity(t *testing.T) {
	st, cleanup := setupTestStore(t)
	defer cleanup()

	// Store vectors with known similarity order to query [1,0,0]
	storeTestData(t, st, "test", nil, []float32{0.0, 1.0, 0.0})   // orthogonal = 0
	storeTestData(t, st, "test", nil, []float32{0.5, 0.5, 0.0})   // ~0.7
	storeTestData(t, st, "test", nil, []float32{0.95, 0.05, 0.0}) // ~0.99

	adapter := NewSQLiteAdapter(st)
	results, err := adapter.Search([]float32{1.0, 0.0, 0.0}, 10, strPtr("test"), nil)
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	if len(results) < 2 {
		t.Fatalf("Expected at least 2 results, got %d", len(results))
	}

	// Results should be sorted by descending similarity
	// The first result's embedding should be more similar to [1,0,0] than the last
	first := cosineSimilarity(results[0].Embedding, []float32{1.0, 0.0, 0.0})
	last := cosineSimilarity(results[len(results)-1].Embedding, []float32{1.0, 0.0, 0.0})
	if first < last {
		t.Errorf("Results not sorted by similarity: first=%.4f, last=%.4f", first, last)
	}
}

// --- SQLiteOverlapCache ---

func TestSQLiteOverlapCache_SetAndGet(t *testing.T) {
	st, cleanup := setupTestStore(t)
	defer cleanup()

	cache := NewSQLiteOverlapCache(st.DB())

	idA := uuid.New()
	idB := uuid.New()

	// Initially empty
	_, found := cache.Get(idA, idB)
	if found {
		t.Error("Expected cache miss on empty cache")
	}

	// Set a value
	cache.Set(idA, idB, 0.85)

	// Now should be found
	val, found := cache.Get(idA, idB)
	if !found {
		t.Fatal("Expected cache hit after Set")
	}
	if math.Abs(val-0.85) > 0.0001 {
		t.Errorf("Expected 0.85, got %v", val)
	}
}

func TestSQLiteOverlapCache_KeyOrdering(t *testing.T) {
	st, cleanup := setupTestStore(t)
	defer cleanup()

	cache := NewSQLiteOverlapCache(st.DB())

	idA := uuid.New()
	idB := uuid.New()

	// Set with (A, B)
	cache.Set(idA, idB, 0.75)

	// Get with (B, A) — should return same value due to consistent ordering
	val, found := cache.Get(idB, idA)
	if !found {
		t.Fatal("Expected cache hit with reversed key order")
	}
	if math.Abs(val-0.75) > 0.0001 {
		t.Errorf("Expected 0.75 with reversed keys, got %v", val)
	}
}

func TestSQLiteOverlapCache_OverwriteValue(t *testing.T) {
	st, cleanup := setupTestStore(t)
	defer cleanup()

	cache := NewSQLiteOverlapCache(st.DB())

	idA := uuid.New()
	idB := uuid.New()

	cache.Set(idA, idB, 0.5)
	cache.Set(idA, idB, 0.9) // overwrite

	val, found := cache.Get(idA, idB)
	if !found {
		t.Fatal("Expected cache hit")
	}
	if math.Abs(val-0.9) > 0.0001 {
		t.Errorf("Expected overwritten value 0.9, got %v", val)
	}
}

func TestSQLiteOverlapCache_MultiplePairs(t *testing.T) {
	st, cleanup := setupTestStore(t)
	defer cleanup()

	cache := NewSQLiteOverlapCache(st.DB())

	pairs := make([][2]uuid.UUID, 5)
	for i := range pairs {
		pairs[i] = [2]uuid.UUID{uuid.New(), uuid.New()}
		cache.Set(pairs[i][0], pairs[i][1], float64(i)*0.2)
	}

	for i, pair := range pairs {
		expected := float64(i) * 0.2
		val, found := cache.Get(pair[0], pair[1])
		if !found {
			t.Errorf("Pair %d: expected cache hit", i)
			continue
		}
		if math.Abs(val-expected) > 0.0001 {
			t.Errorf("Pair %d: expected %v, got %v", i, expected, val)
		}
	}
}

func TestSQLiteOverlapCache_ExpiredTTL(t *testing.T) {
	st, cleanup := setupTestStore(t)
	defer cleanup()

	cache := NewSQLiteOverlapCache(st.DB())

	idA := uuid.New()
	idB := uuid.New()

	// Manually insert with expired TTL
	var first, second uuid.UUID
	if idA.String() < idB.String() {
		first, second = idA, idB
	} else {
		first, second = idB, idA
	}

	// Insert with TTL in the past
	_, err := st.DB().Exec(
		`INSERT OR REPLACE INTO overlap_cache (id_a, id_b, similarity, computed_at, ttl)
		 VALUES (?, ?, ?, unixepoch(), unixepoch() - 1)`,
		first[:], second[:], 0.5,
	)
	if err != nil {
		t.Fatalf("Manual insert error: %v", err)
	}

	// Should not be found (TTL expired)
	_, found := cache.Get(idA, idB)
	if found {
		t.Error("Expected cache miss for expired TTL entry")
	}
}

func TestSQLiteOverlapCache_SameID(t *testing.T) {
	st, cleanup := setupTestStore(t)
	defer cleanup()

	cache := NewSQLiteOverlapCache(st.DB())

	id := uuid.New()

	// Self-overlap should be settable and gettable
	cache.Set(id, id, 1.0)

	val, found := cache.Get(id, id)
	if !found {
		t.Fatal("Expected cache hit for self-overlap")
	}
	if math.Abs(val-1.0) > 0.0001 {
		t.Errorf("Expected 1.0, got %v", val)
	}
}

// --- Benchmarks ---

func BenchmarkCosineSimilarity384(b *testing.B) {
	vec1 := make([]float32, 384)
	vec2 := make([]float32, 384)
	for i := 0; i < 384; i++ {
		vec1[i] = float32(i) / 384.0
		vec2[i] = float32(384-i) / 384.0
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cosineSimilarity(vec1, vec2)
	}
}
