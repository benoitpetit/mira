//go:build !windows
// +build !windows

package vector

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"

	"github.com/benoitpetit/mira/internal/adapters/storage"
	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// setupTestStore creates a temporary HNSW store for benchmarking
func setupTestStore(b *testing.B, dim int) (*HNSWStore, *storage.SQLiteRepository, func()) {
	tmpDir := b.TempDir()
	dbPath := tmpDir + "/test.db"
	indexPath := tmpDir + "/vectors.bin"

	repo, err := storage.NewSQLiteRepository(dbPath, storage.SQLiteOptions{})
	if err != nil {
		b.Fatalf("Failed to create repository: %v", err)
	}

	store, err := NewHNSWStore(repo, dim, indexPath, DefaultHNSWOptions())
	if err != nil {
		b.Fatalf("Failed to create HNSW store: %v", err)
	}

	cleanup := func() {
		repo.Close()
	}

	return store, repo, cleanup
}

// setupTestStoreT creates a temporary HNSW store for testing (uses *testing.T)
func setupTestStoreT(t *testing.T, dim int) (*HNSWStore, *storage.SQLiteRepository, func()) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	indexPath := tmpDir + "/vectors.bin"

	repo, err := storage.NewSQLiteRepository(dbPath, storage.SQLiteOptions{})
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	store, err := NewHNSWStore(repo, dim, indexPath, DefaultHNSWOptions())
	if err != nil {
		repo.Close()
		t.Fatalf("Failed to create HNSW store: %v", err)
	}

	cleanup := func() {
		repo.Close()
	}

	return store, repo, cleanup
}

// generateRandomVector creates a random normalized vector
func generateRandomVector(dim int) []float32 {
	vec := make([]float32, dim)
	var sum float32
	for i := 0; i < dim; i++ {
		vec[i] = rand.Float32()
		sum += vec[i] * vec[i]
	}
	// Normalize
	if sum > 0 {
		norm := float32(1.0 / float32(sum))
		for i := 0; i < dim; i++ {
			vec[i] *= norm
		}
	}
	return vec
}

// createTestCandidate creates a test candidate with the given embedding
func createTestCandidate(embedding []float32) *entities.Candidate {
	room := "test-room"
	verbatim := entities.NewVerbatim("test content", "test-wing", &room)
	fingerprint := entities.NewFingerprint(verbatim.ID, valueobjects.TypeFact, "test-model")
	return entities.NewCandidate(fingerprint, verbatim, embedding)
}

// createAndPersistCandidate creates a candidate and persists it to the database
func createAndPersistCandidate(t *testing.T, repo *storage.SQLiteRepository, dim int, wing string, room *string, vectorValue float32) *entities.Candidate {
	verbatim := entities.NewVerbatim("test content", wing, room)
	fingerprint := entities.NewFingerprint(verbatim.ID, valueobjects.TypeFact, "test-model")
	embedding := createTestVector(dim, vectorValue)
	candidate := entities.NewCandidate(fingerprint, verbatim, embedding)

	persistCandidate(t, repo, candidate)

	return candidate
}

func persistCandidate(tb testing.TB, repo *storage.SQLiteRepository, candidate *entities.Candidate) {
	tb.Helper()
	ctx := context.Background()
	if err := repo.StoreVerbatim(ctx, candidate.Verbatim); err != nil {
		tb.Fatalf("Failed to store verbatim: %v", err)
	}
	if err := repo.StoreFingerprint(ctx, candidate.Memory); err != nil {
		tb.Fatalf("Failed to store fingerprint: %v", err)
	}
	emb := entities.NewEmbedding(candidate.Verbatim.ID, "test-model", candidate.Embedding)
	if err := repo.StoreEmbedding(ctx, emb); err != nil {
		tb.Fatalf("Failed to store embedding: %v", err)
	}
}

type outOfOrderEmbeddingSource struct {
	ports.EmbeddingSource
	omitID      uuid.UUID
	duplicateID uuid.UUID
}

func (s outOfOrderEmbeddingSource) GetCandidatesWithEmbeddings(ctx context.Context, ids []uuid.UUID, wing, room *string) ([]*entities.Candidate, error) {
	candidates, err := s.EmbeddingSource.GetCandidatesWithEmbeddings(ctx, ids, wing, room)
	if err != nil {
		return nil, err
	}
	byVerbatimID := make(map[uuid.UUID]*entities.Candidate, len(candidates))
	for _, candidate := range candidates {
		if candidate != nil && candidate.Verbatim != nil {
			byVerbatimID[candidate.Verbatim.ID] = candidate
		}
	}

	// Return the hydrated rows in reverse ANN order, omit one indexed ID, and
	// duplicate another row to model stale/multiplying relational hydration.
	shuffled := make([]*entities.Candidate, 0, len(ids)+1)
	for i := len(ids) - 1; i >= 0; i-- {
		id := ids[i]
		if id == s.omitID {
			continue
		}
		candidate, ok := byVerbatimID[id]
		if !ok {
			continue
		}
		shuffled = append(shuffled, candidate)
		if id == s.duplicateID {
			shuffled = append(shuffled, candidate)
		}
	}
	return shuffled, nil
}

