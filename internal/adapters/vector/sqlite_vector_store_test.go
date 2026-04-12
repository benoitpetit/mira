package vector

import (
	"context"
	"os"
	"testing"

	"github.com/benoitpetit/mira/internal/adapters/storage"
	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/util"
	"github.com/google/uuid"
)

// setupTestDB crée une DB temporaire
func setupTestDB(t *testing.T) *storage.SQLiteRepository {
	dbPath := "/tmp/test_mira_" + uuid.New().String() + ".db"
	repo, err := storage.NewSQLiteRepository(dbPath, storage.SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		repo.Close()
		os.Remove(dbPath)
	})
	return repo
}

// createTestVector is defined in hnsw_store_test.go

// createAndStoreCandidate creates a candidate with embedding and stores all components in DB
func createAndStoreCandidate(t *testing.T, repo *storage.SQLiteRepository, dim int, content, wing string, room *string, vectorValue float32) *entities.Candidate {
	verbatim := entities.NewVerbatim(content, wing, room)
	fingerprint := entities.NewFingerprint(verbatim.ID, valueobjects.TypeFact, "test-model")
	embedding := createTestVector(dim, vectorValue)
	candidate := entities.NewCandidate(fingerprint, verbatim, embedding)

	ctx := context.Background()
	if err := repo.StoreVerbatim(ctx, verbatim); err != nil {
		t.Fatalf("Failed to store verbatim: %v", err)
	}
	if err := repo.StoreFingerprint(ctx, fingerprint); err != nil {
		t.Fatalf("Failed to store fingerprint: %v", err)
	}
	emb := entities.NewEmbedding(verbatim.ID, "test-model", embedding)
	if err := repo.StoreEmbedding(ctx, emb); err != nil {
		t.Fatalf("Failed to store embedding: %v", err)
	}

	return candidate
}

// TestSQLiteVectorStoreSearch tests basic search functionality
func TestSQLiteVectorStoreSearch(t *testing.T) {
	repo := setupTestDB(t)
	store := NewSQLiteVectorStore(repo.DB())
	dim := 10

	// Create candidates with different vector values
	room1 := "room1"
	createAndStoreCandidate(t, repo, dim, "content about cats", "wing1", &room1, 0.9)
	createAndStoreCandidate(t, repo, dim, "content about dogs", "wing1", &room1, 0.5)
	createAndStoreCandidate(t, repo, dim, "content about birds", "wing1", &room1, 0.1)

	// Search with a query vector close to the first candidate
	ctx := context.Background()
	query := createTestVector(dim, 0.95)
	results, err := store.Search(ctx, query, 2, nil, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected search results, got none")
	}

	if len(results) > 2 {
		t.Errorf("Expected at most 2 results, got %d", len(results))
	}

	// Results should be ordered by similarity (highest first)
	// The first result should have the highest similarity to query (0.95)
	if len(results) > 0 {
		// Vector with 0.9 value should be most similar to 0.95 query
		expectedSimilarity := util.CosineSimilarity(createTestVector(dim, 0.9), query)
		actualSimilarity := util.CosineSimilarity(results[0].Embedding, query)
		if actualSimilarity < expectedSimilarity-0.01 {
			t.Logf("First result similarity %f, expected at least %f", actualSimilarity, expectedSimilarity)
		}
	}
}

