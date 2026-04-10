package vector

import (
	"context"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/adapters/storage"
	"github.com/google/uuid"
)

// setupTestDBForCache uses the same setup as other tests
func setupTestDBForCache(t *testing.T) *storage.SQLiteRepository {
	return setupTestDB(t)
}

// TestOverlapCacheGetSet tests basic get and set operations
func TestOverlapCacheGetSet(t *testing.T) {
	repo := setupTestDBForCache(t)
	cache := NewSQLiteOverlapCache(repo.DB())

	idA := uuid.New()
	idB := uuid.New()
	similarity := 0.75

	ctx := context.Background()
	// Set a similarity value
	cache.Set(ctx, idA, idB, similarity)

	// Get the value back
	value, found := cache.Get(ctx, idA, idB)
	if !found {
		t.Error("Expected to find cached value, but it was not found")
	}
	if value != similarity {
		t.Errorf("Expected similarity %f, got %f", similarity, value)
	}
}

// TestOverlapCacheTTL tests that entries expire after TTL
func TestOverlapCacheTTL(t *testing.T) {
	repo := setupTestDBForCache(t)
	cache := NewSQLiteOverlapCache(repo.DB())

	idA := uuid.New()
	idB := uuid.New()
	similarity := 0.85

	// Set a similarity value with short TTL by directly inserting
	// The default TTL is 30 days, so we need to test differently
	// We'll insert with an expired TTL directly
	id1, id2 := idA[:], idB[:]
	if string(idA[:]) > string(idB[:]) {
		id1, id2 = id2, id1
	}

	// Insert with TTL in the past (expired)
	_, err := repo.DB().Exec(
		`INSERT INTO overlap_cache (id_a, id_b, similarity, computed_at, ttl) 
		 VALUES (?, ?, ?, unixepoch() - 100, unixepoch() - 1)`,
		id1, id2, similarity,
	)
	if err != nil {
		t.Fatalf("Failed to insert expired cache entry: %v", err)
	}

	ctx := context.Background()
	// Try to get the expired value
	_, found := cache.Get(ctx, idA, idB)
	if found {
		t.Error("Expected expired cache entry to not be found")
	}

	// Now test with a valid entry - using cache.Set which handles TTL automatically
	idC := uuid.New()
	idD := uuid.New()
	validSimilarity := 0.65

	// Use the Set method (which sets TTL to 30 days from now)
	cache.Set(ctx, idC, idD, validSimilarity)

	// Should find the valid entry
	value, found := cache.Get(ctx, idC, idD)
	if !found {
		t.Error("Expected to find valid cached value")
	}
	if value != validSimilarity {
		t.Errorf("Expected similarity %f, got %f", validSimilarity, value)
	}
}

// TestOverlapCacheKeyOrder tests that key order is normalized
func TestOverlapCacheKeyOrder(t *testing.T) {
	repo := setupTestDBForCache(t)
	cache := NewSQLiteOverlapCache(repo.DB())

	idA := uuid.New()
	idB := uuid.New()
	similarity := 0.90

	ctx := context.Background()
	// Set with (idA, idB)
	cache.Set(ctx, idA, idB, similarity)

	// Get with (idB, idA) - should return the same value
	value, found := cache.Get(ctx, idB, idA)
	if !found {
		t.Error("Expected to find cached value with reversed key order")
	}
	if value != similarity {
		t.Errorf("Expected similarity %f with reversed key order, got %f", similarity, value)
	}

	// Also test that getting with original order still works
	value2, found2 := cache.Get(ctx, idA, idB)
	if !found2 {
		t.Error("Expected to find cached value with original key order")
	}
	if value2 != similarity {
		t.Errorf("Expected similarity %f with original key order, got %f", similarity, value2)
	}
}

// TestOverlapCacheMultipleEntries tests multiple cache entries
func TestOverlapCacheMultipleEntries(t *testing.T) {
	repo := setupTestDBForCache(t)
	cache := NewSQLiteOverlapCache(repo.DB())

	// Create multiple pairs
	pairs := []struct {
		idA        uuid.UUID
		idB        uuid.UUID
		similarity float64
	}{
		{uuid.New(), uuid.New(), 0.1},
		{uuid.New(), uuid.New(), 0.3},
		{uuid.New(), uuid.New(), 0.5},
		{uuid.New(), uuid.New(), 0.7},
		{uuid.New(), uuid.New(), 0.9},
	}

	ctx := context.Background()
	// Set all pairs
	for _, p := range pairs {
		cache.Set(ctx, p.idA, p.idB, p.similarity)
	}

	// Verify all pairs can be retrieved
	for _, p := range pairs {
		value, found := cache.Get(ctx, p.idA, p.idB)
		if !found {
			t.Errorf("Expected to find cached value for pair (%s, %s)", p.idA, p.idB)
			continue
		}
		if value != p.similarity {
			t.Errorf("Expected similarity %f for pair (%s, %s), got %f", p.similarity, p.idA, p.idB, value)
		}
	}
}

