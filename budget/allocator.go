package budget

import (
	"container/heap"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/benoitpetit/mira/extract"
	"github.com/benoitpetit/mira/internal/util"
	"github.com/benoitpetit/mira/types"
	"github.com/google/uuid"
)

const (
	DefaultBudget         = 4000
	MaxCandidates         = 100
	EarlyPruningThreshold = 0.6
	SessionWindow         = 2 * time.Hour
	SessionBoostBeta      = 0.2
	CausalPenaltyAlpha    = 0.15
	DensitySigmoidK       = 2.0
	DensitySigmoidMu      = 0.3
)

// AllocatorOptions holds configurable parameters for the allocator.
// Zero values are replaced by the default constants above.
type AllocatorOptions struct {
	DefaultBudget         int
	MaxCandidates         int
	EarlyPruningThreshold float64
	SessionWindowSeconds  int
	SessionBoostBeta      float64
	CausalPenaltyAlpha    float64
	DensitySigmoidK       float64
	DensitySigmoidMu      float64
	EmbeddingCacheSize    int
}

func (o AllocatorOptions) withDefaults() AllocatorOptions {
	if o.DefaultBudget <= 0 {
		o.DefaultBudget = DefaultBudget
	}
	if o.MaxCandidates <= 0 {
		o.MaxCandidates = MaxCandidates
	}
	if o.EarlyPruningThreshold <= 0 {
		o.EarlyPruningThreshold = EarlyPruningThreshold
	}
	if o.SessionWindowSeconds <= 0 {
		o.SessionWindowSeconds = int(SessionWindow.Seconds())
	}
	if o.SessionBoostBeta <= 0 {
		o.SessionBoostBeta = SessionBoostBeta
	}
	if o.CausalPenaltyAlpha <= 0 {
		o.CausalPenaltyAlpha = CausalPenaltyAlpha
	}
	if o.DensitySigmoidK <= 0 {
		o.DensitySigmoidK = DensitySigmoidK
	}
	if o.DensitySigmoidMu <= 0 {
		o.DensitySigmoidMu = DensitySigmoidMu
	}
	if o.EmbeddingCacheSize <= 0 {
		o.EmbeddingCacheSize = 1000
	}
	return o
}

// VectorStore interface for vector search
type VectorStore interface {
	Search(vector []float32, limit int, wing, room *string) ([]*types.Candidate, error)
}

// CausalGraph interface for causal graph
type CausalGraph interface {
	HasEdge(fromID, toID uuid.UUID) bool
	GetParents(nodeID uuid.UUID, relations ...types.RelationType) ([]*types.CausalNode, error)
	GetChildren(nodeID uuid.UUID, relations ...types.RelationType) ([]*types.CausalNode, error)
}

// embeddingCache is an LRU cache to avoid re-computations (thread-safe)
type embeddingCache struct {
	mu      sync.RWMutex
	cache   map[string][]float32
	order   []string
	maxSize int
}

// newEmbeddingCache creates a new embedding cache
func newEmbeddingCache(maxSize int) *embeddingCache {
	return &embeddingCache{
		cache:   make(map[string][]float32),
		order:   make([]string, 0, maxSize),
		maxSize: maxSize,
	}
}

// get retrieves a value from cache (thread-safe)
func (c *embeddingCache) get(key string) ([]float32, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	vec, ok := c.cache[key]
	return vec, ok
}

// set stores a value in cache (thread-safe)
func (c *embeddingCache) set(key string, vec []float32) {
	c.mu.Lock()
	defer c.mu.Unlock()

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
	overlapCache types.OverlapCache
	causalGraph  CausalGraph
	embedder     *extract.Extractor
	cache        *embeddingCache
	opts         AllocatorOptions
}

// NewAllocatorWithOptions creates a new allocator with explicit options
func NewAllocatorWithOptions(vectorDB VectorStore, overlapCache types.OverlapCache, causal CausalGraph, embedder *extract.Extractor, opts AllocatorOptions) *Allocator {
	opts = opts.withDefaults()
	return &Allocator{
		vectorDB:     vectorDB,
		overlapCache: overlapCache,
		causalGraph:  causal,
		embedder:     embedder,
		cache:        newEmbeddingCache(opts.EmbeddingCacheSize),
		opts:         opts,
	}
}