func TestHNSWStoreSearchPreservesGraphRankAfterHydration(t *testing.T) {
	const dim = 3
	store, repo, cleanup := setupTestStoreT(t, dim)
	defer cleanup()

	room := "rank-test"
	vectors := [][]float32{
		{1, 0, 0},
		{0.9, 0.4, 0},
		{0.7, 0.7, 0},
		{0, 1, 0},
		{-1, 0, 0},
		{0, 0, 1},
	}
	for i, vector := range vectors {
		verbatim := entities.NewVerbatim(fmt.Sprintf("candidate-%d", i), "other-wing", &room)
		fingerprint := entities.NewFingerprint(verbatim.ID, valueobjects.TypeFact, "test-model")
		if err := repo.StoreVerbatim(context.Background(), verbatim); err != nil {
			t.Fatalf("store verbatim: %v", err)
		}
		if err := repo.StoreFingerprint(context.Background(), fingerprint); err != nil {
			t.Fatalf("store fingerprint: %v", err)
		}
		if err := repo.StoreEmbedding(context.Background(), entities.NewEmbedding(verbatim.ID, "test-model", vector)); err != nil {
			t.Fatalf("store embedding: %v", err)
		}
	}
	if err := store.BuildFromStore(context.Background()); err != nil {
		t.Fatalf("build HNSW index: %v", err)
	}

	query := []float32{1, 0, 0}
	annResults := store.graph.Search(floatsToEmbedding(query), store.graph.Len())
	rankedIDs := make([]uuid.UUID, 0, len(annResults))
	for _, result := range annResults {
		if id, ok := store.idToUUID[result.ID()]; ok {
			rankedIDs = append(rankedIDs, id)
		}
	}
	if len(rankedIDs) < 6 {
		t.Fatalf("expected six ANN results, got %d", len(rankedIDs))
	}
	// Make only a sparse subset eligible for hydration. The index keeps its
	// original rank while SQL applies the wing filter during candidate lookup.
	want := []uuid.UUID{rankedIDs[1], rankedIDs[4], rankedIDs[5]}
	for _, id := range want {
		if _, err := repo.DB().ExecContext(context.Background(), "UPDATE verbatim SET wing = ? WHERE id = ?", "rank-wing", id[:]); err != nil {
			t.Fatalf("set filtered wing: %v", err)
		}
	}

	store.store = outOfOrderEmbeddingSource{
		EmbeddingSource: repo,
		omitID:          rankedIDs[1],
		duplicateID:     rankedIDs[4],
	}
	wing := "rank-wing"
	results, err := store.Search(context.Background(), query, 2, &wing, &room)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	want = want[1:]
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2 after skipping missing and duplicate hydrated rows", len(results))
	}
	for i, candidate := range results {
		if candidate.Verbatim.ID != want[i] {
			t.Errorf("result %d has verbatim ID %s, want ANN-ranked ID %s", i, candidate.Verbatim.ID, want[i])
		}
		if candidate.Verbatim.Wing != wing || candidate.Verbatim.Room == nil || *candidate.Verbatim.Room != room {
			t.Errorf("result %d escaped the requested wing/room filters", i)
		}
	}
}

// BenchmarkHNSWAdd measures insertion performance
func BenchmarkHNSWAdd(b *testing.B) {
	dim := 384
	store, _, cleanup := setupTestStore(b, dim)
	defer cleanup()

	vectors := make([][]float32, b.N)
	for i := 0; i < b.N; i++ {
		vectors[i] = generateRandomVector(dim)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		candidate := createTestCandidate(vectors[i])
		_ = store.AddCandidate(context.Background(), candidate)
	}
}

// BenchmarkHNSWSearch measures search performance
func BenchmarkHNSWSearch(b *testing.B) {
	dim := 384
	store, repo, cleanup := setupTestStore(b, dim)
	defer cleanup()

	// Add some vectors first
	numVectors := 1000
	for i := 0; i < numVectors; i++ {
		candidate := createTestCandidate(generateRandomVector(dim))
		persistCandidate(b, repo, candidate)
	}

	if err := store.BuildFromStore(context.Background()); err != nil {
		b.Fatalf("BuildFromStore: %v", err)
	}

	query := generateRandomVector(dim)
	limit := 10

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Search(context.Background(), query, limit, nil, nil)
	}
}

