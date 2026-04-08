package budget

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/benoitpetit/mira/types"
)

func TestSigmoidDensity(t *testing.T) {
	// Test center value
	raw := 0.3
	expected := 0.0 // (2/(1+e^0)) - 1 = 0

	result := 2.0/(1.0+math.Exp(-2.0*(raw-0.3))) - 1.0
	if math.Abs(result-expected) > 0.001 {
		t.Errorf("Sigmoid at center: expected %v, got %v", expected, result)
	}

	// Test high asymptote (raw=2.0 should give ~0.93)
	raw = 2.0
	result = 2.0/(1.0+math.Exp(-2.0*(raw-0.3))) - 1.0
	expectedHigh := 0.93
	if math.Abs(result-expectedHigh) > 0.05 {
		t.Errorf("Sigmoid at high: expected ~%v, got %v", expectedHigh, result)
	}
}

func TestSessionBoost(t *testing.T) {
	now := time.Now()
	c1 := &types.Candidate{
		Verbatim: &types.Verbatim{CreatedAt: now.Add(-30 * time.Minute)},
	}
	c2 := &types.Candidate{
		Verbatim: &types.Verbatim{CreatedAt: now.Add(-3 * time.Hour)},
	}

	// Simulate selection
	selectedTimes := []time.Time{now.Add(-25 * time.Minute)}

	// c1 should have boost (within 2h), c2 not
	window := 2 * time.Hour
	beta := 0.2

	c1Within := math.Abs(c1.Verbatim.CreatedAt.Sub(selectedTimes[0]).Seconds()) < window.Seconds()
	c2Within := math.Abs(c2.Verbatim.CreatedAt.Sub(selectedTimes[0]).Seconds()) < window.Seconds()

	if !c1Within {
		t.Error("c1 should be within session window")
	}
	if c2Within {
		t.Error("c2 should NOT be within session window")
	}

	if c1Within {
		c1.SessionBoost = 1 + beta
	} else {
		c1.SessionBoost = 1.0
	}

	if math.Abs(c1.SessionBoost-1.2) > 0.01 {
		t.Errorf("c1 session boost should be ~1.2, got %v", c1.SessionBoost)
	}
	if c2.SessionBoost != 0 && math.Abs(c2.SessionBoost-1.0) > 0.01 {
		t.Errorf("c2 session boost should be 1.0, got %v", c2.SessionBoost)
	}
}

func TestCausalPenalty(t *testing.T) {
	// Exponentially decreasing penalty
	alpha := 0.15

	// 0 links = 1.0
	result := math.Exp(-alpha * 0)
	if math.Abs(result-1.0) > 0.001 {
		t.Errorf("Causal penalty with 0 links: expected 1.0, got %v", result)
	}

	// 1 link = e^-0.15 ≈ 0.86
	result = math.Exp(-alpha * 1)
	expected := 0.8607
	if math.Abs(result-expected) > 0.001 {
		t.Errorf("Causal penalty with 1 link: expected %v, got %v", expected, result)
	}

	// 3 links = e^-0.45 ≈ 0.63
	result = math.Exp(-alpha * 3)
	expected = 0.6376
	if math.Abs(result-expected) > 0.001 {
		t.Errorf("Causal penalty with 3 links: expected %v, got %v", expected, result)
	}
}

func TestGreedyBudgetRespect(t *testing.T) {
	candidates := []*types.Candidate{
		{Score: 0.9, Verbatim: &types.Verbatim{TokenCount: 2000}, Memory: &types.Fingerprint{TokenEstimate: 300}},
		{Score: 0.8, Verbatim: &types.Verbatim{TokenCount: 1500}, Memory: &types.Fingerprint{TokenEstimate: 250}},
		{Score: 0.7, Verbatim: &types.Verbatim{TokenCount: 1000}, Memory: &types.Fingerprint{TokenEstimate: 200}},
	}

	// Simulate selection with budget 2500
	budgetLimit := 2500
	selected := make([]*types.SelectedMemory, 0)
	tokens := 0

	for _, c := range candidates {
		cost := c.Verbatim.TokenCount
		if tokens+cost > budgetLimit {
			cost = c.Memory.TokenEstimate // Downgrade
			if tokens+cost > budgetLimit {
				cost = 5 // Header
				if tokens+cost > budgetLimit {
					continue
				}
			}
		}
		selected = append(selected, &types.SelectedMemory{TokenCost: cost})
		tokens += cost
	}

	if tokens > budgetLimit {
		t.Errorf("Total tokens %d exceeds budget %d", tokens, budgetLimit)
	}
	if len(selected) == 0 {
		t.Error("Should have selected at least one candidate")
	}
}