// TestOverlapCacheUpdate tests updating an existing entry
func TestOverlapCacheUpdate(t *testing.T) {
	repo := setupTestDBForCache(t)
	cache := NewSQLiteOverlapCache(repo.DB())

	idA := uuid.New()
	idB := uuid.New()

	ctx := context.Background()
	// Set initial value
	cache.Set(ctx, idA, idB, 0.5)

	// Update with new value
	cache.Set(ctx, idA, idB, 0.8)

	// Should get the updated value
	value, found := cache.Get(ctx, idA, idB)
	if !found {
		t.Error("Expected to find updated cached value")
	}
	if value != 0.8 {
		t.Errorf("Expected updated similarity 0.8, got %f", value)
	}
}

// TestOverlapCacheNonExistent tests getting a non-existent entry
func TestOverlapCacheNonExistent(t *testing.T) {
	repo := setupTestDBForCache(t)
	cache := NewSQLiteOverlapCache(repo.DB())

	idA := uuid.New()
	idB := uuid.New()

	_, found := cache.Get(context.Background(), idA, idB)
	if found {
		t.Error("Expected to not find non-existent cached value")
	}
}

// TestOverlapCacheSameID tests behavior with same ID
func TestOverlapCacheSameID(t *testing.T) {
	repo := setupTestDBForCache(t)
	cache := NewSQLiteOverlapCache(repo.DB())

	idA := uuid.New()
	similarity := 1.0

	ctx := context.Background()
	// Set with same ID (self-similarity)
	cache.Set(ctx, idA, idA, similarity)

	// Should be able to retrieve it
	value, found := cache.Get(ctx, idA, idA)
	if !found {
		t.Error("Expected to find self-similarity cached value")
	}
	if value != similarity {
		t.Errorf("Expected self-similarity %f, got %f", similarity, value)
	}
}

// TestOverlapCacheConcurrencyStress tests concurrent access
func TestOverlapCacheConcurrencyStress(t *testing.T) {
	repo := setupTestDBForCache(t)
	cache := NewSQLiteOverlapCache(repo.DB())

	idA := uuid.New()
	idB := uuid.New()

	ctx := context.Background()
	// Concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(val float64) {
			cache.Set(ctx, idA, idB, val)
			done <- true
		}(float64(i) * 0.1)
	}

	// Wait for all writes
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should be able to read a value
	_, found := cache.Get(ctx, idA, idB)
	if !found {
		t.Error("Expected to find cached value after concurrent writes")
	}
}

// TestOverlapCacheTTLExpirationReal tests actual TTL expiration with short TTL
func TestOverlapCacheTTLExpirationReal(t *testing.T) {
	repo := setupTestDBForCache(t)
	cache := NewSQLiteOverlapCache(repo.DB())

	idA := uuid.New()
	idB := uuid.New()
	similarity := 0.75

	// Insert with very short TTL (1 second from now)
	id1, id2 := idA[:], idB[:]
	if string(idA[:]) > string(idB[:]) {
		id1, id2 = id2, id1
	}

	_, err := repo.DB().Exec(
		`INSERT INTO overlap_cache (id_a, id_b, similarity, computed_at, ttl) 
		 VALUES (?, ?, ?, unixepoch(), unixepoch() + 1)`,
		id1, id2, similarity,
	)
	if err != nil {
		t.Fatalf("Failed to insert cache entry: %v", err)
	}

	ctx := context.Background()
	// Should find it immediately
	value, found := cache.Get(ctx, idA, idB)
	if !found {
		t.Error("Expected to find cached value immediately after insert")
	}
	if value != similarity {
		t.Errorf("Expected similarity %f, got %f", similarity, value)
	}

	// Wait for expiration
	time.Sleep(2 * time.Second)

	// Should not find it after expiration
	_, found = cache.Get(ctx, idA, idB)
	if found {
		t.Error("Expected cached value to expire")
	}
}

// TestOverlapCacheNegativeSimilarity tests with negative similarity value
func TestOverlapCacheNegativeSimilarity(t *testing.T) {
	repo := setupTestDBForCache(t)
	cache := NewSQLiteOverlapCache(repo.DB())

	idA := uuid.New()
	idB := uuid.New()
	similarity := -0.5

	ctx := context.Background()
	cache.Set(ctx, idA, idB, similarity)

	value, found := cache.Get(ctx, idA, idB)
	if !found {
		t.Error("Expected to find cached value with negative similarity")
	}
	if value != similarity {
		t.Errorf("Expected similarity %f, got %f", similarity, value)
	}
}

// TestOverlapCacheZeroSimilarity tests with zero similarity value
func TestOverlapCacheZeroSimilarity(t *testing.T) {
	repo := setupTestDBForCache(t)
	cache := NewSQLiteOverlapCache(repo.DB())

	idA := uuid.New()
	idB := uuid.New()
	similarity := 0.0

	ctx := context.Background()
	cache.Set(ctx, idA, idB, similarity)

	value, found := cache.Get(ctx, idA, idB)
	if !found {
		t.Error("Expected to find cached value with zero similarity")
	}
	if value != similarity {
		t.Errorf("Expected similarity %f, got %f", similarity, value)
	}
}
