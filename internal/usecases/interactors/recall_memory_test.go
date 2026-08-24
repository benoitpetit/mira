package interactors

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/benoitpetit/mira/internal/util"
	"github.com/google/uuid"
)

// Mock implementations
type mockRecallEmbedder struct {
	encodeFunc func(ctx context.Context, text string) ([]float32, error)
}

func (m *mockRecallEmbedder) Encode(ctx context.Context, text string) ([]float32, error) {
	if m.encodeFunc != nil {
		return m.encodeFunc(ctx, text)
	}
	vec := make([]float32, 384)
	for i := range vec {
		vec[i] = float32(len(text)) / 100.0
	}
	return vec, nil
}

type mockRecallOverlapCache struct {
	data map[string]float64
}

func (m *mockRecallOverlapCache) Get(ctx context.Context, idA, idB uuid.UUID) (float64, bool) {
	if m.data == nil {
		return 0.0, false
	}
	key := idA.String() + ":" + idB.String()
	val, ok := m.data[key]
	return val, ok
}

func (m *mockRecallOverlapCache) Set(ctx context.Context, idA, idB uuid.UUID, similarity float64) {
	if m.data == nil {
		m.data = make(map[string]float64)
	}
	key := idA.String() + ":" + idB.String()
	m.data[key] = similarity
}

type mockRecallCausalGraph struct {
	edges map[string]bool
}

func (m *mockRecallCausalGraph) HasEdge(ctx context.Context, fromID, toID uuid.UUID) bool {
	if m.edges == nil {
		return false
	}
	key := fromID.String() + ":" + toID.String()
	return m.edges[key]
}

func (m *mockRecallCausalGraph) GetParents(ctx context.Context, nodeID uuid.UUID, relations ...valueobjects.RelationType) ([]*entities.CausalNode, error) {
	return nil, nil
}

func (m *mockRecallCausalGraph) GetChildren(ctx context.Context, nodeID uuid.UUID, relations ...valueobjects.RelationType) ([]*entities.CausalNode, error) {
	return nil, nil
}

type mockRecallVectorStore struct {
	candidates []*entities.Candidate
	searchFunc func(ctx context.Context, vector []float32, limit int, wing, room *string) ([]*entities.Candidate, error)
}

func (m *mockRecallVectorStore) Search(ctx context.Context, vector []float32, limit int, wing, room *string) ([]*entities.Candidate, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, vector, limit, wing, room)
	}
	return m.candidates, nil
}

func (m *mockRecallVectorStore) SearchLexical(ctx context.Context, query string, limit int, wing, room *string) ([]*entities.Candidate, error) {
	return nil, nil
}

func (m *mockRecallVectorStore) AddCandidate(ctx context.Context, candidate *entities.Candidate) error {
	return nil
}

func (m *mockRecallVectorStore) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockRecallVectorStore) ClearAll(ctx context.Context) error {
	return nil
}

func (m *mockRecallVectorStore) ClearByRoom(ctx context.Context, wing string, room *string) error {
	return nil
}

type mockRecallRenderer struct{}

func (m *mockRecallRenderer) RenderHeader(candidate *entities.Candidate) string {
	return "Header: " + candidate.ID().String()
}

func (m *mockRecallRenderer) RenderFingerprint(candidate *entities.Candidate) string {
	return "Fingerprint: " + candidate.ID().String()
}

// mockRecallMetricsCollector is a mock for the metrics collector
type mockRecallMetricsCollector struct{}

func (m *mockRecallMetricsCollector) IsEnabled() bool { return false }
func (m *mockRecallMetricsCollector) RecordStore(duration time.Duration) {}
func (m *mockRecallMetricsCollector) RecordRecall(duration time.Duration) {}
func (m *mockRecallMetricsCollector) RecordSearch(duration time.Duration, usedHNSW bool) {}
func (m *mockRecallMetricsCollector) RecordEmbed(duration time.Duration) {}
func (m *mockRecallMetricsCollector) GetReport(ctx context.Context) ports.MetricsReport {
	return ports.MetricsReport{}
}
func (m *mockRecallMetricsCollector) RecordError() {}
func (m *mockRecallMetricsCollector) RecordStoreResult(factCount int) {}
func (m *mockRecallMetricsCollector) RecordRecallResult(selectedCount int, budgetUsed float64) {}
func (m *mockRecallMetricsCollector) UpdateMemoryCount(count int) {}
func (m *mockRecallMetricsCollector) UpdateVectorCount(count int) {}

// createTestInteractor creates a RecallMemory interactor with mock dependencies
func createTestInteractor(candidates []*entities.Candidate) *RecallMemory {
	vectorStore := &mockRecallVectorStore{candidates: candidates}
	cache := &mockRecallOverlapCache{}
	graph := &mockRecallCausalGraph{}
	embedder := &mockRecallEmbedder{}
	renderer := &mockRecallRenderer{}
	metrics := &mockRecallMetricsCollector{}

	config := DefaultRecallMemoryConfig()
	return NewRecallMemory(vectorStore, cache, graph, embedder, renderer, config, metrics, nil)
}