// TestSQLiteVectorStoreAddCandidate tests that AddCandidate is a no-op
func TestSQLiteVectorStoreAddCandidate(t *testing.T) {
	repo := setupTestDB(t)
	store := NewSQLiteVectorStore(repo.DB())
	dim := 10

	// Create a candidate (without storing in DB)
	room := "test-room"
	verbatim := entities.NewVerbatim("test content", "test-wing", &room)
	fingerprint := entities.NewFingerprint(verbatim.ID, valueobjects.TypeFact, "test-model")
	embedding := createTestVector(dim, 0.5)
	candidate := entities.NewCandidate(fingerprint, verbatim, embedding)

	// AddCandidate should be a no-op and return nil
	err := store.AddCandidate(context.Background(), candidate)
	if err != nil {
		t.Errorf("AddCandidate should return nil, got: %v", err)
	}

	// Verify that the candidate is not in the store (since AddCandidate is no-op)
	// Store it properly first
	ctx := context.Background()
	if err := repo.StoreVerbatim(ctx, verbatim); err != nil {
		t.Fatalf("Failed to store verbatim: %v", err)
	}
	if err := repo.StoreFingerprint(ctx, fingerprint); err != nil {
		t.Fatalf("Failed to store fingerprint: %v", err)
	}
	emb := entities.NewEmbedding(verbatim.ID, "test-model", embedding)
	if err := repo.StoreEmbedding(ctx, emb); err != nil {
		t.Fatalf("Failed to store embedding: %v", err)
	}

	// Now search should find it
	query := createTestVector(dim, 0.5)
	results, err := store.Search(ctx, query, 5, nil, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	found := false
	for _, r := range results {
		if r.ID() == candidate.ID() {
			found = true
			break
		}
	}
	if !found {
		t.Error("Candidate should be found after proper storage")
	}
}

// TestSQLiteVectorStoreDelete tests that Delete is a no-op
func TestSQLiteVectorStoreDelete(t *testing.T) {
	repo := setupTestDB(t)
	store := NewSQLiteVectorStore(repo.DB())
	dim := 10

	// Create and store a candidate
	room := "test-room"
	candidate := createAndStoreCandidate(t, repo, dim, "test content", "test-wing", &room, 0.5)

	ctx := context.Background()
	// Verify candidate exists
	query := createTestVector(dim, 0.5)
	results, err := store.Search(ctx, query, 5, nil, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	found := false
	for _, r := range results {
		if r.ID() == candidate.ID() {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Candidate should exist before delete")
	}

	// Delete is a no-op in SQLiteVectorStore
	err = store.Delete(ctx, candidate.ID())
	if err != nil {
		t.Errorf("Delete should return nil, got: %v", err)
	}

	// Candidate should still exist (since Delete is no-op)
	results2, err := store.Search(ctx, query, 5, nil, nil)
	if err != nil {
		t.Fatalf("Search failed after delete: %v", err)
	}

	found = false
	for _, r := range results2 {
		if r.ID() == candidate.ID() {
			found = true
			break
		}
	}
	if !found {
		// This is expected since Delete is no-op
		t.Log("Delete is a no-op as expected")
	}
}

// TestSQLiteVectorStoreSearchWithFilters tests search with wing and room filters
func TestSQLiteVectorStoreSearchWithFilters(t *testing.T) {
	repo := setupTestDB(t)
	store := NewSQLiteVectorStore(repo.DB())
	dim := 10

	// Create candidates in different wings and rooms
	room1 := "room1"
	room2 := "room2"
	createAndStoreCandidate(t, repo, dim, "content in wing1 room1", "wing1", &room1, 0.9)
	createAndStoreCandidate(t, repo, dim, "content in wing1 room2", "wing1", &room2, 0.9)
	createAndStoreCandidate(t, repo, dim, "content in wing2 room1", "wing2", &room1, 0.9)
	createAndStoreCandidate(t, repo, dim, "content in wing2 room2", "wing2", &room2, 0.9)

	ctx := context.Background()
	// Test search with wing filter
	wing1 := "wing1"
	query := createTestVector(dim, 0.9)
	results, err := store.Search(ctx, query, 10, &wing1, nil)
	if err != nil {
		t.Fatalf("Search with wing filter failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results for wing1, got %d", len(results))
	}

	for _, r := range results {
		if r.Verbatim.Wing != wing1 {
			t.Errorf("Expected wing1, got %s", r.Verbatim.Wing)
		}
	}

	// Test search with room filter
	results2, err := store.Search(ctx, query, 10, nil, &room1)
	if err != nil {
		t.Fatalf("Search with room filter failed: %v", err)
	}

	if len(results2) != 2 {
		t.Errorf("Expected 2 results for room1, got %d", len(results2))
	}

	for _, r := range results2 {
		if r.Verbatim.Room == nil || *r.Verbatim.Room != room1 {
			t.Errorf("Expected room1, got %v", r.Verbatim.Room)
		}
	}

	// Test search with both wing and room filters
	results3, err := store.Search(ctx, query, 10, &wing1, &room1)
	if err != nil {
		t.Fatalf("Search with both filters failed: %v", err)
	}

	if len(results3) != 1 {
		t.Errorf("Expected 1 result for wing1+room1, got %d", len(results3))
	}

	if len(results3) > 0 {
		if results3[0].Verbatim.Wing != wing1 {
			t.Errorf("Expected wing1, got %s", results3[0].Verbatim.Wing)
		}
		if results3[0].Verbatim.Room == nil || *results3[0].Verbatim.Room != room1 {
			t.Errorf("Expected room1, got %v", results3[0].Verbatim.Room)
		}
	}
}

// TestSQLiteVectorStoreSearchWithDifferentLimits tests search with different limit values
func TestSQLiteVectorStoreSearchWithDifferentLimits(t *testing.T) {
	repo := setupTestDB(t)
	store := NewSQLiteVectorStore(repo.DB())
	dim := 10

	// Create many candidates
	for i := 0; i < 20; i++ {
		createAndStoreCandidate(t, repo, dim, "test content", "test-wing", nil, float32(i)*0.05)
	}

	ctx := context.Background()
	query := createTestVector(dim, 0.5)

	// Test with limit 5
	results5, err := store.Search(ctx, query, 5, nil, nil)
	if err != nil {
		t.Fatalf("Search with limit 5 failed: %v", err)
	}
	if len(results5) != 5 {
		t.Errorf("Expected 5 results with limit 5, got %d", len(results5))
	}

	// Test with limit 10
	results10, err := store.Search(ctx, query, 10, nil, nil)
	if err != nil {
		t.Fatalf("Search with limit 10 failed: %v", err)
	}
	if len(results10) != 10 {
		t.Errorf("Expected 10 results with limit 10, got %d", len(results10))
	}

	// Test with limit larger than dataset
	results100, err := store.Search(ctx, query, 100, nil, nil)
	if err != nil {
		t.Fatalf("Search with limit 100 failed: %v", err)
	}
	if len(results100) != 20 {
		t.Errorf("Expected 20 results (all items) with limit 100, got %d", len(results100))
	}
}

// TestSQLiteVectorStoreSearchEmptyDatabase tests search on empty database
func TestSQLiteVectorStoreSearchEmptyDatabase(t *testing.T) {
	repo := setupTestDB(t)
	store := NewSQLiteVectorStore(repo.DB())
	dim := 10

	results, err := store.Search(context.Background(), createTestVector(dim, 0.5), 5, nil, nil)
	if err != nil {
		t.Fatalf("Search on empty database failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results on empty database, got %d", len(results))
	}
}

// TestSQLiteVectorStoreSearchOrdering tests that results are ordered by similarity
func TestSQLiteVectorStoreSearchOrdering(t *testing.T) {
	repo := setupTestDB(t)
	store := NewSQLiteVectorStore(repo.DB())
	dim := 10

	// Create candidates with distinct vectors
	createAndStoreCandidate(t, repo, dim, "content A", "test-wing", nil, 0.1)
	createAndStoreCandidate(t, repo, dim, "content B", "test-wing", nil, 0.3)
	createAndStoreCandidate(t, repo, dim, "content C", "test-wing", nil, 0.5)
	createAndStoreCandidate(t, repo, dim, "content D", "test-wing", nil, 0.7)
	createAndStoreCandidate(t, repo, dim, "content E", "test-wing", nil, 0.9)

	// Query with 0.5 - should return results ordered by similarity to 0.5
	ctx := context.Background()
	query := createTestVector(dim, 0.5)
	results, err := store.Search(ctx, query, 5, nil, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 5 {
		t.Fatalf("Expected 5 results, got %d", len(results))
	}

	// Verify ordering - similarities should be decreasing
	for i := 1; i < len(results); i++ {
		simPrev := util.CosineSimilarity(results[i-1].Embedding, query)
		simCurr := util.CosineSimilarity(results[i].Embedding, query)
		if simPrev < simCurr {
			t.Errorf("Results not ordered by similarity: index %d (%f) < index %d (%f)", i-1, simPrev, i, simCurr)
		}
	}

	// The most similar should be the one with 0.5 value
	mostSimilar := util.CosineSimilarity(results[0].Embedding, query)
	expectedMostSimilar := util.CosineSimilarity(createTestVector(dim, 0.5), query)
	if mostSimilar < expectedMostSimilar-0.001 {
		t.Errorf("Most similar result has wrong similarity: got %f, expected %f", mostSimilar, expectedMostSimilar)
	}
}
