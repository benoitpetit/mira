package budget

import (
	"container/heap"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"mira/extract"
	"mira/types"
)

const (
	DefaultBudget           = 4000
	MaxCandidates           = 100
	EarlyPruningThreshold   = 0.6
	SessionWindow           = 2 * time.Hour
	SessionBoostBeta        = 0.2
	CausalPenaltyAlpha      = 0.15
	DensitySigmoidK         = 2.0
	DensitySigmoidMu        = 0.3
)

// VectorStore interface for vector search
type VectorStore interface {
	Search(vector []float32, limit int, wing, room *string) ([]*types.Candidate, error)
}

// OverlapCache interface for overlap cache
type OverlapCache interface {
	Get(idA, idB uuid.UUID) (float64, bool)
	Set(idA, idB uuid.UUID, similarity float64)
}

// CausalGraph interface for causal graph
type CausalGraph interface {
	HasEdge(fromID, toID uuid.UUID) bool
	GetParents(nodeID uuid.UUID, relations ...types.RelationType) ([]*types.CausalNode, error)
	GetChildren(nodeID uuid.UUID, relations ...types.RelationType) ([]*types.CausalNode, error)
}

// EmbeddingCache LRU to avoid re-computations
type EmbeddingCache struct {
	cache   map[string][]float32
	order   []string
	maxSize int
}

// NewEmbeddingCache creates a new embedding cache
func NewEmbeddingCache(maxSize int) *EmbeddingCache {
	return &EmbeddingCache{
		cache:   make(map[string][]float32),
		order:   make([]string, 0, maxSize),
		maxSize: maxSize,
	}
}

// Get retrieves a value from cache
func (c *EmbeddingCache) Get(key string) ([]float32, bool) {
	vec, ok := c.cache[key]
	return vec, ok
}

// Set stores a value in cache
func (c *EmbeddingCache) Set(key string, vec []float32) {
	if _, exists := c.cache[key]; exists {
		// Move to end
		for i, k := range c.order {
			if k == key {
				c.order = append(c.order[:i], c.order[i+1:]...)
				break
			}
		}
	} else if len(c.cache) >= c.maxSize {
		// LRU eviction
		oldest := c.order[0]
		delete(c.cache, oldest)
		c.order = c.order[1:]
	}

	c.cache[key] = vec
	c.order = append(c.order, key)
}

// Allocator manages context budget allocation
type Allocator struct {
	vectorDB     VectorStore
	overlapCache OverlapCache
	causalGraph  CausalGraph
	embedder     *extract.Extractor
	cache        *EmbeddingCache
}

// NewAllocator creates a new allocator
func NewAllocator(vectorDB VectorStore, overlapCache OverlapCache, causal CausalGraph, embedder *extract.Extractor) *Allocator {
	return &Allocator{
		vectorDB:     vectorDB,
		overlapCache: overlapCache,
		causalGraph:  causal,
		embedder:     embedder,
		cache:        NewEmbeddingCache(1000),
	}
}

// Allocate with wing/room filtering and early pruning
func (a *Allocator) Allocate(query string, budget int, wing, room *string) ([]*types.SelectedMemory, error) {
	if budget <= 0 {
		budget = DefaultBudget
	}

	// 1. Embedding with cache
	queryVec, err := a.getQueryEmbedding(query)
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}

	// 2. Vector search (wide for early pruning)
	candidates, err := a.vectorDB.Search(queryVec, MaxCandidates, wing, room)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	// 3. Initial scoring with early pruning
	scored := a.scoreCandidates(candidates, queryVec)

	// Early pruning: keep only ρ > 0.6
	var pruned []*types.Candidate
	for _, c := range scored {
		if c.Relevance > EarlyPruningThreshold {
			pruned = append(pruned, c)
		}
	}

	if len(pruned) == 0 && len(scored) > 0 {
		// Fallback: keep top 5 even if < threshold
		sort.Slice(scored, func(i, j int) bool { return scored[i].Relevance > scored[j].Relevance })
		pruned = scored[:min(5, len(scored))]
	}

	// 4. Greedy selection with dynamic renormalization
	selected := a.selectGreedyDynamic(pruned, budget)

	return selected, nil
}