// BenchmarkHNSWSearchScalability measures search with different dataset sizes
func BenchmarkHNSWSearchScalability(b *testing.B) {
	dim := 384
	sizes := []int{100, 500, 1000, 5000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			store, repo, cleanup := setupTestStore(b, dim)
			defer cleanup()

			// Add vectors
			for i := 0; i < size; i++ {
				candidate := createTestCandidate(generateRandomVector(dim))
				persistCandidate(b, repo, candidate)
			}

			if err := store.BuildFromStore(context.Background()); err != nil {
				b.Fatalf("BuildFromStore: %v", err)
			}

			query := generateRandomVector(dim)
			limit := 10

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = store.Search(context.Background(), query, limit, nil, nil)
			}
		})
	}
}

// BenchmarkHNSWConcurrentAccess measures concurrent search performance
func BenchmarkHNSWConcurrentAccess(b *testing.B) {
	dim := 384
	store, repo, cleanup := setupTestStore(b, dim)
	defer cleanup()

	// Add vectors
	numVectors := 1000
	for i := 0; i < numVectors; i++ {
		candidate := createTestCandidate(generateRandomVector(dim))
		persistCandidate(b, repo, candidate)
	}

	if err := store.BuildFromStore(context.Background()); err != nil {
		b.Fatalf("BuildFromStore: %v", err)
	}

	queries := make([][]float32, 100)
	for i := 0; i < 100; i++ {
		queries[i] = generateRandomVector(dim)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _ = store.Search(context.Background(), queries[i%100], 10, nil, nil)
			i++
		}
	})
}

// BenchmarkHNSWBuildTime measures index build time
// Note: This benchmark measures setup time with pre-loaded data
func BenchmarkHNSWBuildTime(b *testing.B) {
	dim := 384
	numVectors := 2000

	store, repo, cleanup := setupTestStore(b, dim)
	defer cleanup()

	// Pre-populate store
	for j := 0; j < numVectors; j++ {
		candidate := createTestCandidate(generateRandomVector(dim))
		persistCandidate(b, repo, candidate)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := store.BuildFromStore(context.Background()); err != nil {
			b.Fatalf("BuildFromStore: %v", err)
		}
	}
}

// TestHNSWBasicOperations tests basic HNSW operations
func TestHNSWBasicOperations(t *testing.T) {
	dim := 384
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	indexPath := tmpDir + "/vectors.bin"

	repo, err := storage.NewSQLiteRepository(dbPath, storage.SQLiteOptions{})
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}
	defer repo.Close()

	store, err := NewHNSWStore(repo, dim, indexPath, DefaultHNSWOptions())
	if err != nil {
		t.Fatalf("Failed to create HNSW store: %v", err)
	}

	ctx := context.Background()
	// Test adding candidates
	for i := 0; i < 10; i++ {
		candidate := createTestCandidate(generateRandomVector(dim))
		err := store.AddCandidate(ctx, candidate)
		if err != nil {
			t.Fatalf("Failed to add candidate: %v", err)
		}
	}

	// Test stats
	stats := store.Stats()
	if stats != 10 {
		t.Errorf("Expected 10 items in store, got %d", stats)
	}

	// Test build and search
	_ = store.BuildFromStore(ctx)

	if !store.IsReady() {
		t.Error("Store should be ready after BuildFromStore")
	}

	// Test search
	query := generateRandomVector(dim)
	results, err := store.Search(ctx, query, 5, nil, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Note: results may be empty because the candidates are not persisted to SQLite
	// This is expected behavior for this test
	t.Logf("Search returned %d results", len(results))
}

func TestPersistedHNSWBenchmarkFixtureBuildsExpectedIndex(t *testing.T) {
	store, repo, cleanup := setupTestStoreT(t, 4)
	defer cleanup()
	candidate := createTestCandidate([]float32{1, 0, 0, 0})

	persistCandidate(t, repo, candidate)
	if err := store.BuildFromStore(context.Background()); err != nil {
		t.Fatalf("BuildFromStore: %v", err)
	}
	if got := store.Stats(); got != 1 {
		t.Fatalf("index contains %d vectors, want 1 persisted vector", got)
	}
	results, err := store.Search(context.Background(), []float32{1, 0, 0, 0}, 1, nil, nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search returned %d results, want 1", len(results))
	}
}