// Allocate with wing/room filtering and early pruning
func (a *Allocator) Allocate(query string, budget int, wing, room *string) ([]*types.SelectedMemory, error) {
	if budget <= 0 {
		budget = a.opts.DefaultBudget
	}

	// 1. Embedding with cache
	queryVec, err := a.getQueryEmbedding(query)
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}

	// 2. Vector search (wide for early pruning)
	candidates, err := a.vectorDB.Search(queryVec, a.opts.MaxCandidates, wing, room)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	// 3. Initial scoring with early pruning
	scored := a.scoreCandidates(candidates, queryVec)

	// Early pruning: keep only ρ > threshold
	var pruned []*types.Candidate
	for _, c := range scored {
		if c.Relevance > a.opts.EarlyPruningThreshold {
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
	if vec, ok := a.cache.get(query); ok {
		return vec, nil
	}

	vec, err := a.embedder.Encode(query)
	if err != nil {
		return nil, err
	}

	a.cache.set(query, vec)
	return vec, nil
}

// scoreCandidates with sigmoid density
func (a *Allocator) scoreCandidates(candidates []*types.Candidate, queryVec []float32) []*types.Candidate {
	now := time.Now()

	for _, c := range candidates {
		// ρ: semantic relevance [0,1]
		c.Relevance = util.CosineSimilarity(c.Embedding, queryVec)
		c.Relevance = (1 + c.Relevance) / 2 // [-1,1] → [0,1]

		// δ_sig: density with sigmoid
		if c.Verbatim.TokenCount > 0 {
			rawDensity := float64(c.Memory.FactCount) / math.Sqrt(float64(c.Verbatim.TokenCount))
			// Sigmoid: 2/(1+e^(-k(x-mu))) - 1
			c.Density = 2.0/(1.0+math.Exp(-a.opts.DensitySigmoidK*(rawDensity-a.opts.DensitySigmoidMu))) - 1.0
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
	pq := make(priorityQueue, len(candidates))
	for i, c := range candidates {
		pq[i] = &item{candidate: c, priority: c.Score, index: i}
	}
	heap.Init(&pq)

	var selected []*types.SelectedMemory
	tokensUsed := 0
	selectedEmbeddings := make([][]float32, 0)
	selectedIDs := make(map[uuid.UUID]bool)
	selectedTimes := make([]time.Time, 0)

	for pq.Len() > 0 && budget-tokensUsed >= 50 {
		// Get best candidate
		item := heap.Pop(&pq).(*item)
		c := item.candidate

		if selectedIDs[c.Memory.ID] {
			continue
		}

		// Compute max overlap with already selected (using cache when available)
		maxOverlap := 0.0
		for i, sel := range selected {
			selID := sel.Candidate.Memory.ID
			var overlap float64
			if a.overlapCache != nil {
				if cached, ok := a.overlapCache.Get(c.Memory.ID, selID); ok {
					overlap = cached
				} else {
					overlap = util.CosineSimilarity(c.Embedding, selectedEmbeddings[i])
					a.overlapCache.Set(c.Memory.ID, selID, overlap)
				}
			} else {
				overlap = util.CosineSimilarity(c.Embedding, selectedEmbeddings[i])
			}
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
		c.CausalPenalty = math.Exp(-a.opts.CausalPenaltyAlpha * float64(causalCount))

		// Compute session boost: temporal proximity with S
		sessionWindowSec := float64(a.opts.SessionWindowSeconds)
		sessionBoost := 1.0
		for _, t := range selectedTimes {
			if math.Abs(c.Verbatim.CreatedAt.Sub(t).Seconds()) < sessionWindowSec {
				sessionBoost = 1.0 + a.opts.SessionBoostBeta
				break
			}
		}
		c.SessionBoost = sessionBoost

		// Recalculate score with all CBA factors:
		// score = ρ * δ * η * (1 - maxOverlap) * causalPenalty * sessionBoost
		c.Score = c.Relevance * c.Density * c.Recency *
			(1.0 - c.MaxOverlap) * c.CausalPenalty * c.SessionBoost

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
	dateStr := d.Date
	if len(dateStr) >= 10 {
		dateStr = dateStr[:10]
	} else if dateStr == "" {
		dateStr = "unknown"
	}
	parts := []string{fmt.Sprintf("[%s|%s]", strings.Join(d.Subject, ","), dateStr)}

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

// Priority Queue implementation
type item struct {
	candidate *types.Candidate
	priority  float64
	index     int
}
type priorityQueue []*item

func (pq priorityQueue) Len() int           { return len(pq) }
func (pq priorityQueue) Less(i, j int) bool { return pq[i].priority > pq[j].priority }
func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}
func (pq *priorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*item)
	item.index = n
	*pq = append(*pq, item)
}
func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[0 : n-1]
	return item
}