// TestExecute tests end-to-end RecallMemory use case
func TestExecute(t *testing.T) {
	now := time.Now()

	// Create test candidates with varying properties
	candidates := []*entities.Candidate{
		createTestCandidateWithScore("high-relevance", now, 0.95, 100),
		createTestCandidateWithScore("medium-relevance", now.Add(-1*time.Hour), 0.75, 80),
		createTestCandidateWithScore("low-relevance", now.Add(-24*time.Hour), 0.55, 50),
	}

	interactor := createTestInteractor(candidates)
	interactor.earlyPruningThreshold = 0.5 // Lower threshold to include more candidates

	tests := []struct {
		name           string
		input          RecallMemoryInput
		expectResults  bool
		maxBudgetUsed  float64
		minMemories    int
	}{
		{
			name: "successful recall with budget",
			input: RecallMemoryInput{
				Query:  "test query",
				Budget: 500,
			},
			expectResults: true,
			maxBudgetUsed: 100.0,
			minMemories:   1,
		},
		{
			name: "recall with default budget",
			input: RecallMemoryInput{
				Query:  "another query",
				Budget: 0, // Should use default
			},
			expectResults: true,
			maxBudgetUsed: 100.0,
			minMemories:   1,
		},
		{
			name: "recall with wing filter",
			input: RecallMemoryInput{
				Query:  "filtered query",
				Budget: 500,
				Wing:   strPtr("test-wing"),
			},
			expectResults: true,
			maxBudgetUsed: 100.0,
			minMemories:   1,
		},
		{
			name: "recall with room filter",
			input: RecallMemoryInput{
				Query:  "room filtered query",
				Budget: 500,
				Room:   strPtr("test-room"),
			},
			expectResults: true,
			maxBudgetUsed: 100.0,
			minMemories:   1,
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := interactor.Execute(ctx, tt.input)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			if tt.expectResults {
				if output == nil {
					t.Error("Expected output, got nil")
					return
				}

				// Verify memories are selected
				if len(output.Memories) < tt.minMemories {
					t.Errorf("Expected at least %d memories, got %d", tt.minMemories, len(output.Memories))
				}

				// Verify budget is respected
				if output.BudgetUsed > tt.maxBudgetUsed {
					t.Errorf("Budget used (%.1f%%) exceeds maximum (%.1f%%)", output.BudgetUsed, tt.maxBudgetUsed)
				}

				// Verify total tokens
				if output.TotalTokens < 0 {
					t.Error("Total tokens should be non-negative")
				}

				// Verify each selected memory has required fields
				for i, mem := range output.Memories {
					if mem.CandidateID == uuid.Nil {
						t.Errorf("Memory %d has nil CandidateID", i)
					}
					if mem.VerbatimID == uuid.Nil {
						t.Errorf("Memory %d has nil VerbatimID", i)
					}
					if mem.TokenCost <= 0 {
						t.Errorf("Memory %d has invalid TokenCost: %d", i, mem.TokenCost)
					}
					// Rendered content can be empty for some modes, so we don't check
				}
			}
		})
	}
}

// TestScoreCandidatesDetailed tests detailed scoring calculations
func TestScoreCandidatesDetailed(t *testing.T) {
	interactor := createTestInteractor(nil)

	now := time.Now()

	tests := []struct {
		name           string
		candidate      *entities.Candidate
		queryVec       []float32
		checkRelevance bool
		checkDensity   bool
		checkRecency   bool
		recencyMin     float64
		recencyMax     float64
	}{
		{
			name:           "high fact count, recent",
			candidate:      createTestCandidateWithData("1", now.Add(-1*time.Hour), 50, 100),
			queryVec:       createQueryVector(1.0),
			checkRelevance: true,
			checkDensity:   true,
			checkRecency:   true,
			recencyMin:     0.9, // Very recent
			recencyMax:     1.0,
		},
		{
			name:           "low fact count, old",
			candidate:      createTestCandidateWithData("2", now.Add(-30*24*time.Hour), 2, 500),
			queryVec:       createQueryVector(0.5),
			checkRelevance: true,
			checkDensity:   true,
			checkRecency:   true,
			recencyMin:     0.0,
			recencyMax:     0.5, // Old memory
		},
		{
			name:           "medium density with decision type",
			candidate:      createTestCandidateWithType("3", now.Add(-7*24*time.Hour), 20, 200, valueobjects.TypeDecision),
			queryVec:       createQueryVector(0.8),
			checkRelevance: true,
			checkDensity:   true,
			checkRecency:   true,
			recencyMin:     0.0,
			recencyMax:     1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidates := []*entities.Candidate{tt.candidate}
			scored := interactor.scoreCandidates(candidates, tt.queryVec, nil)

			if len(scored) != 1 {
				t.Fatalf("Expected 1 scored candidate, got %d", len(scored))
			}

			c := scored[0]

			// Verify Relevance is in valid range
			if tt.checkRelevance {
				if c.Relevance < 0 || c.Relevance > 1 {
					t.Errorf("Relevance = %f, expected [0, 1]", c.Relevance)
				}
			}

			// Verify Density is in valid range
			if tt.checkDensity {
				if c.Density < 0 || c.Density > 1 {
					t.Errorf("Density = %f, expected [0, 1]", c.Density)
				}
			}

			// Verify Recency is in expected range
			if tt.checkRecency {
				if c.Recency < tt.recencyMin || c.Recency > tt.recencyMax {
					t.Errorf("Recency = %f, expected [%f, %f]", c.Recency, tt.recencyMin, tt.recencyMax)
				}
			}

			// Verify Score = Relevance * Density * Recency
			expectedScore := c.Relevance * c.Density * c.Recency
			if math.Abs(c.Score-expectedScore) > 0.0001 {
				t.Errorf("Score = %f, expected %f (Relevance * Density * Recency)", c.Score, expectedScore)
			}

			// Verify all scores are non-negative
			if c.Relevance < 0 || c.Density < 0 || c.Recency < 0 || c.Score < 0 {
				t.Error("All scores should be non-negative")
			}
		})
	}
}

