package budget

import (
	"container/heap"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/benoitpetit/mira/extract"
	"github.com/benoitpetit/mira/types"
)

// Mock embedder pour l'intégration
type mockEmbedderIntegration struct {
	vectors map[string][]float32
}

func newMockEmbedderIntegration() *mockEmbedderIntegration {
	return &mockEmbedderIntegration{
		vectors: map[string][]float32{
			"query1": {1.0, 0.0, 0.0, 0.0},
			"query2": {0.0, 1.0, 0.0, 0.0},
			"PostgreSQL database": {0.9, 0.1, 0.0, 0.0},
		},
	}
}

func (m *mockEmbedderIntegration) Encode(text string) ([]float32, error) {
	if vec, ok := m.vectors[text]; ok {
		return vec, nil
	}
	return []float32{0.5, 0.5, 0.5, 0.5}, nil
}

// Mock vector store pour l'intégration
type mockVectorStoreIntegration struct {
	candidates []*types.Candidate
}

func (m *mockVectorStoreIntegration) Search(vector []float32, limit int, wing, room *string) ([]*types.Candidate, error) {
	// Filter par wing/room si spécifié
	var filtered []*types.Candidate
	for _, c := range m.candidates {
		if wing != nil && c.Verbatim.Wing != *wing {
			continue
		}
		if room != nil {
			if c.Verbatim.Room == nil || *c.Verbatim.Room != *room {
				continue
			}
		}
		filtered = append(filtered, c)
	}
	if len(filtered) > limit {
		return filtered[:limit], nil
	}
	return filtered, nil
}

// Mock causal graph pour l'intégration
type mockCausalGraphIntegration struct {
	edges map[string]bool
}

func newMockCausalGraphIntegration() *mockCausalGraphIntegration {
	return &mockCausalGraphIntegration{
		edges: make(map[string]bool),
	}
}

func (m *mockCausalGraphIntegration) HasEdge(fromID, toID uuid.UUID) bool {
	key := fromID.String() + "->" + toID.String()
	return m.edges[key]
}

func (m *mockCausalGraphIntegration) GetParents(nodeID uuid.UUID, relations ...types.RelationType) ([]*types.CausalNode, error) {
	return nil, nil
}

func (m *mockCausalGraphIntegration) GetChildren(nodeID uuid.UUID, relations ...types.RelationType) ([]*types.CausalNode, error) {
	return nil, nil
}

// Helper pour créer des candidats de test
func createIntegrationCandidate(id uuid.UUID, content string, wing, room string, tokenCount int, factCount int, memType types.MemoryType, embedding []float32) *types.Candidate {
	now := time.Now()
	roomPtr := &room
	if room == "" {
		roomPtr = nil
	}
	return &types.Candidate{
		Verbatim: &types.Verbatim{
			ID:         uuid.New(),
			Content:    content,
			Wing:       wing,
			Room:       roomPtr,
			TokenCount: tokenCount,
			CreatedAt:  now,
		},
		Memory: &types.Fingerprint{
			ID:            id,
			Type:          memType,
			ExtractedAt:   now,
			FactCount:     factCount,
			TokenEstimate: tokenCount / 4,
			Data: types.FingerprintData{
				Date:        now.Format("2006-01-02T15:04:05Z"),
				Subject:     []string{"test"},
				Decision:    "decide",
				VerbatimRef: id.String(),
			},
		},
		Embedding: embedding,
	}
}

