package vector

import (
	"database/sql"
	"fmt"
	"sort"

	"github.com/benoitpetit/mira/internal/util"
	"github.com/benoitpetit/mira/types"
	"github.com/google/uuid"
)

// Cache TTL constants (in seconds)
const (
	defaultOverlapCacheTTLSeconds = 30 * 24 * 60 * 60 // 30 days
)

// StoreInterface defines the interface needed from store
type StoreInterface interface {
	DB() *sql.DB
	SearchCandidates(wing, room *string, limit int) ([]*types.Candidate, error)
}

// SQLiteAdapter adapts SQLite store for VectorStore interface
type SQLiteAdapter struct {
	store StoreInterface
}

// NewSQLiteAdapter creates a new adapter
func NewSQLiteAdapter(s StoreInterface) *SQLiteAdapter {
	return &SQLiteAdapter{store: s}
}

// Search searches for nearest candidates
func (a *SQLiteAdapter) Search(vector []float32, limit int, wing, room *string) ([]*types.Candidate, error) {
	// For now, use store method to retrieve candidates
	// In a real implementation with HNSW, this would do ANN search
	candidates, err := a.store.SearchCandidates(wing, room, limit*2) // *2 to have enough candidates after scoring
	if err != nil {
		return nil, err
	}

	// Sort by cosine similarity
	type scored struct {
		cand  *types.Candidate
		score float64
	}

	scoredCandidates := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		if len(c.Embedding) == len(vector) {
			score := cosineSimilarity(c.Embedding, vector)
			scoredCandidates = append(scoredCandidates, scored{cand: c, score: score})
		}
	}

	// Sort by descending score (O(n log n) instead of O(n²))
	sort.Slice(scoredCandidates, func(i, j int) bool {
		return scoredCandidates[i].score > scoredCandidates[j].score
	})

	// Return best
	result := make([]*types.Candidate, 0, min(limit, len(scoredCandidates)))
	for i := 0; i < min(limit, len(scoredCandidates)); i++ {
		result = append(result, scoredCandidates[i].cand)
	}

	return result, nil
}

// cosineSimilarity computes cosine similarity using the utility function
func cosineSimilarity(a, b []float32) float64 {
	return util.CosineSimilarity(a, b)
}

// SQLiteOverlapCache persistent overlap cache using SQLite
type SQLiteOverlapCache struct {
	db *sql.DB
}

// NewSQLiteOverlapCache creates a new persistent overlap cache
func NewSQLiteOverlapCache(db *sql.DB) *SQLiteOverlapCache {
	return &SQLiteOverlapCache{db: db}
}

// Get retrieves value from cache with TTL check
func (c *SQLiteOverlapCache) Get(idA, idB uuid.UUID) (float64, bool) {
	var similarity float64
	// Ensure consistent ordering for the key
	var first, second uuid.UUID
	if idA.String() < idB.String() {
		first, second = idA, idB
	} else {
		first, second = idB, idA
	}

	err := c.db.QueryRow(
		`SELECT similarity FROM overlap_cache 
		 WHERE id_a = ? AND id_b = ? AND ttl > unixepoch()`,
		first[:], second[:],
	).Scan(&similarity)

	return similarity, err == nil
}

// Set stores value in cache with TTL (30 days)
func (c *SQLiteOverlapCache) Set(idA, idB uuid.UUID, similarity float64) {
	// Ensure consistent ordering for the key
	var first, second uuid.UUID
	if idA.String() < idB.String() {
		first, second = idA, idB
	} else {
		first, second = idB, idA
	}

	// Use CAST to REAL for STRICT table compatibility
	c.db.Exec(
		fmt.Sprintf(`INSERT OR REPLACE INTO overlap_cache 
		 (id_a, id_b, similarity, computed_at, ttl)
		 VALUES (?, ?, ?, CAST(unixepoch() AS REAL), CAST(unixepoch() + %d AS REAL))`,
			defaultOverlapCacheTTLSeconds),
		first[:], second[:], similarity,
	)
}