// TestSelectGreedy tests the greedy selection algorithm
func TestSelectGreedy(t *testing.T) {
	interactor := createTestInteractor(nil)

	now := time.Now()

	tests := []struct {
		name              string
		candidates        []*entities.Candidate
		budget            int
		expectResults     bool
		expectedMaxTokens int
		checkNoDuplicates bool
	}{
		{
			name: "select highest score candidates first",
			candidates: []*entities.Candidate{
				createTestCandidateWithScoreAndTokens("low", now, 0.3, 50),
				createTestCandidateWithScoreAndTokens("high", now, 0.9, 50),
				createTestCandidateWithScoreAndTokens("medium", now, 0.6, 50),
			},
			budget:            200,
			expectResults:     true,
			expectedMaxTokens: 200,
			checkNoDuplicates: true,
		},
		{
			name: "respect budget limit",
			candidates: []*entities.Candidate{
				createTestCandidateWithScoreAndTokens("a", now, 0.9, 100),
				createTestCandidateWithScoreAndTokens("b", now, 0.8, 100),
				createTestCandidateWithScoreAndTokens("c", now, 0.7, 100),
			},
			budget:            250,
			expectResults:     true,
			expectedMaxTokens: 250,
			checkNoDuplicates: true,
		},
		{
			name: "low relevance fallback",
			candidates: []*entities.Candidate{
				// Add one candidate that will be pruned (below threshold)
				// but the fallback will still include it
				createTestCandidateWithRelevance("low", now, 0.1),
			},
			budget:            500,
			expectResults:     true, // Fallback keeps top candidates
			expectedMaxTokens: 500,
			checkNoDuplicates: true,
		},
		{
			name: "budget too small for any candidate",
			candidates: []*entities.Candidate{
				createTestCandidateWithScoreAndTokens("expensive", now, 0.9, 200),
			},
			budget:            10,
			expectResults:     false,
			expectedMaxTokens: 0,
			checkNoDuplicates: true,
		},
		{
			name: "many candidates with budget constraint",
			candidates: []*entities.Candidate{
				createTestCandidateWithScoreAndTokens("1", now, 0.95, 30),
				createTestCandidateWithScoreAndTokens("2", now, 0.90, 30),
				createTestCandidateWithScoreAndTokens("3", now, 0.85, 30),
				createTestCandidateWithScoreAndTokens("4", now, 0.80, 30),
				createTestCandidateWithScoreAndTokens("5", now, 0.75, 30),
			},
			budget:            100,
			expectResults:     true,
			expectedMaxTokens: 100,
			checkNoDuplicates: true,
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Pre-score candidates
			queryVec := createQueryVector(1.0)
			scored := interactor.scoreCandidates(tt.candidates, queryVec, nil)
			pruned := interactor.pruneCandidates(scored)

			selected := interactor.selectGreedy(ctx, pruned, tt.budget, nil)

			// Verify budget is respected
			totalTokens := 0
			for _, s := range selected {
				totalTokens += s.TokenCost
			}

			if totalTokens > tt.budget {
				t.Errorf("Total tokens (%d) exceeds budget (%d)", totalTokens, tt.budget)
			}

			if totalTokens > tt.expectedMaxTokens {
				t.Errorf("Total tokens (%d) exceeds expected max (%d)", totalTokens, tt.expectedMaxTokens)
			}

			// Verify no duplicates
			if tt.checkNoDuplicates {
				seen := make(map[uuid.UUID]bool)
				for _, s := range selected {
					if seen[s.CandidateID] {
						t.Errorf("Duplicate candidate ID: %s", s.CandidateID)
					}
					seen[s.CandidateID] = true
					if s.VerbatimID == uuid.Nil {
						t.Errorf("Nil VerbatimID for candidate: %s", s.CandidateID)
					}
				}
			}

			// Verify expected results
			if tt.expectResults && len(selected) == 0 {
				t.Error("Expected results but got none")
			}
		})
	}
}