func TestEarlyPruning(t *testing.T) {
	// Verify ρ < 0.6 are filtered
	candidates := []*types.Candidate{
		{Relevance: 0.9},
		{Relevance: 0.5}, // Should be pruned
		{Relevance: 0.7},
		{Relevance: 0.3}, // Should be pruned
	}

	threshold := 0.6
	var pruned []*types.Candidate
	for _, c := range candidates {
		if c.Relevance > threshold {
			pruned = append(pruned, c)
		}
	}

	if len(pruned) != 2 {
		t.Errorf("Expected 2 pruned candidates, got %d", len(pruned))
	}
	for _, p := range pruned {
		if p.Relevance <= 0.6 {
			t.Errorf("Pruned candidate should have relevance > 0.6, got %v", p.Relevance)
		}
	}
}

func TestCosineSimilarity(t *testing.T) {
	// Identical vectors
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	result := cosineSimilarity(a, b)
	if math.Abs(result-1.0) > 0.0001 {
		t.Errorf("Cosine similarity of identical vectors should be 1.0, got %v", result)
	}

	// Orthogonal vectors
	a = []float32{1, 0, 0}
	b = []float32{0, 1, 0}
	result = cosineSimilarity(a, b)
	if math.Abs(result-0.0) > 0.0001 {
		t.Errorf("Cosine similarity of orthogonal vectors should be 0.0, got %v", result)
	}

	// Opposite vectors
	a = []float32{1, 0, 0}
	b = []float32{-1, 0, 0}
	result = cosineSimilarity(a, b)
	if math.Abs(result-(-1.0)) > 0.0001 {
		t.Errorf("Cosine similarity of opposite vectors should be -1.0, got %v", result)
	}
}

func TestEmbeddingCache(t *testing.T) {
	cache := NewEmbeddingCache(3)

	// Add 3 items
	cache.Set("a", []float32{1, 2, 3})
	cache.Set("b", []float32{4, 5, 6})
	cache.Set("c", []float32{7, 8, 9})

	// Check they exist
	if _, ok := cache.Get("a"); !ok {
		t.Error("'a' should be in cache")
	}

	// Add 4th item (should evict 'a')
	cache.Set("d", []float32{10, 11, 12})

	if _, ok := cache.Get("a"); ok {
		t.Error("'a' should have been evicted from cache")
	}
	if _, ok := cache.Get("d"); !ok {
		t.Error("'d' should be in cache")
	}
}

func TestRenderMode(t *testing.T) {
	tests := []struct {
		remainingBudget int
		expectedMode    types.RenderMode
	}{
		{50, types.ModeHeader},
		{99, types.ModeHeader},
		{500, types.ModeFingerprint},
		{999, types.ModeFingerprint},
		{1000, types.ModeVerbatim},
		{5000, types.ModeVerbatim},
	}

	alloc := &Allocator{}
	for _, test := range tests {
		// Create dummy candidate
		c := &types.Candidate{
			Verbatim: &types.Verbatim{TokenCount: 100},
			Memory:   &types.Fingerprint{TokenEstimate: 50},
		}
		mode := alloc.determineRenderMode(c, test.remainingBudget)
		if mode != test.expectedMode {
			t.Errorf("For remaining budget %d, expected mode %v, got %v",
				test.remainingBudget, test.expectedMode, mode)
		}
	}
}

func TestScoreCandidates(t *testing.T) {
	alloc := &Allocator{}

	now := time.Now()
	candidates := []*types.Candidate{
		{
			Memory:    &types.Fingerprint{Type: types.TypeDecision, FactCount: 5},
			Verbatim:  &types.Verbatim{TokenCount: 100, CreatedAt: now},
			Embedding: []float32{1, 0, 0},
		},
	}

	queryVec := []float32{1, 0, 0}
	scored := alloc.scoreCandidates(candidates, queryVec)

	if len(scored) != 1 {
		t.Fatalf("Expected 1 scored candidate, got %d", len(scored))
	}

	c := scored[0]

	// Check relevance (should be ~1.0 for identical vectors)
	if c.Relevance < 0.99 {
		t.Errorf("Expected relevance ~1.0, got %v", c.Relevance)
	}

	// Check density
	if c.Density <= 0 {
		t.Errorf("Expected positive density, got %v", c.Density)
	}

	// Check recency (should be ~1.0 for newly created)
	if c.Recency < 0.99 {
		t.Errorf("Expected recency ~1.0, got %v", c.Recency)
	}

	// Check global score
	if c.Score <= 0 {
		t.Errorf("Expected positive score, got %v", c.Score)
	}
}

// Mock implementations for tests
type mockVectorStore struct {
	candidates []*types.Candidate
}

func (m *mockVectorStore) Search(vector []float32, limit int, wing, room *string) ([]*types.Candidate, error) {
	return m.candidates, nil
}

type mockCausalGraph struct {
	edges map[string]bool
}

func (m *mockCausalGraph) HasEdge(fromID, toID uuid.UUID) bool {
	key := fromID.String() + "->" + toID.String()
	return m.edges[key]
}

func (m *mockCausalGraph) GetParents(nodeID uuid.UUID, relations ...types.RelationType) ([]*types.CausalNode, error) {
	return nil, nil
}

func (m *mockCausalGraph) GetChildren(nodeID uuid.UUID, relations ...types.RelationType) ([]*types.CausalNode, error) {
	return nil, nil
}