func TestAllocate(t *testing.T) {
	emb := newMockEmbedderIntegration()
	ext, err := extract.NewExtractor("test-model", emb)
	if err != nil {
		t.Fatalf("NewExtractor() error = %v", err)
	}
	vs := &mockVectorStoreIntegration{}
	cg := newMockCausalGraphIntegration()
	alloc := NewAllocator(vs, nil, cg, ext)

	// Créer des candidats avec des embeddings similaires à query1
	c1 := createIntegrationCandidate(
		uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		"We decided to use PostgreSQL for the database",
		"backend", "db",
		100, 5, types.TypeDecision,
		[]float32{0.9, 0.1, 0.0, 0.0}, // Très similaire à query1 {1.0, 0.0, 0.0, 0.0}
	)
	c2 := createIntegrationCandidate(
		uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		"Redis will be used for caching",
		"backend", "cache",
		80, 3, types.TypeDecision,
		[]float32{0.8, 0.2, 0.0, 0.0}, // Aussi similaire à query1
	)
	vs.candidates = []*types.Candidate{c1, c2}

	tests := []struct {
		name      string
		query     string
		budget    int
		wing      *string
		room      *string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "basic allocation",
			query:     "query1",
			budget:    1000,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "small budget",
			query:     "query1",
			budget:    50,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:      "default budget",
			query:     "query1",
			budget:    0,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "filter by wing",
			query:     "query1",
			budget:    1000,
			wing:      strPtr("backend"),
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "filter by non-existent wing",
			query:     "query1",
			budget:    1000,
			wing:      strPtr("frontend"),
			wantCount: 0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := alloc.Allocate(tt.query, tt.budget, tt.wing, tt.room)
			if (err != nil) != tt.wantErr {
				t.Errorf("Allocate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(result) != tt.wantCount {
				t.Errorf("Allocate() got %d memories, want %d", len(result), tt.wantCount)
			}
		})
	}
}

func TestAllocateModes(t *testing.T) {
	emb := newMockEmbedderIntegration()
	ext, _ := extract.NewExtractor("test-model", emb)
	vs := &mockVectorStoreIntegration{}
	cg := newMockCausalGraphIntegration()
	alloc := NewAllocator(vs, nil, cg, ext)

	// Créer un candidat avec beaucoup de tokens
	c1 := createIntegrationCandidate(
		uuid.New(),
		"Large content with many tokens for testing mode selection based on budget constraints",
		"backend", "db",
		500, 10, types.TypeDecision,
		[]float32{0.9, 0.1, 0.0, 0.0},
	)
	vs.candidates = []*types.Candidate{c1}

	tests := []struct {
		name     string
		budget   int
		wantMode types.RenderMode
	}{
		{
			name:     "large budget gets verbatim",
			budget:   5000,
			wantMode: types.ModeVerbatim,
		},
		{
			name:     "medium budget gets fingerprint",
			budget:   500,
			wantMode: types.ModeFingerprint,
		},
		{
			name:     "small budget gets header",
			budget:   50,
			wantMode: types.ModeHeader,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := alloc.Allocate("query1", tt.budget, nil, nil)
			if err != nil {
				t.Fatalf("Allocate() error = %v", err)
			}
			if len(result) == 0 {
				t.Fatal("Expected at least one result")
			}
			if result[0].Mode != tt.wantMode {
				t.Errorf("got mode %v, want %v", result[0].Mode, tt.wantMode)
			}
		})
	}
}

func TestEmbeddingCacheIntegration(t *testing.T) {
	cache := NewEmbeddingCache(3)

	vec1 := []float32{1.0, 0.0, 0.0}
	vec2 := []float32{0.0, 1.0, 0.0}
	vec3 := []float32{0.0, 0.0, 1.0}
	vec4 := []float32{0.5, 0.5, 0.5}

	// Test Get sur cache vide
	if _, ok := cache.Get("key1"); ok {
		t.Error("Expected cache miss on empty cache")
	}

	// Test Set et Get
	cache.Set("key1", vec1)
	if got, ok := cache.Get("key1"); !ok {
		t.Error("Expected cache hit after Set")
	} else if len(got) != len(vec1) {
		t.Error("Vector mismatch")
	}

	// Test LRU eviction
	cache.Set("key2", vec2)
	cache.Set("key3", vec3)
	cache.Set("key4", vec4) // Devrait évincer key1 (le plus ancien)

	if _, ok := cache.Get("key1"); ok {
		t.Error("Expected key1 to be evicted")
	}
	if _, ok := cache.Get("key2"); !ok {
		t.Error("Expected key2 to still be in cache")
	}
	if _, ok := cache.Get("key3"); !ok {
		t.Error("Expected key3 to still be in cache")
	}
	if _, ok := cache.Get("key4"); !ok {
		t.Error("Expected key4 to be in cache")
	}

	// Test mise à jour (Set sur une clé existante)
	cache.Set("key2", vec1) // key2 devient le plus récent
	cache.Set("key5", vec4) // Devrait évincer key3 (pas key2 qui vient d'être mis à jour)

	if _, ok := cache.Get("key2"); !ok {
		t.Error("Expected key2 to still be in cache after being updated")
	}
	if _, ok := cache.Get("key3"); ok {
		t.Error("Expected key3 to be evicted")
	}
}