// TestEmbeddingCacheDetailed tests the LRU embedding cache thoroughly
func TestEmbeddingCacheDetailed(t *testing.T) {
	tests := []struct {
		name    string
		maxSize int
		ops     []cacheOp
	}{
		{
			name:    "basic add and get",
			maxSize: 3,
			ops: []cacheOp{
				{op: "set", key: "a", vec: []float32{1.0, 2.0}},
				{op: "get", key: "a", expectFound: true, expectVec: []float32{1.0, 2.0}},
			},
		},
		{
			name:    "eviction when max size reached",
			maxSize: 2,
			ops: []cacheOp{
				{op: "set", key: "a", vec: []float32{1.0}},
				{op: "set", key: "b", vec: []float32{2.0}},
				{op: "set", key: "c", vec: []float32{3.0}}, // Should evict "a"
				{op: "get", key: "a", expectFound: false},
				{op: "get", key: "b", expectFound: true},
				{op: "get", key: "c", expectFound: true},
			},
		},
		{
			name:    "no duplicates on update",
			maxSize: 2,
			ops: []cacheOp{
				// After each set, the order is updated
				{op: "set", key: "a", vec: []float32{1.0}}, // order: [a]
				{op: "set", key: "a", vec: []float32{1.1}}, // a already exists, moves to end: [a]
				{op: "set", key: "b", vec: []float32{2.0}}, // order: [a, b]
				{op: "set", key: "c", vec: []float32{3.0}}, // Evicts "a" (at front): [b, c]
				{op: "get", key: "a", expectFound: false},
				{op: "get", key: "b", expectFound: true},
				{op: "get", key: "c", expectFound: true},
			},
		},
		{
			name:    "access updates LRU order",
			maxSize: 2,
			ops: []cacheOp{
				{op: "set", key: "a", vec: []float32{1.0}}, // order (MRU→LRU): [a]
				{op: "set", key: "b", vec: []float32{2.0}}, // order: [b, a]
				// get("a") promotes "a" to MRU: order becomes [a, b]
				{op: "get", key: "a", expectFound: true},
				// Adding "c" evicts the LRU entry, which is now "b"
				{op: "set", key: "c", vec: []float32{3.0}}, // evicts b → [c, a]
				{op: "get", key: "a", expectFound: true},   // "a" is still present
				{op: "get", key: "b", expectFound: false},  // "b" was evicted
				{op: "get", key: "c", expectFound: true},
			},
		},
		{
			name:    "multiple updates same key",
			maxSize: 2,
			ops: []cacheOp{
				{op: "set", key: "a", vec: []float32{1.0}},
				{op: "set", key: "a", vec: []float32{1.1}},
				{op: "set", key: "a", vec: []float32{1.2}},
				{op: "set", key: "a", vec: []float32{1.3}},
				{op: "get", key: "a", expectFound: true, expectVec: []float32{1.3}},
			},
		},
		{
			name:    "cache size 1",
			maxSize: 1,
			ops: []cacheOp{
				{op: "set", key: "a", vec: []float32{1.0}},
				{op: "set", key: "b", vec: []float32{2.0}}, // Evicts "a"
				{op: "get", key: "a", expectFound: false},
				{op: "get", key: "b", expectFound: true},
				{op: "set", key: "c", vec: []float32{3.0}}, // Evicts "b"
				{op: "get", key: "b", expectFound: false},
				{op: "get", key: "c", expectFound: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := newEmbeddingCache(tt.maxSize)

			for _, op := range tt.ops {
				switch op.op {
				case "set":
					cache.set(op.key, op.vec)
				case "get":
					vec, found := cache.get(op.key)
					if found != op.expectFound {
						t.Errorf("get(%s) found=%v, expected %v", op.key, found, op.expectFound)
					}
					if op.expectFound && op.expectVec != nil {
						if len(vec) != len(op.expectVec) {
							t.Errorf("get(%s) vec length=%d, expected %d", op.key, len(vec), len(op.expectVec))
						} else {
							for i := range vec {
								if vec[i] != op.expectVec[i] {
									t.Errorf("get(%s) vec[%d]=%v, expected %v", op.key, i, vec[i], op.expectVec[i])
									break
								}
							}
						}
					}
				}
			}
		})
	}
}

type cacheOp struct {
	op          string
	key         string
	vec         []float32
	expectFound bool
	expectVec   []float32
}

// TestCosineSimilarity tests the cosine similarity function
func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float64
	}{
		{
			name:     "identical vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 1.0,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{-1, 0, 0},
			expected: -1.0,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{0, 1, 0},
			expected: 0.0,
		},
		{
			name:     "45 degree angle",
			a:        []float32{1, 0},
			b:        []float32{1, 1},
			expected: 1.0 / math.Sqrt(2),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := util.CosineSimilarity(tt.a, tt.b)
			if math.Abs(result-tt.expected) > 0.0001 {
				t.Errorf("CosineSimilarity() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestScoreCandidates tests candidate scoring
func TestScoreCandidates(t *testing.T) {
	interactor := createTestInteractor(nil)

	// Create test candidates
	now := time.Now()
	candidates := []*entities.Candidate{
		createTestCandidateWithData("1", now.Add(-1*time.Hour), 10, 100),
		createTestCandidateWithData("2", now.Add(-24*time.Hour), 20, 200),
	}

	queryVec := make([]float32, 384)
	queryVec[0] = 1.0

	scored := interactor.scoreCandidates(candidates, queryVec, nil)

	// Verify all candidates have scores
	for _, c := range scored {
		if c.Relevance < 0 || c.Relevance > 1 {
			t.Errorf("Relevance out of range: %f", c.Relevance)
		}
		if c.Density < 0 || c.Density > 1 {
			t.Errorf("Density out of range: %f", c.Density)
		}
		if c.Recency <= 0 || c.Recency > 1 {
			t.Errorf("Recency out of range: %f", c.Recency)
		}
		if c.Score <= 0 {
			t.Errorf("Score should be positive: %f", c.Score)
		}
	}
}

// TestAdaptiveThreshold tests dynamic threshold computation
func TestAdaptiveThreshold(t *testing.T) {
	interactor := createTestInteractor(nil)

	tests := []struct {
		name     string
		scores   []float64
		wantMin  float64
		wantMax  float64
	}{
		{"sparse corpus", []float64{0.9, 0.8}, 0.3, 0.3}, // < 3 scores -> fixed 0.3
		{"tight cluster", []float64{0.6, 0.62, 0.58, 0.61}, 0.15, 0.75},
		{"spread out", []float64{0.9, 0.8, 0.5, 0.4, 0.3}, 0.15, 0.75},
		{"high floor", []float64{0.95, 0.94, 0.96}, 0.15, 0.75},
		{"low ceiling", []float64{0.1, 0.12, 0.11}, 0.15, 0.15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := interactor.adaptiveThreshold(tt.scores)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("adaptiveThreshold(%v) = %f, want between %f and %f", tt.scores, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// TestPruneCandidates tests the early pruning logic
func TestPruneCandidates(t *testing.T) {
	interactor := createTestInteractor(nil)
	interactor.earlyPruningThreshold = 0.5

	now := time.Now()
	candidates := []*entities.Candidate{
		createTestCandidateWithRelevance("1", now, 0.8),
		createTestCandidateWithRelevance("2", now, 0.6),
		createTestCandidateWithRelevance("3", now, 0.3),
	}

	pruned := interactor.pruneCandidates(candidates)

	// Should keep candidates above threshold (0.8 and 0.6)
	if len(pruned) != 2 {
		t.Errorf("Expected 2 pruned candidates, got %d", len(pruned))
	}
}

// TestExecute_ExtremeBudgets tests edge cases for budget parameter
func TestExecute_ExtremeBudgets(t *testing.T) {
	now := time.Now()
	candidates := []*entities.Candidate{
		createTestCandidateWithScoreAndTokens("high", now, 0.9, 30),
	}
	interactor := createTestInteractor(candidates)
	ctx := context.Background()

	tests := []struct {
		name         string
		budget       int
		expectZero   bool
	}{
		{"budget zero uses default", 0, false},
		{"budget one too small", 1, true},
		{"budget below minimum loop", 40, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := interactor.Execute(ctx, RecallMemoryInput{Query: "test", Budget: tt.budget})
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}
			if tt.expectZero && len(output.Memories) != 0 {
				t.Errorf("expected 0 memories for budget=%d, got %d", tt.budget, len(output.Memories))
			}
		})
	}
}

// TestCalculateTokenCost tests token cost calculation
func TestCalculateTokenCost(t *testing.T) {
	interactor := createTestInteractor(nil)

	c := &entities.Candidate{
		Verbatim: &entities.Verbatim{
			TokenCount: 100,
		},
		Memory: &entities.Fingerprint{
			TokenEstimate: 50,
		},
	}

	tests := []struct {
		mode     valueobjects.RenderMode
		expected int
	}{
		{valueobjects.ModeHeader, 5},
		{valueobjects.ModeFingerprint, 50},
		{valueobjects.ModeVerbatim, 100},
	}

	for _, tt := range tests {
		result := interactor.calculateTokenCost(c, tt.mode)
		if result != tt.expected {
			t.Errorf("calculateTokenCost(%v) = %d, want %d", tt.mode, result, tt.expected)
		}
	}
}

// TestEmbeddingCache tests the LRU embedding cache
func TestEmbeddingCache(t *testing.T) {
	cache := newEmbeddingCache(2)

	// Add items
	vec1 := []float32{1.0, 2.0, 3.0}
	vec2 := []float32{4.0, 5.0, 6.0}
	vec3 := []float32{7.0, 8.0, 9.0}

	cache.set("key1", vec1)
	cache.set("key2", vec2)

	// Retrieve key1 — this promotes it to most-recently-used.
	// After this get(), LRU order is: key2 (oldest) → key1 (newest).
	if v, ok := cache.get("key1"); !ok {
		t.Error("Expected to find key1")
	} else if len(v) != 3 || v[0] != 1.0 {
		t.Error("Retrieved wrong value for key1")
	}

	// Add third item: cache is full, so the LRU entry (key2) is evicted.
	cache.set("key3", vec3)

	// key2 should be evicted (it was least-recently used after the get above)
	if _, ok := cache.get("key2"); ok {
		t.Error("Expected key2 to be evicted (LRU)")
	}

	// key1 and key3 should still exist
	if _, ok := cache.get("key1"); !ok {
		t.Error("Expected to find key1")
	}
	if _, ok := cache.get("key3"); !ok {
		t.Error("Expected to find key3")
	}
}

// TestDetermineRenderMode tests render mode determination
func TestDetermineRenderMode(t *testing.T) {
	interactor := createTestInteractor(nil)

	tests := []struct {
		budget   int
		expected valueobjects.RenderMode
	}{
		{50, valueobjects.ModeHeader},
		{500, valueobjects.ModeFingerprint},
		{2000, valueobjects.ModeVerbatim},
	}

	for _, tt := range tests {
		result := interactor.determineRenderMode(tt.budget)
		if result != tt.expected {
			t.Errorf("determineRenderMode(%d) = %v, want %v", tt.budget, result, tt.expected)
		}
	}
}

// TestConcurrency tests thread-safety of embedding cache
func TestConcurrency(t *testing.T) {
	cache := newEmbeddingCache(100)
	vec := make([]float32, 384)

	// Concurrent writes
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			for j := 0; j < 100; j++ {
				key := string(rune('a' + idx))
				cache.set(key, vec)
			}
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func(idx int) {
			for j := 0; j < 100; j++ {
				key := string(rune('a' + idx))
				cache.get(key)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	// Cache should still be functional
	cache.set("final", []float32{1.0, 2.0, 3.0})
	if v, ok := cache.get("final"); !ok || v[0] != 1.0 {
		t.Error("Cache not functional after concurrent access")
	}
}

// Benchmarks
func BenchmarkCosineSimilarity(b *testing.B) {
	a := make([]float32, 384)
	b_vec := make([]float32, 384)
	for i := 0; i < 384; i++ {
		a[i] = float32(i) / 384.0
		b_vec[i] = float32(384-i) / 384.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		util.CosineSimilarity(a, b_vec)
	}
}

func BenchmarkEmbeddingCache(b *testing.B) {
	cache := newEmbeddingCache(1000)
	vec := make([]float32, 384)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune(i % 100))
		cache.set(key, vec)
		cache.get(key)
	}
}

// BenchmarkSelectGreedy benchmarks the greedy selection algorithm
func BenchmarkSelectGreedy(b *testing.B) {
	uc := createTestInteractor(nil)
	now := time.Now()

	// Create 100 candidates with varying scores
	candidates := make([]*entities.Candidate, 100)
	for i := 0; i < 100; i++ {
		score := 0.95 - float64(i)*0.009 // Decreasing scores from 0.95 to 0.05
		candidates[i] = createTestCandidateWithScoreAndTokens(
			string(rune('a'+i%26))+string(rune('0'+i/26)),
			now.Add(-time.Duration(i)*time.Hour),
			score,
			50,
		)
	}

	queryVec := createQueryVector(1.0)
	scored := uc.scoreCandidates(candidates, queryVec, nil)
	pruned := uc.pruneCandidates(scored)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Work on a copy to avoid modifying original data between iterations
		prunedCopy := make([]*entities.Candidate, len(pruned))
		copy(prunedCopy, pruned)
		uc.selectGreedy(ctx, prunedCopy, 4000, nil)
	}
}

// Helper function - generate valid UUIDs from simple IDs
func createTestCandidateWithData(id string, createdAt time.Time, factCount, tokenCount int) *entities.Candidate {
	uid := uuid.New()
	// Create a non-zero embedding for valid cosine similarity
	embedding := make([]float32, 384)
	embedding[0] = 1.0 // Non-zero vector for valid similarity calculation
	return &entities.Candidate{
		Memory: &entities.Fingerprint{
			ID:            uid,
			FactCount:     factCount,
			TokenEstimate: tokenCount / 2,
			Type:          valueobjects.TypeSessionNote,
		},
		Verbatim: &entities.Verbatim{
			ID:         uid,
			TokenCount: tokenCount,
			CreatedAt:  createdAt,
		},
		Embedding: embedding,
	}
}

func createTestCandidateWithType(id string, createdAt time.Time, factCount, tokenCount int, memType valueobjects.MemoryType) *entities.Candidate {
	c := createTestCandidateWithData(id, createdAt, factCount, tokenCount)
	c.Memory.Type = memType
	return c
}

func createTestCandidateWithRelevance(id string, createdAt time.Time, relevance float64) *entities.Candidate {
	c := createTestCandidateWithData(id, createdAt, 10, 100)
	c.Relevance = relevance
	return c
}

func createTestCandidateWithScore(id string, createdAt time.Time, score float64, tokens int) *entities.Candidate {
	uid := uuid.New()
	embedding := make([]float32, 384)
	embedding[0] = float32(score)
	return &entities.Candidate{
		Memory: &entities.Fingerprint{
			ID:            uid,
			FactCount:     int(score * 100),
			TokenEstimate: tokens / 2,
			Type:          valueobjects.TypeSessionNote,
		},
		Verbatim: &entities.Verbatim{
			ID:         uid,
			TokenCount: tokens,
			CreatedAt:  createdAt,
		},
		Embedding: embedding,
		Score:     score,
	}
}

func createTestCandidateWithScoreAndTokens(id string, createdAt time.Time, score float64, tokens int) *entities.Candidate {
	return createTestCandidateWithScore(id, createdAt, score, tokens)
}

func createQueryVector(val float32) []float32 {
	vec := make([]float32, 384)
	vec[0] = val
	return vec
}

func strPtr(s string) *string {
	return &s
}

// Ensure interfaces are implemented
var _ ports.Embedder = (*mockRecallEmbedder)(nil)
var _ ports.OverlapCache = (*mockRecallOverlapCache)(nil)
var _ ports.CausalGraph = (*mockRecallCausalGraph)(nil)
var _ ports.VectorStore = (*mockRecallVectorStore)(nil)
var _ ports.FingerprintRenderer = (*mockRecallRenderer)(nil)
var _ ports.MetricsCollector = (*mockRecallMetricsCollector)(nil)

// ----------------------------------------------------------------------------
// adaptiveThresholdMeanStddev
// ----------------------------------------------------------------------------

func TestAdaptiveThresholdMeanStddev_FewScores(t *testing.T) {
	// < 3 scores → fixed fallback of 0.3
	if got := adaptiveThresholdMeanStddev([]float64{0.8, 0.9}); got != 0.3 {
		t.Errorf("expected 0.3, got %f", got)
	}
	if got := adaptiveThresholdMeanStddev(nil); got != 0.3 {
		t.Errorf("expected 0.3 for nil, got %f", got)
	}
}

func TestAdaptiveThresholdMeanStddev_MeanMinusStddev(t *testing.T) {
	// scores [0.6, 0.7, 0.8] → mean=0.7, variance=((0.1)²+(0)²+(0.1)²)/3≈0.00667, stddev≈0.0816
	// threshold ≈ 0.7 - 0.0816 ≈ 0.618
	scores := []float64{0.6, 0.7, 0.8}
	got := adaptiveThresholdMeanStddev(scores)
	if got < 0.55 || got > 0.7 {
		t.Errorf("expected threshold in [0.55, 0.70], got %f", got)
	}
}

func TestAdaptiveThresholdMeanStddev_IdenticalScores(t *testing.T) {
	// All same → stddev=0 → threshold = mean
	scores := []float64{0.5, 0.5, 0.5}
	got := adaptiveThresholdMeanStddev(scores)
	if math.Abs(got-0.5) > 1e-9 {
		t.Errorf("expected 0.5 for identical scores, got %f", got)
	}
}

// ----------------------------------------------------------------------------
// adaptiveThresholdElbow
// ----------------------------------------------------------------------------

func TestAdaptiveThresholdElbow_FewScores(t *testing.T) {
	// < 3 scores → fixed 0.3
	if got := adaptiveThresholdElbow([]float64{0.9}); got != 0.3 {
		t.Errorf("expected 0.3, got %f", got)
	}
	if got := adaptiveThresholdElbow(nil); got != 0.3 {
		t.Errorf("expected 0.3 for nil, got %f", got)
	}
}

func TestAdaptiveThresholdElbow_ClearElbow(t *testing.T) {
	// Big drop between 0.9 and 0.3 → elbow should cut at or after the big gap.
	// Sorted desc: [0.9, 0.89, 0.88, 0.3, 0.29]
	// Derivatives: [0.01, 0.01, 0.58, 0.01]
	// mean≈0.1525, std≈0.2627, cutoff≈0.415
	// first d > cutoff is index 2 (d=0.58) → return sorted[3] = 0.3
	scores := []float64{0.9, 0.89, 0.88, 0.3, 0.29}
	got := adaptiveThresholdElbow(scores)
	if got > 0.35 {
		t.Errorf("expected elbow threshold ≤ 0.35, got %f", got)
	}
}

func TestAdaptiveThresholdElbow_NoElbow(t *testing.T) {
	// Uniform gap → no derivative exceeds cutoff → returns last element
	scores := []float64{0.9, 0.8, 0.7}
	got := adaptiveThresholdElbow(scores)
	if got < 0.0 || got > 1.0 {
		t.Errorf("expected value in [0, 1], got %f", got)
	}
}

// ----------------------------------------------------------------------------
// adaptiveThreshold dispatch (iqr / elbow / default)
// ----------------------------------------------------------------------------

func TestAdaptiveThresholdDispatch_IQR(t *testing.T) {
	config := DefaultRecallMemoryConfig()
	config.ThresholdMethod = "iqr"
	uc := NewRecallMemory(
		&mockRecallVectorStore{}, &mockRecallOverlapCache{}, &mockRecallCausalGraph{},
		&mockRecallEmbedder{}, &mockRecallRenderer{}, config, &mockRecallMetricsCollector{}, nil,
	)
	scores := []float64{0.8, 0.7, 0.6, 0.5, 0.4}
	got := uc.adaptiveThreshold(scores)
	if got < uc.thresholdFloor || got > uc.thresholdCeiling {
		t.Errorf("iqr threshold %f outside [%f, %f]", got, uc.thresholdFloor, uc.thresholdCeiling)
	}
}

func TestAdaptiveThresholdDispatch_Elbow(t *testing.T) {
	config := DefaultRecallMemoryConfig()
	config.ThresholdMethod = "elbow"
	uc := NewRecallMemory(
		&mockRecallVectorStore{}, &mockRecallOverlapCache{}, &mockRecallCausalGraph{},
		&mockRecallEmbedder{}, &mockRecallRenderer{}, config, &mockRecallMetricsCollector{}, nil,
	)
	scores := []float64{0.9, 0.89, 0.88, 0.3, 0.29}
	got := uc.adaptiveThreshold(scores)
	if got < uc.thresholdFloor || got > uc.thresholdCeiling {
		t.Errorf("elbow threshold %f outside [%f, %f]", got, uc.thresholdFloor, uc.thresholdCeiling)
	}
}

func TestAdaptiveThresholdDispatch_FloorCeiling(t *testing.T) {
	config := DefaultRecallMemoryConfig()
	config.ThresholdFloor = 0.5
	config.ThresholdCeiling = 0.6
	uc := NewRecallMemory(
		&mockRecallVectorStore{}, &mockRecallOverlapCache{}, &mockRecallCausalGraph{},
		&mockRecallEmbedder{}, &mockRecallRenderer{}, config, &mockRecallMetricsCollector{}, nil,
	)
	// Mean-stddev of identical scores = 0.0 → clamped to floor 0.5
	got := uc.adaptiveThreshold([]float64{0.1, 0.1, 0.1})
	if got != 0.5 {
		t.Errorf("expected threshold clamped to floor 0.5, got %f", got)
	}
	// Very high scores → raw > ceiling → clamped to 0.6
	got = uc.adaptiveThreshold([]float64{0.99, 0.98, 0.97})
	if got != 0.6 {
		t.Errorf("expected threshold clamped to ceiling 0.6, got %f", got)
	}
}

// ----------------------------------------------------------------------------
// applyReranker
// ----------------------------------------------------------------------------

// mockRecallReranker implements ports.Reranker for applyReranker tests.
type mockRecallReranker struct {
	rerankFunc func(ctx context.Context, query string, candidates []string) ([]float64, error)
}

func (m *mockRecallReranker) Rerank(ctx context.Context, query string, candidates []string) ([]float64, error) {
	if m.rerankFunc != nil {
		return m.rerankFunc(ctx, query, candidates)
	}
	scores := make([]float64, len(candidates))
	for i := range scores {
		scores[i] = 0.5
	}
	return scores, nil
}

var _ ports.Reranker = (*mockRecallReranker)(nil)

func TestApplyReranker_BlendsScores(t *testing.T) {
	config := DefaultRecallMemoryConfig()
	config.RerankerEnabled = true
	config.RerankerTopK = 10
	reranker := &mockRecallReranker{
		rerankFunc: func(_ context.Context, _ string, candidates []string) ([]float64, error) {
			// Return 1.0 for first candidate, 0.0 for rest
			scores := make([]float64, len(candidates))
			scores[0] = 1.0
			return scores, nil
		},
	}
	config.Reranker = reranker

	uc := NewRecallMemory(
		&mockRecallVectorStore{}, &mockRecallOverlapCache{}, &mockRecallCausalGraph{},
		&mockRecallEmbedder{}, &mockRecallRenderer{}, config, &mockRecallMetricsCollector{}, nil,
	)
	uc.rerankerTopK = 10

	now := time.Now()
	c0 := createTestCandidateWithRelevance("c0", now, 0.8)
	c1 := createTestCandidateWithRelevance("c1", now, 0.8)

	out := uc.applyReranker(context.Background(), "query", []*entities.Candidate{c0, c1})

	// c0: blended = 0.7*0.8 + 0.3*1.0 = 0.86
	// c1: blended = 0.7*0.8 + 0.3*0.0 = 0.56
	if len(out) != 2 {
		t.Fatalf("expected 2 results, got %d", len(out))
	}
	// Re-sorted by blended relevance: c0 should be first
	if math.Abs(out[0].Relevance-0.86) > 0.01 {
		t.Errorf("c0 blended relevance: want ≈0.86, got %f", out[0].Relevance)
	}
	if math.Abs(out[1].Relevance-0.56) > 0.01 {
		t.Errorf("c1 blended relevance: want ≈0.56, got %f", out[1].Relevance)
	}
}

func TestApplyReranker_FallbackOnError(t *testing.T) {
	config := DefaultRecallMemoryConfig()
	config.RerankerTopK = 10
	config.Reranker = &mockRecallReranker{
		rerankFunc: func(_ context.Context, _ string, _ []string) ([]float64, error) {
			return nil, errors.New("reranker unavailable")
		},
	}

	uc := NewRecallMemory(
		&mockRecallVectorStore{}, &mockRecallOverlapCache{}, &mockRecallCausalGraph{},
		&mockRecallEmbedder{}, &mockRecallRenderer{}, config, &mockRecallMetricsCollector{}, nil,
	)
	uc.rerankerTopK = 10

	now := time.Now()
	orig := []*entities.Candidate{
		createTestCandidateWithRelevance("c0", now, 0.9),
		createTestCandidateWithRelevance("c1", now, 0.7),
	}
	// Copy original relevances
	wantRelev := []float64{0.9, 0.7}

	out := uc.applyReranker(context.Background(), "query", orig)
	if len(out) != 2 {
		t.Fatalf("expected 2 results, got %d", len(out))
	}
	// Scores should be unchanged (returned as-is on error)
	for i, c := range out {
		if math.Abs(c.Relevance-wantRelev[i]) > 1e-9 {
			t.Errorf("candidate %d: relevance should be unchanged, want %f got %f", i, wantRelev[i], c.Relevance)
		}
	}
}

func TestApplyReranker_TopKLimitsInput(t *testing.T) {
	config := DefaultRecallMemoryConfig()
	config.Reranker = &mockRecallReranker{}

	uc := NewRecallMemory(
		&mockRecallVectorStore{}, &mockRecallOverlapCache{}, &mockRecallCausalGraph{},
		&mockRecallEmbedder{}, &mockRecallRenderer{}, config, &mockRecallMetricsCollector{}, nil,
	)
	uc.rerankerTopK = 2 // Only top 2 sent to reranker

	var receivedCount int
	uc.reranker = &mockRecallReranker{
		rerankFunc: func(_ context.Context, _ string, candidates []string) ([]float64, error) {
			receivedCount = len(candidates)
			return make([]float64, len(candidates)), nil
		},
	}

	now := time.Now()
	candidates := []*entities.Candidate{
		createTestCandidateWithRelevance("c0", now, 0.9),
		createTestCandidateWithRelevance("c1", now, 0.8),
		createTestCandidateWithRelevance("c2", now, 0.7),
		createTestCandidateWithRelevance("c3", now, 0.6),
	}

	uc.applyReranker(context.Background(), "query", candidates)

	if receivedCount != 2 {
		t.Errorf("expected reranker to receive 2 candidates (topK=2), got %d", receivedCount)
	}
}