func (a *Allocator) getQueryEmbedding(query string) ([]float32, error) {
	// Check cache
	if vec, ok := a.cache.Get(query); ok {
		return vec, nil
	}

	vec, err := a.embedder.Encode(query)
	if err != nil {
		return nil, err
	}

	a.cache.Set(query, vec)
	return vec, nil
}

// scoreCandidates with sigmoid density
func (a *Allocator) scoreCandidates(candidates []*types.Candidate, queryVec []float32) []*types.Candidate {
	now := time.Now()

	for _, c := range candidates {
		// ρ: semantic relevance [0,1]
		c.Relevance = cosineSimilarity(c.Embedding, queryVec)
		c.Relevance = (1 + c.Relevance) / 2 // [-1,1] → [0,1]

		// δ_sig: density with sigmoid
		if c.Verbatim.TokenCount > 0 {
			rawDensity := float64(c.Memory.FactCount) / math.Sqrt(float64(c.Verbatim.TokenCount))
			// Sigmoid: 2/(1+e^(-2(x-0.3))) - 1
			c.Density = 2.0/(1.0+math.Exp(-DensitySigmoidK*(rawDensity-DensitySigmoidMu))) - 1.0
			if c.Density < 0 {
				c.Density = 0
			}
		}

		// η: recency
		ageDays := now.Sub(c.Verbatim.CreatedAt).Hours() / 24
		lambda := types.DecayRates[c.Memory.Type]
		c.Recency = math.Exp(-lambda * ageDays)

		// Initial score (without overlap/causal/session - computed dynamically)
		c.Score = c.Relevance * c.Density * c.Recency
	}

	return candidates
}

// selectGreedyDynamic with recalculation at each iteration
func (a *Allocator) selectGreedyDynamic(candidates []*types.Candidate, budget int) []*types.SelectedMemory {
	if len(candidates) == 0 {
		return nil
	}

	// Max-heap for efficient best candidate selection at each round
	pq := make(PriorityQueue, len(candidates))
	for i, c := range candidates {
		pq[i] = &Item{candidate: c, priority: c.Score, index: i}
	}
	heap.Init(&pq)

	var selected []*types.SelectedMemory
	tokensUsed := 0
	selectedEmbeddings := make([][]float32, 0)
	selectedIDs := make(map[uuid.UUID]bool)
	selectedTimes := make([]time.Time, 0)

	for pq.Len() > 0 && budget-tokensUsed >= 50 {
		// Get best candidate
		item := heap.Pop(&pq).(*Item)
		c := item.candidate

		if selectedIDs[c.Memory.ID] {
			continue
		}

		// Compute max overlap with already selected
		maxOverlap := 0.0
		for _, emb := range selectedEmbeddings {
			overlap := cosineSimilarity(c.Embedding, emb)
			if overlap > maxOverlap {
				maxOverlap = overlap
			}
		}
		c.MaxOverlap = maxOverlap

		// Compute causal penalty: how many links already exist with S?
		causalCount := 0
		for _, sel := range selected {
			if a.causalGraph.HasEdge(sel.Candidate.Memory.ID, c.Memory.ID) ||
				a.causalGraph.HasEdge(c.Memory.ID, sel.Candidate.Memory.ID) {
				causalCount++
			}
		}
		c.CausalPenalty = math.Exp(-CausalPenaltyAlpha * float64(causalCount))

		// Compute session boost: temporal proximity with S
		sessionBoost := 1.0
		for _, t := range selectedTimes {
			if math.Abs(c.Verbatim.CreatedAt.Sub(t).Seconds()) < SessionWindow.Seconds() {
				sessionBoost = 1.0 + SessionBoostBeta
				break
			}
		}
		c.SessionBoost = sessionBoost

		// Determine render mode based on REMAINING budget (not overlap)
		remainingBudget := budget - tokensUsed
		mode := a.determineRenderMode(c, remainingBudget)
		tokenCost := a.calculateTokenCost(c, mode)

		// Check budget
		if tokensUsed+tokenCost > budget {
			// Try downgrade
			if mode == types.ModeVerbatim {
				mode = types.ModeFingerprint
				tokenCost = c.Memory.TokenEstimate
				if tokensUsed+tokenCost > budget {
					mode = types.ModeHeader
					tokenCost = 5
				}
			} else if mode == types.ModeFingerprint {
				mode = types.ModeHeader
				tokenCost = 5
			}

			if tokensUsed+tokenCost > budget {
				continue // Skip
			}
		}

		// Render
		rendered := a.render(c, mode)

		sel := &types.SelectedMemory{
			Candidate:  c,
			Mode:       mode,
			TokenCost:  tokenCost,
			Rendered:   rendered,
			SelectedAt: time.Now(),
		}

		selected = append(selected, sel)
		selectedEmbeddings = append(selectedEmbeddings, c.Embedding)
		selectedIDs[c.Memory.ID] = true
		selectedTimes = append(selectedTimes, c.Verbatim.CreatedAt)
		tokensUsed += tokenCost
	}

	return selected
}