func TestRenderHeader(t *testing.T) {
	alloc := &Allocator{}

	c := &types.Candidate{
		Memory: &types.Fingerprint{
			Type: types.TypeDecision,
			Data: types.FingerprintData{
				VerbatimRef: "abc-123",
			},
		},
		Verbatim: &types.Verbatim{
			CreatedAt: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			Wing:      "backend",
		},
	}

	header := alloc.renderHeader(c)
	expected := "[decision|2024-01-15|backend] → abc-123 (load with mira_load)"
	if header != expected {
		t.Errorf("renderHeader() = %q, want %q", header, expected)
	}
}

func TestRenderFingerprint(t *testing.T) {
	alloc := &Allocator{}

	tests := []struct {
		name string
		c    *types.Candidate
		want string
	}{
		{
			name: "basic fingerprint",
			c: &types.Candidate{
				Memory: &types.Fingerprint{
					Data: types.FingerprintData{
						Date:        "2024-01-15T10:00:00Z",
						Subject:     []string{"database", "postgres"},
						Decision:    "Use PostgreSQL",
						VerbatimRef: "abc-123",
					},
				},
			},
			want: "[database,postgres|2024-01-15] | Use PostgreSQL | → abc-123",
		},
		{
			name: "fingerprint with rejected",
			c: &types.Candidate{
				Memory: &types.Fingerprint{
					Data: types.FingerprintData{
						Date:        "2024-01-15T10:00:00Z",
						Subject:     []string{"cache"},
						Decision:    "Use Redis",
						Rejected:    []string{"Memcached", "Varnish"},
						VerbatimRef: "def-456",
					},
				},
			},
			want: "[cache|2024-01-15] | Use Redis | (rejected: Memcached,Varnish) | → def-456",
		},
		{
			name: "fingerprint with reason",
			c: &types.Candidate{
				Memory: &types.Fingerprint{
					Data: types.FingerprintData{
						Date:        "2024-01-15T10:00:00Z",
						Subject:     []string{"auth"},
						Decision:    "Use OAuth2",
						Reason:      []string{"better security", "industry standard"},
						VerbatimRef: "ghi-789",
					},
				},
			},
			want: "[auth|2024-01-15] | Use OAuth2 | reason: better security; industry standard | → ghi-789",
		},
		{
			name: "fingerprint full",
			c: &types.Candidate{
				Memory: &types.Fingerprint{
					Data: types.FingerprintData{
						Date:        "2024-01-15T10:00:00Z",
						Subject:     []string{"api"},
						Decision:    "Use GraphQL",
						Rejected:    []string{"REST"},
						Reason:      []string{"flexibility"},
						Assignee:    "john",
						Deadline:    "2024-02-01",
						VerbatimRef: "jkl-012",
					},
				},
			},
			want: "[api|2024-01-15] | Use GraphQL | (rejected: REST) | reason: flexibility | @john | due:2024-02-01 | → jkl-012",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := alloc.renderFingerprint(tt.c)
			if got != tt.want {
				t.Errorf("renderFingerprint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRender(t *testing.T) {
	alloc := &Allocator{}

	c := &types.Candidate{
		Verbatim: &types.Verbatim{
			Content: "Full verbatim content here",
		},
		Memory: &types.Fingerprint{
			Type: types.TypeDecision,
			Data: types.FingerprintData{
				Date:        "2024-01-15T10:00:00Z",
				Subject:     []string{"test"},
				Decision:    "Decision text",
				VerbatimRef: "ref-123",
			},
		},
	}

	tests := []struct {
		mode types.RenderMode
		want string
	}{
		{types.ModeVerbatim, "Full verbatim content here"},
		{types.ModeHeader, "[decision|0001-01-01|] → ref-123 (load with mira_load)"},
		{types.ModeFingerprint, "[test|2024-01-15] | Decision text | → ref-123"},
		{types.RenderMode(999), ""}, // Mode invalide
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("mode_%d", tt.mode), func(t *testing.T) {
			got := alloc.render(c, tt.mode)
			if got != tt.want {
				t.Errorf("render() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCalculateTokenCost(t *testing.T) {
	alloc := &Allocator{}

	c := &types.Candidate{
		Verbatim: &types.Verbatim{
			TokenCount: 100,
		},
		Memory: &types.Fingerprint{
			TokenEstimate: 25,
		},
	}

	tests := []struct {
		mode types.RenderMode
		want int
	}{
		{types.ModeHeader, 5},
		{types.ModeFingerprint, 25},
		{types.ModeVerbatim, 100},
		{types.RenderMode(999), 0},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("mode_%d", tt.mode), func(t *testing.T) {
			got := alloc.calculateTokenCost(c, tt.mode)
			if got != tt.want {
				t.Errorf("calculateTokenCost() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDetermineRenderMode(t *testing.T) {
	alloc := &Allocator{}

	tests := []struct {
		budget int
		want   types.RenderMode
	}{
		{50, types.ModeHeader},
		{99, types.ModeHeader},
		{100, types.ModeFingerprint},
		{500, types.ModeFingerprint},
		{999, types.ModeFingerprint},
		{1000, types.ModeVerbatim},
		{5000, types.ModeVerbatim},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("budget_%d", tt.budget), func(t *testing.T) {
			c := &types.Candidate{}
			got := alloc.determineRenderMode(c, tt.budget)
			if got != tt.want {
				t.Errorf("determineRenderMode(budget=%d) = %v, want %v", tt.budget, got, tt.want)
			}
		})
	}
}

func TestMinFunc(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{5, 5, 5},
		{-1, 1, -1},
		{0, 100, 0},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("min_%d_%d", tt.a, tt.b), func(t *testing.T) {
			got := min(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestPriorityQueueIntegration(t *testing.T) {
	pq := make(PriorityQueue, 0)

	c1 := &types.Candidate{Score: 0.9}
	c2 := &types.Candidate{Score: 0.5}
	c3 := &types.Candidate{Score: 0.7}

	// Test Push
	heap.Push(&pq, &Item{candidate: c1, priority: c1.Score, index: 0})
	heap.Push(&pq, &Item{candidate: c2, priority: c2.Score, index: 1})
	heap.Push(&pq, &Item{candidate: c3, priority: c3.Score, index: 2})

	if pq.Len() != 3 {
		t.Errorf("Expected queue length 3, got %d", pq.Len())
	}

	// Test Pop (max-heap: highest score first)
	item := heap.Pop(&pq).(*Item)
	if item.priority != 0.9 {
		t.Errorf("Expected priority 0.9, got %f", item.priority)
	}

	item = heap.Pop(&pq).(*Item)
	if item.priority != 0.7 {
		t.Errorf("Expected priority 0.7, got %f", item.priority)
	}

	item = heap.Pop(&pq).(*Item)
	if item.priority != 0.5 {
		t.Errorf("Expected priority 0.5, got %f", item.priority)
	}

	if pq.Len() != 0 {
		t.Errorf("Expected empty queue, got %d", pq.Len())
	}
}

func TestNewAllocator(t *testing.T) {
	emb := newMockEmbedderIntegration()
	ext, _ := extract.NewExtractor("test-model", emb)
	vs := &mockVectorStoreIntegration{}
	cg := newMockCausalGraphIntegration()

	alloc := NewAllocator(vs, nil, cg, ext)

	if alloc == nil {
		t.Fatal("NewAllocator() returned nil")
	}
	if alloc.vectorDB != vs {
		t.Error("vectorDB not set correctly")
	}
	if alloc.causalGraph != cg {
		t.Error("causalGraph not set correctly")
	}
	if alloc.embedder != ext {
		t.Error("embedder not set correctly")
	}
	if alloc.cache == nil {
		t.Error("cache not initialized")
	}
}

func TestAllocateEmptyCandidates(t *testing.T) {
	emb := newMockEmbedderIntegration()
	ext, _ := extract.NewExtractor("test-model", emb)
	vs := &mockVectorStoreIntegration{candidates: []*types.Candidate{}}
	cg := newMockCausalGraphIntegration()
	alloc := NewAllocator(vs, nil, cg, ext)

	result, err := alloc.Allocate("query1", 1000, nil, nil)
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected 0 results for empty candidates, got %d", len(result))
	}
}

func TestAllocateEarlyPruning(t *testing.T) {
	emb := newMockEmbedderIntegration()
	ext, _ := extract.NewExtractor("test-model", emb)
	vs := &mockVectorStoreIntegration{}
	cg := newMockCausalGraphIntegration()
	alloc := NewAllocator(vs, nil, cg, ext)

	// Créer des candidats avec différents niveaux de pertinence
	now := time.Now()
	room := "test-room"

	// Candidat avec embedding très différent de la query (faible pertinence)
	lowRel := &types.Candidate{
		Verbatim: &types.Verbatim{
			ID:          uuid.New(),
			Content:     "Unrelated content",
			TokenCount:  50,
			CreatedAt:   now,
			Wing:        "test",
			Room:        &room,
		},
		Memory: &types.Fingerprint{
			ID:            uuid.New(),
			Type:          types.TypeDecision,
			ExtractedAt:   now,
			FactCount:     3,
			TokenEstimate: 15,
			Data: types.FingerprintData{
				Date:        now.Format("2006-01-02T15:04:05Z"),
				Subject:     []string{"unrelated"},
				VerbatimRef: "low-ref",
			},
		},
		Embedding: []float32{-0.9, -0.1, 0.0, 0.0}, // Très différent de query1 {1.0, 0.0, 0.0, 0.0}
	}

	// Candidat avec embedding similaire (haute pertinence)
	highRel := &types.Candidate{
		Verbatim: &types.Verbatim{
			ID:          uuid.New(),
			Content:     "Related content about databases",
			TokenCount:  50,
			CreatedAt:   now,
			Wing:        "test",
			Room:        &room,
		},
		Memory: &types.Fingerprint{
			ID:            uuid.New(),
			Type:          types.TypeDecision,
			ExtractedAt:   now,
			FactCount:     3,
			TokenEstimate: 15,
			Data: types.FingerprintData{
				Date:        now.Format("2006-01-02T15:04:05Z"),
				Subject:     []string{"database"},
				VerbatimRef: "high-ref",
			},
		},
		Embedding: []float32{0.95, 0.05, 0.0, 0.0}, // Très similaire à query1
	}

	vs.candidates = []*types.Candidate{lowRel, highRel}

	result, err := alloc.Allocate("query1", 1000, nil, nil)
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}

	// Seul le candidat à haute pertinence devrait être sélectionné
	// (après élagage précoce à 0.6)
	foundHigh := false
	for _, r := range result {
		if r.Candidate.Memory.Data.VerbatimRef == "high-ref" {
			foundHigh = true
		}
	}

	if !foundHigh {
		t.Error("Expected high relevance candidate to be selected")
	}
}

func strPtr(s string) *string {
	return &s
}
