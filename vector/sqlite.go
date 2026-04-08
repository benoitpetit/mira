package vector

import (
	"math"

	"github.com/google/uuid"
	"github.com/benoitpetit/mira/store"
	"github.com/benoitpetit/mira/types"
)

// SQLiteAdapter adapts SQLite store for VectorStore interface
type SQLiteAdapter struct {
	store *store.Store
}

// NewSQLiteAdapter creates a new adapter
func NewSQLiteAdapter(s *store.Store) *SQLiteAdapter {
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

	// Sort by descending score
	for i := 0; i < len(scoredCandidates); i++ {
		for j := i + 1; j < len(scoredCandidates); j++ {
			if scoredCandidates[j].score > scoredCandidates[i].score {
				scoredCandidates[i], scoredCandidates[j] = scoredCandidates[j], scoredCandidates[i]
			}
		}
	}

	// Return best
	result := make([]*types.Candidate, 0, min(limit, len(scoredCandidates)))
	for i := 0; i < min(limit, len(scoredCandidates)); i++ {
		result = append(result, scoredCandidates[i].cand)
	}

	return result, nil
}

// cosineSimilarity computes cosine similarity
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot float64
	for i := range a {
		dot += float64(a[i] * b[i])
	}
	return dot // Pre-normalized vectors
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SimpleOverlapCache simple overlap cache implementation
type SimpleOverlapCache struct {
	data map[string]float64
}

// NewSimpleOverlapCache creates a new overlap cache
func NewSimpleOverlapCache() *SimpleOverlapCache {
	return &SimpleOverlapCache{
		data: make(map[string]float64),
	}
}

func (c *SimpleOverlapCache) key(idA, idB uuid.UUID) string {
	// Consistent order for key
	if idA.String() < idB.String() {
		return idA.String() + ":" + idB.String()
	}
	return idB.String() + ":" + idA.String()
}

// Get retrieves value from cache
func (c *SimpleOverlapCache) Get(idA, idB uuid.UUID) (float64, bool) {
	val, ok := c.data[c.key(idA, idB)]
	return val, ok
}

// Set stores value in cache
func (c *SimpleOverlapCache) Set(idA, idB uuid.UUID, similarity float64) {
	c.data[c.key(idA, idB)] = similarity
}

// CosineDistance computes cosine distance (1 - similarity)
func CosineDistance(a, b []float32) float64 {
	return 1 - cosineSimilarity(a, b)
}

// Normalize normalizes a vector (L2)
func Normalize(v []float32) []float32 {
	var norm float64
	for _, x := range v {
		norm += float64(x * x)
	}
	norm = math.Sqrt(norm)

	if norm == 0 {
		return v
	}

	result := make([]float32, len(v))
	for i, x := range v {
		result[i] = float32(float64(x) / norm)
	}
	return result
}