// determineRenderMode: based only on budget, not overlap
func (a *Allocator) determineRenderMode(c *types.Candidate, remainingBudget int) types.RenderMode {
	switch {
	case remainingBudget < 100:
		return types.ModeHeader
	case remainingBudget < 1000:
		return types.ModeFingerprint
	default:
		return types.ModeVerbatim
	}
}

func (a *Allocator) calculateTokenCost(c *types.Candidate, mode types.RenderMode) int {
	switch mode {
	case types.ModeHeader:
		return 5
	case types.ModeFingerprint:
		return c.Memory.TokenEstimate
	case types.ModeVerbatim:
		return c.Verbatim.TokenCount
	default:
		return 0
	}
}

func (a *Allocator) render(c *types.Candidate, mode types.RenderMode) string {
	switch mode {
	case types.ModeHeader:
		return a.renderHeader(c)
	case types.ModeFingerprint:
		return a.renderFingerprint(c)
	case types.ModeVerbatim:
		return c.Verbatim.Content
	default:
		return ""
	}
}

func (a *Allocator) renderHeader(c *types.Candidate) string {
	return fmt.Sprintf("[%s|%s|%s] → %s (load with mira_load)",
		c.Memory.Type,
		c.Verbatim.CreatedAt.Format("2006-01-02"),
		c.Verbatim.Wing,
		c.Memory.Data.VerbatimRef,
	)
}

func (a *Allocator) renderFingerprint(c *types.Candidate) string {
	d := c.Memory.Data
	parts := []string{fmt.Sprintf("[%s|%s]", strings.Join(d.Subject, ","), d.Date[:10])}

	if d.Decision != "" {
		parts = append(parts, d.Decision)
	}
	if len(d.Rejected) > 0 {
		parts = append(parts, fmt.Sprintf("(rejected: %s)", strings.Join(d.Rejected, ",")))
	}
	if len(d.Reason) > 0 {
		parts = append(parts, fmt.Sprintf("reason: %s", strings.Join(d.Reason, "; ")))
	}
	if d.Assignee != "" {
		parts = append(parts, fmt.Sprintf("@%s", d.Assignee))
	}
	if d.Deadline != "" {
		parts = append(parts, fmt.Sprintf("due:%s", d.Deadline))
	}

	parts = append(parts, fmt.Sprintf("→ %s", d.VerbatimRef))
	return strings.Join(parts, " | ")
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

// Priority Queue implementation
type Item struct {
	candidate *types.Candidate
	priority  float64
	index     int
}
type PriorityQueue []*Item

func (pq PriorityQueue) Len() int { return len(pq) }
func (pq PriorityQueue) Less(i, j int) bool { return pq[i].priority > pq[j].priority }
func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}
func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*Item)
	item.index = n
	*pq = append(*pq, item)
}
func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[0 : n-1]
	return item
}