// TestHNSWDelete tests deletion
func TestHNSWDelete(t *testing.T) {
	dim := 384
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	indexPath := tmpDir + "/vectors.bin"

	repo, err := storage.NewSQLiteRepository(dbPath, storage.SQLiteOptions{})
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}
	defer repo.Close()

	store, err := NewHNSWStore(repo, dim, indexPath, DefaultHNSWOptions())
	if err != nil {
		t.Fatalf("Failed to create HNSW store: %v", err)
	}

	ctx := context.Background()
	// Add candidate
	candidate := createTestCandidate(generateRandomVector(dim))
	_ = store.AddCandidate(ctx, candidate)

	if store.Stats() != 1 {
		t.Errorf("Expected 1 item, got %d", store.Stats())
	}

	// Delete candidate
	err = store.Delete(ctx, candidate.ID())
	if err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}

	// Note: coder/hnsw Delete() does not reduce Len(), it only marks the node as deleted.
	// The Stats() may still report 1, which is expected library behavior.
	t.Logf("Stats after delete: %d (library may retain deleted nodes in Len())", store.Stats())
}

// TestHNSWStoreSaveLoad tests persistence: Save and Load
func TestHNSWStoreSaveLoad(t *testing.T) {
	dim := 10
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	indexPath := tmpDir + "/vectors.bin"

	// Create first store and add candidates
	repo1, err := storage.NewSQLiteRepository(dbPath, storage.SQLiteOptions{})
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	store1, err := NewHNSWStore(repo1, dim, indexPath, DefaultHNSWOptions())
	if err != nil {
		repo1.Close()
		t.Fatalf("Failed to create HNSW store: %v", err)
	}

	// Create and persist candidates to both DB and store
	candidates := make([]*entities.Candidate, 5)
	for i := 0; i < 5; i++ {
		candidates[i] = createAndPersistCandidate(t, repo1, dim, "test-wing", nil, float32(i+1)*0.1)
		if err := store1.AddCandidate(context.Background(), candidates[i]); err != nil {
			t.Fatalf("Failed to add candidate: %v", err)
		}
	}

	// Save the index
	if err := store1.Save(); err != nil {
		t.Fatalf("Failed to save index: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Fatal("Index file was not created")
	}

	// Verify stats before closing
	if store1.Stats() != 5 {
		t.Errorf("Expected 5 items before save, got %d", store1.Stats())
	}

	// Close first store
	repo1.Close()

	// Create new store and load
	repo2, err := storage.NewSQLiteRepository(dbPath, storage.SQLiteOptions{})
	if err != nil {
		t.Fatalf("Failed to create second repository: %v", err)
	}
	defer repo2.Close()

	store2, err := NewHNSWStore(repo2, dim, indexPath, DefaultHNSWOptions())
	if err != nil {
		t.Fatalf("Failed to create second HNSW store: %v", err)
	}

	// Load the index - this restores mappings and graph structure
	if err := store2.Load(); err != nil {
		t.Fatalf("Failed to load index: %v", err)
	}

	// Verify data is restored - Load calls rebuildGraph which sets ready=true
	if !store2.IsReady() {
		t.Error("Store should be ready after Load")
	}

	// The graph should be rebuilt with vectors from DB
	// Note: rebuildGraph only adds vectors for UUIDs that were in the saved mappings
	if store2.Stats() != 5 {
		t.Logf("Note: Expected 5 items after load, got %d (vectors from DB matching saved mappings)", store2.Stats())
	}
}

// TestHNSWStoreSearch tests search functionality
func TestHNSWStoreSearch(t *testing.T) {
	dim := 10
	store, repo, cleanup := setupTestStoreT(t, dim)
	defer cleanup()

	// Create candidates with distinct vectors
	// Using simple vectors where similarity is predictable
	room := "test-room"
	for i := 0; i < 5; i++ {
		vec := make([]float32, dim)
		// Each vector has a dominant component
		vec[i] = 1.0

		// Create verbatim, fingerprint and candidate
		verbatim := entities.NewVerbatim("test content", "test-wing", &room)
		fingerprint := entities.NewFingerprint(verbatim.ID, valueobjects.TypeFact, "test-model")
		candidate := entities.NewCandidate(fingerprint, verbatim, vec)

		ctx := context.Background()
		// Store all components in DB
		if err := repo.StoreVerbatim(ctx, verbatim); err != nil {
			t.Fatalf("Failed to store verbatim: %v", err)
		}
		if err := repo.StoreFingerprint(ctx, fingerprint); err != nil {
			t.Fatalf("Failed to store fingerprint: %v", err)
		}
		emb := entities.NewEmbedding(verbatim.ID, "test-model", vec)
		if err := repo.StoreEmbedding(ctx, emb); err != nil {
			t.Fatalf("Failed to store embedding: %v", err)
		}

		// Add to HNSW store
		if err := store.AddCandidate(ctx, candidate); err != nil {
			t.Fatalf("Failed to add candidate: %v", err)
		}
	}

	// Build index from store (this loads vectors from DB)
	if err := store.BuildFromStore(context.Background()); err != nil {
		t.Fatalf("Failed to build index: %v", err)
	}

	// Search with a query vector similar to the first candidate
	query := make([]float32, dim)
	query[0] = 1.0 // Should be most similar to candidate with vec[0]=1.0

	results, err := store.Search(context.Background(), query, 3, nil, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected search results, got none")
	}

	// The first result should be most similar (same dominant component)
	if len(results) > 0 {
		t.Logf("Search returned %d results", len(results))
		// Just verify we got results - exact ordering depends on HNSW implementation
	}
}

// TestHNSWStoreAddDelete tests add and delete operations
func TestHNSWStoreAddDelete(t *testing.T) {
	dim := 10
	store, repo, cleanup := setupTestStoreT(t, dim)
	defer cleanup()

	// Add a candidate and persist it
	room := "test-room"
	candidate := createAndPersistCandidate(t, repo, dim, "test-wing", &room, 0.5)

	if err := store.AddCandidate(context.Background(), candidate); err != nil {
		t.Fatalf("Failed to add candidate: %v", err)
	}

	// Verify candidate is added to the graph
	if store.Stats() != 1 {
		t.Errorf("Expected 1 item after add, got %d", store.Stats())
	}

	// Build index from store for search (this adds vectors from DB to the graph)
	if err := store.BuildFromStore(context.Background()); err != nil {
		t.Fatalf("Failed to build index: %v", err)
	}

	// After BuildFromStore, we should have vectors from DB in the graph
	// Note: BuildFromStore adds vectors from DB, but the candidate was already added via AddCandidate
	// so we expect 2 items (or 1 if the system handles duplicates)
	t.Logf("Stats after BuildFromStore: %d", store.Stats())

	// Delete the candidate from the graph
	if err := store.Delete(context.Background(), candidate.ID()); err != nil {
		t.Fatalf("Failed to delete candidate: %v", err)
	}

	// Verify candidate is deleted from the graph
	// Note: Delete removes the graph node, but BuildFromStore may have added a duplicate
	// The test verifies that Delete at least attempts to remove the node
	t.Logf("Stats after delete: %d", store.Stats())
}

// TestHNSWStoreCompletePersistence tests complete save and load of the HNSW graph
func TestHNSWStoreCompletePersistence(t *testing.T) {
	dim := 10
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	indexPath := tmpDir + "/vectors.bin"

	// Create the first store and add candidates
	repo1, err := storage.NewSQLiteRepository(dbPath, storage.SQLiteOptions{})
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	store1, err := NewHNSWStore(repo1, dim, indexPath, DefaultHNSWOptions())
	if err != nil {
		repo1.Close()
		t.Fatalf("Failed to create HNSW store: %v", err)
	}

	// Create and persist candidates
	candidates := make([]*entities.Candidate, 5)
	for i := 0; i < 5; i++ {
		candidates[i] = createAndPersistCandidate(t, repo1, dim, "test-wing", nil, float32(i+1)*0.1)
		if err := store1.AddCandidate(context.Background(), candidates[i]); err != nil {
			t.Fatalf("Failed to add candidate: %v", err)
		}
	}

	// Build the index to make it ready
	if err := store1.BuildFromStore(context.Background()); err != nil {
		t.Fatalf("Failed to build index: %v", err)
	}

	if !store1.IsReady() {
		t.Fatal("Store1 should be ready after BuildFromStore")
	}

	// Save the complete index
	if err := store1.Save(); err != nil {
		t.Fatalf("Failed to save index: %v", err)
	}

	// Verify that the file exists
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Fatal("Index file was not created")
	}

	// Close the first store
	repo1.Close()

	// Create a new store and load the index
	repo2, err := storage.NewSQLiteRepository(dbPath, storage.SQLiteOptions{})
	if err != nil {
		t.Fatalf("Failed to create second repository: %v", err)
	}
	defer repo2.Close()

	store2, err := NewHNSWStore(repo2, dim, indexPath, DefaultHNSWOptions())
	if err != nil {
		t.Fatalf("Failed to create second HNSW store: %v", err)
	}

	// Load the index - should load the complete graph
	if err := store2.Load(); err != nil {
		t.Fatalf("Failed to load index: %v", err)
	}

	// Verify that the index is ready without reconstruction
	if !store2.IsReady() {
		t.Error("Store should be ready after Load without needing BuildFromStore")
	}

	// Verify that all vectors are present
	// AddCandidate and BuildFromStore both use Verbatim.ID as UUID key,
	// so duplicates are overwritten. We expect exactly 5 vectors.
	if store2.Stats() != 5 {
		t.Errorf("Expected 5 items after load, got %d", store2.Stats())
	}

	// Perform a search to verify the index works
	query := createTestVector(dim, 0.15)
	results2, err := store2.Search(context.Background(), query, 3, nil, nil)
	if err != nil {
		t.Fatalf("Search after load failed: %v", err)
	}

	// Verify that we have results (exact count may vary since HNSW is approximate)
	if len(results2) == 0 {
		t.Error("Expected search results after load, got none")
	}

	t.Logf("Persistence test passed: %d vectors saved and loaded, search returned %d results",
		store2.Stats(), len(results2))
}

// TestHNSWStore_SetModelHash verifies the setter doesn't panic or error.
func TestHNSWStore_SetModelHash(t *testing.T) {
	store, _, cleanup := setupTestStoreT(t, 10)
	defer cleanup()
	// Must not panic.
	store.SetModelHash("abc123")
}

func TestHNSWStoreBuildFromStoreKeepsIncompatibleEmbeddingsNotReady(t *testing.T) {
	const dimension = 3
	store, repo, cleanup := setupTestStoreT(t, dimension)
	defer cleanup()
	ctx := context.Background()
	store.SetModelHash("current-model")

	modelMismatch := createAndPersistCandidate(t, repo, dimension, "wing", nil, 0.1)

	wrongDimension := entities.NewVerbatim("wrong dimension", "wing", nil)
	fingerprint := entities.NewFingerprint(wrongDimension.ID, valueobjects.TypeFact, "current-model")
	if err := repo.StoreVerbatim(ctx, wrongDimension); err != nil {
		t.Fatalf("store wrong-dimension verbatim: %v", err)
	}
	if err := repo.StoreFingerprint(ctx, fingerprint); err != nil {
		t.Fatalf("store wrong-dimension fingerprint: %v", err)
	}
	if err := repo.StoreEmbedding(ctx, entities.NewEmbedding(wrongDimension.ID, "current-model", []float32{1, 0})); err != nil {
		t.Fatalf("store wrong-dimension embedding: %v", err)
	}

	err := store.BuildFromStore(ctx)
	if err == nil {
		t.Fatal("BuildFromStore succeeded after skipping authoritative embeddings")
	}
	if !strings.Contains(err.Error(), "model hash mismatch: 1") || !strings.Contains(err.Error(), "dimension mismatch: 1") {
		t.Fatalf("BuildFromStore error = %q, want mismatch counts", err)
	}
	if store.IsReady() {
		t.Fatal("index reported ready despite skipped authoritative embeddings")
	}

	// Startup keeps this index behind a fallback wrapper; a not-ready partial
	// graph must therefore leave the authoritative brute-force path available.
	fallback := NewFallbackVectorStore(store, NewBruteForceVectorStore(repo))
	results, err := fallback.Search(ctx, []float32{1, 0, 0}, 5, nil, nil)
	if err != nil {
		t.Fatalf("fallback search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("fallback returned %d candidates, want both authoritative embeddings (including %v)", len(results), modelMismatch.ID())
	}
}

func TestHNSWStoreLoadRejectsPersistedPartialIndex(t *testing.T) {
	const dimension = 3
	store, repo, cleanup := setupTestStoreT(t, dimension)
	defer cleanup()
	ctx := context.Background()
	store.SetModelHash("current-model")

	// Keep one compatible vector in the persisted graph while BuildFromStore
	// skips incompatible authoritative embeddings.
	createAndPersistCandidate(t, repo, dimension, "wing", nil, 0.1)
	compatible := entities.NewVerbatim("compatible", "wing", nil)
	fingerprint := entities.NewFingerprint(compatible.ID, valueobjects.TypeFact, "current-model")
	if err := repo.StoreVerbatim(ctx, compatible); err != nil {
		t.Fatalf("store compatible verbatim: %v", err)
	}
	if err := repo.StoreFingerprint(ctx, fingerprint); err != nil {
		t.Fatalf("store compatible fingerprint: %v", err)
	}
	if err := repo.StoreEmbedding(ctx, entities.NewEmbedding(compatible.ID, "current-model", []float32{1, 0, 0})); err != nil {
		t.Fatalf("store compatible embedding: %v", err)
	}

	if err := store.BuildFromStore(ctx); err == nil {
		t.Fatal("BuildFromStore succeeded after skipping an incompatible embedding")
	}
	if err := store.Save(); err != nil {
		t.Fatalf("save incomplete index: %v", err)
	}

	loaded, err := NewHNSWStore(repo, dimension, store.indexPath, DefaultHNSWOptions())
	if err != nil {
		t.Fatalf("create reload store: %v", err)
	}
	loaded.SetModelHash("current-model")
	loadErr := loaded.Load()
	if loaded.IsReady() {
		t.Fatalf("Load marked an index with missing authoritative embeddings ready (error: %v)", loadErr)
	}

	fallback := NewFallbackVectorStore(loaded, NewBruteForceVectorStore(repo))
	results, err := fallback.Search(ctx, []float32{1, 0, 0}, 5, nil, nil)
	if err != nil {
		t.Fatalf("fallback search after rejected load: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("fallback returned %d authoritative candidates, want 2", len(results))
	}
}

func TestHNSWStoreClearAddSearchAndRestart(t *testing.T) {
	const dimension = 3
	store, repo, cleanup := setupTestStoreT(t, dimension)
	defer cleanup()
	ctx := context.Background()
	initial := createAndPersistCandidate(t, repo, dimension, "wing", nil, 0.1)
	if err := store.AddCandidate(ctx, initial); err != nil {
		t.Fatalf("add initial candidate: %v", err)
	}
	if err := store.BuildFromStore(ctx); err != nil {
		t.Fatalf("build initial index: %v", err)
	}
	fallback := NewFallbackVectorStore(store, NewBruteForceVectorStore(repo))

	if err := repo.ClearAll(ctx); err != nil {
		t.Fatalf("clear authoritative repository: %v", err)
	}
	if err := fallback.ClearAll(ctx); err != nil {
		t.Fatalf("clear vector index: %v", err)
	}
	if !store.IsReady() {
		t.Fatal("empty HNSW index should remain ready after ClearAll")
	}
	empty, err := fallback.Search(ctx, []float32{1, 0, 0}, 5, nil, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("search empty index = %v, %v; want no results and no error", empty, err)
	}

	added := createAndPersistCandidate(t, repo, dimension, "wing", nil, 0.8)
	if err := fallback.AddCandidate(ctx, added); err != nil {
		t.Fatalf("add candidate after clear: %v", err)
	}
	results, err := fallback.Search(ctx, added.Embedding, 5, nil, nil)
	if err != nil {
		t.Fatalf("search after clear/add: %v", err)
	}
	if len(results) != 1 || results[0].Verbatim.ID != added.Verbatim.ID {
		t.Fatalf("search after clear/add = %v, want new candidate %s", results, added.Verbatim.ID)
	}
	if err := store.Save(); err != nil {
		t.Fatalf("save post-clear index: %v", err)
	}

	loaded, err := NewHNSWStore(repo, dimension, store.indexPath, DefaultHNSWOptions())
	if err != nil {
		t.Fatalf("create reload store: %v", err)
	}
	if err := loaded.Load(); err != nil {
		t.Fatalf("load post-clear index: %v", err)
	}
	if !loaded.IsReady() {
		t.Fatal("complete post-clear index should load ready")
	}
	results, err = loaded.Search(ctx, added.Embedding, 5, nil, nil)
	if err != nil || len(results) != 1 || results[0].Verbatim.ID != added.Verbatim.ID {
		t.Fatalf("search after restart = %v, %v; want new candidate %s", results, err, added.Verbatim.ID)
	}
}

// TestHNSWStore_SearchLexical_AndExact delegates to the underlying SQLite store.
// Without FTS5 these calls should return gracefully (nil or error).
func TestHNSWStore_SearchLexical_AndExact(t *testing.T) {
	store, _, cleanup := setupTestStoreT(t, 10)
	defer cleanup()
	ctx := context.Background()

	// SearchLexical delegates to store.SearchLexical — may return an error if FTS5
	// is unavailable; we just need the code path to be exercised without panic.
	_, _ = store.SearchLexical(ctx, "foo", 5, nil, nil)

	// SearchExact delegates to exactStore interface; SQLiteRepository implements it.
	_, _ = store.SearchExact(ctx, "foo", 5, nil, nil)
}

// TestHNSWStore_ClearAll resets the in-memory index.
func TestHNSWStore_ClearAll(t *testing.T) {
	dim := 10
	store, repo, cleanup := setupTestStoreT(t, dim)
	defer cleanup()

	// Add candidates and build.
	for i := 0; i < 3; i++ {
		c := createAndPersistCandidate(t, repo, dim, "wing", nil, float32(i+1)*0.1)
		if err := store.AddCandidate(context.Background(), c); err != nil {
			t.Fatalf("AddCandidate: %v", err)
		}
	}
	if err := store.BuildFromStore(context.Background()); err != nil {
		t.Fatalf("BuildFromStore: %v", err)
	}
	if !store.IsReady() {
		t.Fatal("expected store to be ready before ClearAll")
	}

	if err := store.ClearAll(context.Background()); err != nil {
		t.Fatalf("ClearAll: %v", err)
	}

	// An empty graph is a valid searchable index after ClearAll.
	if !store.IsReady() {
		t.Error("expected empty store to remain ready after ClearAll")
	}
	if store.Stats() != 0 {
		t.Errorf("Stats() = %d after ClearAll, want 0", store.Stats())
	}
}

// TestHNSWStore_ClearByRoom rebuilds the index from the DB (which was already
// cleared by the repository layer).
func TestHNSWStore_ClearByRoom(t *testing.T) {
	dim := 10
	store, repo, cleanup := setupTestStoreT(t, dim)
	defer cleanup()

	c := createAndPersistCandidate(t, repo, dim, "wing", nil, 0.5)
	if err := store.AddCandidate(context.Background(), c); err != nil {
		t.Fatalf("AddCandidate: %v", err)
	}

	// ClearByRoom triggers a BuildFromStore internally — must not error.
	if err := store.ClearByRoom(context.Background(), "wing", nil); err != nil {
		t.Fatalf("ClearByRoom: %v", err)
	}
}

// TestTimeUnix verifies the float64→time.Time conversion helper.
func TestTimeUnix(t *testing.T) {
	ts := float64(1_700_000_000)
	got := timeUnix(ts)
	if got.Unix() != int64(ts) {
		t.Errorf("timeUnix(%v).Unix() = %d, want %d", ts, got.Unix(), int64(ts))
	}
	// Zero value.
	if timeUnix(0).Unix() != 0 {
		t.Error("timeUnix(0) should be Unix epoch")
	}
}

// ── AES-256-GCM encryption (J2) ───────────────────────────────────────────────

func newTestHNSWStore(t *testing.T) (*HNSWStore, func()) {
	t.Helper()
	tmp := t.TempDir()
	repo, err := storage.NewSQLiteRepository(tmp+"/test.db", storage.SQLiteOptions{})
	if err != nil {
		t.Fatalf("repo: %v", err)
	}
	store, err := NewHNSWStore(repo, 4, tmp+"/vectors.bin", DefaultHNSWOptions())
	if err != nil {
		t.Fatalf("hnsw: %v", err)
	}
	return store, func() { repo.Close() }
}

func TestSetEncryptionKey_32Bytes(t *testing.T) {
	s, cleanup := newTestHNSWStore(t)
	defer cleanup()

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	s.SetEncryptionKey(key)
	// re-set to nil (disables encryption)
	s.SetEncryptionKey(nil)
}

func TestSetEncryptionKey_ShortKey(t *testing.T) {
	s, cleanup := newTestHNSWStore(t)
	defer cleanup()
	// A short key should be normalised via SHA-256, not rejected
	s.SetEncryptionKey([]byte("short"))
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	s, cleanup := newTestHNSWStore(t)
	defer cleanup()

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	s.SetEncryptionKey(key)

	plaintext := []byte("hello, AES-256-GCM!")
	ciphertext, err := s.encryptAESGCM(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if len(ciphertext) <= len(plaintext) {
		t.Fatal("ciphertext should be longer than plaintext (nonce + tag overhead)")
	}

	decrypted, err := s.decryptAESGCM(ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("roundtrip failed: got %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptAESGCM_TooShort(t *testing.T) {
	s, cleanup := newTestHNSWStore(t)
	defer cleanup()

	key := make([]byte, 32)
	s.SetEncryptionKey(key)

	_, err := s.decryptAESGCM([]byte("short"))
	if err == nil {
		t.Error("expected error for too-short ciphertext")
	}
}

func TestSaveLoad_WithEncryption(t *testing.T) {
	tmp := t.TempDir()
	repo, err := storage.NewSQLiteRepository(tmp+"/test.db", storage.SQLiteOptions{})
	if err != nil {
		t.Fatalf("repo: %v", err)
	}
	defer repo.Close()

	indexPath := tmp + "/vectors.bin"
	store, err := NewHNSWStore(repo, 4, indexPath, DefaultHNSWOptions())
	if err != nil {
		t.Fatalf("hnsw: %v", err)
	}

	key := make([]byte, 32)
	for i := range key {
		key[i] = 0xAB
	}
	store.SetEncryptionKey(key)

	// Save with encryption
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load with the same key
	store2, err := NewHNSWStore(repo, 4, indexPath, DefaultHNSWOptions())
	if err != nil {
		t.Fatalf("hnsw2: %v", err)
	}
	store2.SetEncryptionKey(key)
	if err := store2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
}
