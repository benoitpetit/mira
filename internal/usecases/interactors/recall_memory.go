// RecallMemory use case
package interactors

import (
	"container/heap"
	"container/list"
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"golang.org/x/sync/errgroup"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/benoitpetit/mira/internal/util"
	"github.com/google/uuid"
)

// RecallMemoryInput contains the input for recalling memories
type RecallMemoryInput struct {
	Query         string
	Budget        int
	Wing          *string
	Room          *string
	FallbackWings []string
	SessionID     *string
}

// RecallMemoryOutput contains the output of recalling memories
type RecallMemoryOutput struct {
	Memories    []*valueobjects.SelectedMemory `json:"memories"`
	TotalTokens int                            `json:"total_tokens"`
	BudgetUsed  float64                        `json:"budget_used"`
}

// candidateHeap is a max-heap for O(log n) extraction instead of O(n) linear scan.
// Implements heap.Interface for use with the standard library container/heap package.
type candidateHeap struct {
	candidates []*entities.Candidate
}

func (h candidateHeap) Len() int { return len(h.candidates) }
func (h candidateHeap) Less(i, j int) bool { return h.candidates[i].Score > h.candidates[j].Score }
func (h candidateHeap) Swap(i, j int) {
	h.candidates[i], h.candidates[j] = h.candidates[j], h.candidates[i]
}
func (h *candidateHeap) Push(x any) { h.candidates = append(h.candidates, x.(*entities.Candidate)) }
func (h *candidateHeap) Pop() any {
	n := len(h.candidates)
	c := h.candidates[n-1]
	h.candidates = h.candidates[:n-1]
	return c
}

// embeddingCache is an LRU cache to avoid re-computations (thread-safe)
type embeddingCache struct {
	mu      sync.RWMutex
	cache   map[string]*list.Element // key -> list element
	order   *list.List               // doubly-linked list for O(1) LRU tracking
	maxSize int
}

// cacheElement wraps a key and its associated value for LRU tracking.
type cacheElement struct {
	key      string
	vector   []float32
}

// newEmbeddingCache creates a new embedding cache.
// If maxSize is <= 0 it defaults to 128.
func newEmbeddingCache(maxSize int) *embeddingCache {
	if maxSize <= 0 {
		maxSize = 128
	}
	return &embeddingCache{
		cache:   make(map[string]*list.Element),
		order:   list.New(),
		maxSize: maxSize,
	}
}

// get retrieves a value from cache (thread-safe).
// A successful get promotes the key to most-recently-used so it is the first
// candidate for LRU eviction (front of list).
func (c *embeddingCache) get(key string) ([]float32, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.cache[key]
	if ok {
		// Promote to MRU: move element to front of list
		c.order.MoveToFront(element)
		ce := element.Value.(*cacheElement)
		return ce.vector, true
	}
	return nil, false
}

// set stores a value in cache (thread-safe)
func (c *embeddingCache) set(key string, vec []float32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if key already exists
	if element, exists := c.cache[key]; exists {
		// Key exists: move to end (MRU) and update value
		c.order.MoveToBack(element)
		ce := element.Value.(*cacheElement)
		ce.vector = vec
		return
	}

	// Key doesn't exist: check capacity and evict LRU if needed
	if c.order.Len() >= c.maxSize {
		// Evict least recently used (back of list, since we PushFront new items)
		oldest := c.order.Back()
		if oldest != nil {
			ce := oldest.Value.(*cacheElement)
			delete(c.cache, ce.key)
			c.order.Remove(oldest)
		}
	}

	// Add new key-value pair to front (new MRU)
	ce := &cacheElement{key: key, vector: vec}
	element := c.order.PushFront(ce)
	c.cache[key] = element
}

// RecallMemory implements the context budget allocation use case
type RecallMemory struct {
	vectorStore      ports.VectorStore
	overlapCache     ports.OverlapCache
	causalGraph      ports.CausalGraph
	embedder         ports.Embedder
	renderer         ports.FingerprintRenderer
	tagRepo          ports.TagRepository
	reranker         ports.Reranker
	cache            *embeddingCache
	metricsCollector ports.MetricsCollector
	logger           ports.Logger

	// Configuration
	defaultBudget                 int
	maxCandidates                 int
	earlyPruningThreshold         float64
	sessionWindowSeconds          int
	sessionBoostBeta              float64
	sessionBoostMax               float64
	causalPenaltyAlpha            float64
	diversityBoostAlpha           float64
	densitySigmoidK               float64
	densitySigmoidMu              float64
	thresholdMethod               string
	thresholdFloor                float64
	thresholdCeiling              float64
	enableFTS5                    bool
	fts5Limit                     int
	rrfK                          int
	queryExpansionEnabled         bool
	queryExpansionNumVariants     int
	searchTimeClusteringEnabled   bool
	searchTimeClusteringThreshold float64
	rerankerEnabled               bool
	rerankerTopK                  int
	sessionMemoryBoost            float64
	decayRates                    map[string]float64
	sessionCacheTTLSeconds        int

	// Session cache for multi-turn memory injection
	sessionCache     map[string]sessionCacheEntry
	sessionCacheMu   sync.RWMutex
}

// sessionCacheEntry stores cached session memory IDs with a timestamp for TTL eviction.
type sessionCacheEntry struct {
	ids    []uuid.UUID
	expires time.Time
}

// RecallMemoryConfig configures the recall interactor
type RecallMemoryConfig struct {
	DefaultBudget                 int
	MaxCandidates                 int
	EarlyPruningThreshold         float64
	SessionWindowSeconds          int
	SessionBoostBeta              float64
	SessionBoostMax               float64
	CausalPenaltyAlpha            float64
	DiversityBoostAlpha           float64
	DensitySigmoidK               float64
	DensitySigmoidMu              float64
	EmbeddingCacheSize            int
	ThresholdMethod               string
	ThresholdFloor                float64
	ThresholdCeiling              float64
	EnableFTS5                    bool
	FTS5Limit                     int
	RRFK                          int
	QueryExpansionEnabled         bool
	QueryExpansionNumVariants     int
	SearchTimeClusteringEnabled   bool
	SearchTimeClusteringThreshold float64
	RerankerEnabled               bool
	RerankerTopK                  int
	SessionMemoryBoost            float64
	SessionCacheTTLSeconds        int
	TagRepo                       ports.TagRepository
	Reranker                      ports.Reranker
	DecayRates                    map[string]float64
}

// DefaultRecallMemoryConfig returns default configuration
func DefaultRecallMemoryConfig() RecallMemoryConfig {
	return RecallMemoryConfig{
		DefaultBudget:                 4000,
		MaxCandidates:                 100,
		EarlyPruningThreshold:         0.6,
		SessionWindowSeconds:          7200,
		SessionBoostBeta:              0.2,
		SessionBoostMax:               1.2,
		CausalPenaltyAlpha:            0.15,
		DiversityBoostAlpha:           0.1,
		DensitySigmoidK:               2.0,
		DensitySigmoidMu:              0.3,
		EmbeddingCacheSize:            1000,
		ThresholdMethod:               "iqr",
		ThresholdFloor:                0.15,
		ThresholdCeiling:              0.75,
		EnableFTS5:                    true,
		FTS5Limit:                     100,
		RRFK:                          60,
		QueryExpansionEnabled:         true,
		QueryExpansionNumVariants:     3,
		SearchTimeClusteringEnabled:   true,
		SearchTimeClusteringThreshold: 0.88,
		RerankerEnabled:               false,
		RerankerTopK:                  30,
		SessionMemoryBoost:            1.3,
		SessionCacheTTLSeconds:        1800,
		DecayRates: map[string]float64{
			"decision":     0.001,
			"fact":         0.005,
			"preference":   0.01,
			"session_note": 0.1,
			"debug_log":    0.5,
		},
	}
}

// NewRecallMemory creates a new recall interactor
func NewRecallMemory(
	vectorStore ports.VectorStore,
	overlapCache ports.OverlapCache,
	causalGraph ports.CausalGraph,
	embedder ports.Embedder,
	renderer ports.FingerprintRenderer,
	config RecallMemoryConfig,
	metricsCollector ports.MetricsCollector,
	logger ports.Logger,
) *RecallMemory {
	cacheSize := config.EmbeddingCacheSize
	if cacheSize <= 0 {
		cacheSize = 1000
	}

	decayRates := config.DecayRates
	if decayRates == nil {
		decayRates = DefaultRecallMemoryConfig().DecayRates
	}

	return &RecallMemory{
		vectorStore:                   vectorStore,
		overlapCache:                  overlapCache,
		causalGraph:                   causalGraph,
		embedder:                      embedder,
		renderer:                      renderer,
		cache:                         newEmbeddingCache(cacheSize),
		metricsCollector:              metricsCollector,
		logger:                        logger,
		defaultBudget:                 config.DefaultBudget,
		maxCandidates:                 config.MaxCandidates,
		earlyPruningThreshold:         config.EarlyPruningThreshold,
		sessionWindowSeconds:          config.SessionWindowSeconds,
		sessionBoostBeta:              config.SessionBoostBeta,
		sessionBoostMax:               config.SessionBoostMax,
		causalPenaltyAlpha:            config.CausalPenaltyAlpha,
		diversityBoostAlpha:           config.DiversityBoostAlpha,
		densitySigmoidK:               config.DensitySigmoidK,
		densitySigmoidMu:              config.DensitySigmoidMu,
		thresholdMethod:               config.ThresholdMethod,
		thresholdFloor:                config.ThresholdFloor,
		thresholdCeiling:              config.ThresholdCeiling,
		enableFTS5:                    config.EnableFTS5,
		fts5Limit:                     config.FTS5Limit,
		rrfK:                          config.RRFK,
		queryExpansionEnabled:         config.QueryExpansionEnabled,
		queryExpansionNumVariants:     config.QueryExpansionNumVariants,
		searchTimeClusteringEnabled:   config.SearchTimeClusteringEnabled,
		searchTimeClusteringThreshold: config.SearchTimeClusteringThreshold,
		rerankerEnabled:               config.RerankerEnabled,
		rerankerTopK:                  config.RerankerTopK,
		sessionMemoryBoost:            config.SessionMemoryBoost,
		sessionCacheTTLSeconds:        config.SessionCacheTTLSeconds,
		tagRepo:                       config.TagRepo,
		reranker:                      config.Reranker,
		decayRates:                    decayRates,
		sessionCache:                  make(map[string]sessionCacheEntry),
	}
}

// Execute performs context budget allocation
func (uc *RecallMemory) Execute(ctx context.Context, input RecallMemoryInput) (*RecallMemoryOutput, error) {
	start := time.Now()

	uc.cleanupSessionCache()

	budget := input.Budget
	if budget <= 0 {
		budget = uc.defaultBudget
	}

	// Dynamic semantic budget adjustment based on query complexity
	queryTokens := len(strings.Fields(input.Query))
	if queryTokens < 5 {
		budget = int(float64(budget) * 0.8)
	} else if queryTokens > 50 {
		budget = int(float64(budget) * 1.2)
	}

	// 1. Get query embedding with cache
	queryVec, err := uc.getQueryEmbedding(ctx, input.Query)
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}

	// 2. Vector search (dense) and lexical search in parallel
	var denseCandidates, lexicalCandidates []*entities.Candidate
	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		denseCandidates, err = uc.vectorStore.Search(gctx, queryVec, uc.maxCandidates, input.Wing, input.Room)
		if err != nil {
			return fmt.Errorf("vector search failed: %w", err)
		}
		return nil
	})

	if uc.enableFTS5 {
		g.Go(func() error {
			lexicalCandidates, err = uc.vectorStore.SearchLexical(gctx, input.Query, uc.fts5Limit, input.Wing, input.Room)
			if err != nil {
				uc.logger.Warn("FTS5 lexical search failed, continuing without lexical candidates", "error", err)
				lexicalCandidates = nil
			}
			return nil // Don't propagate lexical errors
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// 2b. RRF fusion
	candidates := denseCandidates
	if uc.enableFTS5 && len(lexicalCandidates) > 0 {
		candidates = reciprocalRankFusion(denseCandidates, lexicalCandidates, uc.rrfK)
	}

	// 2c. Search-time clustering (deduplication)
	if uc.searchTimeClusteringEnabled {
		clusters := clusterCandidates(candidates, uc.searchTimeClusteringThreshold)
		candidates = selectClusterRepresentatives(clusters)
	}

	// 3. Tag boost lookup
	tagBoostIDs := uc.getTagBoostIDs(ctx, input.Query)

	// 4. Score and prune candidates
	scored := uc.scoreCandidates(candidates, queryVec, tagBoostIDs)
	pruned := uc.pruneCandidates(scored)

	// 4b. Broad fallback search for cross-language or sparse queries
	if len(pruned) < 3 {
		broadCandidates, err := uc.vectorStore.Search(ctx, queryVec, uc.maxCandidates*3, input.Wing, input.Room)
		if err == nil {
			broadScored := uc.scoreCandidates(broadCandidates, queryVec, tagBoostIDs)
			broadPruned := uc.pruneCandidatesWithThreshold(broadScored, 0.15)
			seen := make(map[uuid.UUID]bool)
			for _, c := range pruned {
				seen[c.ID()] = true
			}
			for _, c := range broadPruned {
				if !seen[c.ID()] {
					pruned = append(pruned, c)
					seen[c.ID()] = true
				}
			}
		}
	}

	// 4c. Fallback wings if primary wing yielded nothing useful
	if len(pruned) == 0 && len(input.FallbackWings) > 0 {
		seen := make(map[uuid.UUID]bool)
		for _, c := range pruned {
			seen[c.ID()] = true
		}
		for _, fw := range input.FallbackWings {
			if len(pruned) > 0 {
				break
			}
			fbWing := fw
			fbCandidates, err := uc.vectorStore.Search(ctx, queryVec, uc.maxCandidates, &fbWing, input.Room)
			if err != nil {
				continue
			}
			fbScored := uc.scoreCandidates(fbCandidates, queryVec, tagBoostIDs)
			fbPruned := uc.pruneCandidates(fbScored)
			for _, c := range fbPruned {
				if !seen[c.ID()] {
					pruned = append(pruned, c)
					seen[c.ID()] = true
				}
			}
		}
	}

	// 5. Heuristic reranking on top-k candidates
	if uc.rerankerEnabled && len(pruned) > 0 {
		if uc.reranker == nil {
			uc.reranker = NewHeuristicReranker()
		}
		pruned = uc.applyReranker(ctx, input.Query, pruned)
	}

	// 6. Greedy selection
	selected := uc.selectGreedy(ctx, pruned, budget, input.SessionID)

	totalTokens := 0
	for _, s := range selected {
		totalTokens += s.TokenCost
	}

	budgetUsed := float64(0)
	if budget > 0 {
		budgetUsed = float64(totalTokens) / float64(budget) * 100
	}

	// Record metrics if collector is available
	if uc.metricsCollector != nil {
		uc.metricsCollector.RecordRecall(time.Since(start))
		uc.metricsCollector.RecordRecallResult(len(selected), budgetUsed)
	}

	if uc.logger != nil {
		uc.logger.Info("Recall completed",
			"query_tokens", queryTokens,
			"budget", budget,
			"candidates", len(pruned),
			"selected", len(selected),
			"total_tokens", totalTokens,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}

	// Update session cache for multi-turn memory injection
	if input.SessionID != nil && *input.SessionID != "" {
		uc.sessionCacheMu.Lock()
		if uc.sessionCache == nil {
			uc.sessionCache = make(map[string]sessionCacheEntry)
		}
		entry, _ := uc.sessionCache[*input.SessionID]
		existing := entry.ids
		for _, sel := range selected {
			found := false
			for _, id := range existing {
				if id == sel.CandidateID {
					found = true
					break
				}
			}
			if !found {
				existing = append(existing, sel.CandidateID)
			}
		}
		if len(existing) > 20 {
			existing = existing[len(existing)-20:]
		}
		uc.sessionCache[*input.SessionID] = sessionCacheEntry{
			ids:    existing,
			expires: time.Now().Add(time.Duration(uc.sessionCacheTTLSeconds) * time.Second),
		}
		uc.sessionCacheMu.Unlock()
	}

	return &RecallMemoryOutput{
		Memories:    selected,
		TotalTokens: totalTokens,
		BudgetUsed:  budgetUsed,
	}, nil
}

func (uc *RecallMemory) getQueryEmbedding(ctx context.Context, query string) ([]float32, error) {
	// Check cache
	if vec, ok := uc.cache.get(query); ok {
		return vec, nil
	}

	var vec []float32
	var err error

	if uc.queryExpansionEnabled && uc.queryExpansionNumVariants > 0 {
		variants := expandQuery(query, uc.queryExpansionNumVariants)
		if len(variants) == 1 {
			vec, err = uc.embedder.Encode(ctx, variants[0])
		} else {
			// Average embeddings of all variants
			dim := -1
			var sum []float64
			for _, v := range variants {
				ev, eErr := uc.embedder.Encode(ctx, v)
				if eErr != nil {
					continue
				}
				if dim == -1 {
					dim = len(ev)
					sum = make([]float64, dim)
				}
				if len(ev) != dim {
					continue
				}
				for i := range ev {
					sum[i] += float64(ev[i])
				}
			}
			if sum != nil {
				vec = make([]float32, dim)
				count := float64(len(variants))
				for i := range sum {
					vec[i] = float32(sum[i] / count)
				}
			}
		}
	} else {
		vec, err = uc.embedder.Encode(ctx, query)
	}

	if err != nil {
		return nil, err
	}
	if vec == nil {
		return nil, fmt.Errorf("failed to generate query embedding")
	}

	uc.cache.set(query, vec)
	return vec, nil
}

func (uc *RecallMemory) getTagBoostIDs(ctx context.Context, query string) map[uuid.UUID]bool {
	if uc.tagRepo == nil {
		return nil
	}
	clean := punctuationRe.ReplaceAllString(query, " ")
	words := strings.Fields(clean)
	var tags []string
	for _, w := range words {
		lw := strings.ToLower(w)
		if len(lw) >= 4 && !stopWords[lw] {
			tags = append(tags, lw)
		}
	}
	if len(tags) == 0 {
		return nil
	}
	ids, err := uc.tagRepo.GetVerbatimsByTags(ctx, tags, uc.maxCandidates*2)
	if err != nil {
		return nil
	}
	set := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

func (uc *RecallMemory) scoreCandidates(candidates []*entities.Candidate, queryVec []float32, tagBoostIDs map[uuid.UUID]bool) []*entities.Candidate {
	now := time.Now()

	for _, c := range candidates {
		// ρ: semantic relevance [0,1]
		c.Relevance = util.CosineSimilarity(c.Embedding, queryVec)
		// Normalize from [-1,1] to [0,1] only if needed
		// If vectors are pre-normalized (L2), similarity is already in [0,1]
		if c.Relevance < 0 {
			c.Relevance = (1 + c.Relevance) / 2 // [-1,0) → [0,0.5)
		}

		// Tag boost (small additive boost for lexical alignment)
		if tagBoostIDs != nil && tagBoostIDs[c.ID()] {
			c.Relevance += 0.05
			if c.Relevance > 1.0 {
				c.Relevance = 1.0
			}
		}

		// δ_sig: density with sigmoid
		if c.Verbatim.TokenCount > 0 {
			rawDensity := float64(c.Memory.FactCount) / math.Sqrt(float64(c.Verbatim.TokenCount))
			c.Density = 2.0/(1.0+math.Exp(-uc.densitySigmoidK*(rawDensity-uc.densitySigmoidMu))) - 1.0
			if c.Density < 0 {
				c.Density = 0
			}
		}

		// η: recency
		ageDays := now.Sub(c.Verbatim.CreatedAt).Hours() / 24
		lambda := uc.decayRates[string(c.Memory.Type)]
		if lambda <= 0 {
			lambda = c.Memory.Type.DecayRate()
		}
		c.Recency = math.Exp(-lambda * ageDays)

		// Initial score (without overlap/causal/session)
		c.Score = c.Relevance * c.Density * c.Recency
	}

	return candidates
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	idx := p / 100.0 * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper {
		return sorted[lower]
	}
	weight := idx - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

func adaptiveThresholdMeanStddev(scores []float64) float64 {
	if len(scores) < 3 {
		return 0.3
	}
	var sum float64
	for _, s := range scores {
		sum += s
	}
	mean := sum / float64(len(scores))

	var variance float64
	for _, s := range scores {
		diff := s - mean
		variance += diff * diff
	}
	stddev := math.Sqrt(variance / float64(len(scores)))

	return mean - stddev
}

func adaptiveThresholdIQR(scores []float64) float64 {
	if len(scores) < 3 {
		return 0.3
	}
	sorted := make([]float64, len(scores))
	copy(sorted, scores)
	sort.Float64s(sorted)

	// Use the first quartile as the threshold.
	// This is more aggressive than q1 - 1.5*iqr and better handles small datasets.
	q1 := percentile(sorted, 25)
	return q1
}

func adaptiveThresholdElbow(scores []float64) float64 {
	if len(scores) < 3 {
		return 0.3
	}
	sorted := make([]float64, len(scores))
	copy(sorted, scores)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] > sorted[j]
	})

	// Compute discrete derivatives
	derivatives := make([]float64, len(sorted)-1)
	for i := 0; i < len(sorted)-1; i++ {
		derivatives[i] = sorted[i] - sorted[i+1]
	}

	var dSum float64
	for _, d := range derivatives {
		dSum += d
	}
	dMean := dSum / float64(len(derivatives))

	var dVar float64
	for _, d := range derivatives {
		dVar += (d - dMean) * (d - dMean)
	}
	dStd := math.Sqrt(dVar / float64(len(derivatives)))

	cutoff := dMean + dStd
	for i, d := range derivatives {
		if d > cutoff {
			return sorted[i+1]
		}
	}
	return sorted[len(sorted)-1]
}

// adaptiveThreshold computes a dynamic threshold based on score distribution.
func (uc *RecallMemory) adaptiveThreshold(scores []float64) float64 {
	var threshold float64
	switch uc.thresholdMethod {
	case "iqr":
		threshold = adaptiveThresholdIQR(scores)
	case "elbow":
		threshold = adaptiveThresholdElbow(scores)
	default:
		threshold = adaptiveThresholdMeanStddev(scores)
	}

	if threshold < uc.thresholdFloor {
		threshold = uc.thresholdFloor
	}
	if threshold > uc.thresholdCeiling {
		threshold = uc.thresholdCeiling
	}
	return threshold
}

func (uc *RecallMemory) pruneCandidates(candidates []*entities.Candidate) []*entities.Candidate {
	scores := make([]float64, 0, len(candidates))
	for _, c := range candidates {
		scores = append(scores, c.Relevance)
	}
	return uc.pruneCandidatesWithThreshold(candidates, uc.adaptiveThreshold(scores))
}

func (uc *RecallMemory) pruneCandidatesWithThreshold(candidates []*entities.Candidate, threshold float64) []*entities.Candidate {
	if threshold < 0.15 {
		threshold = 0.15 // hard floor for broad cross-language search
	}
	var pruned []*entities.Candidate
	for _, c := range candidates {
		if c.Relevance > threshold {
			pruned = append(pruned, c)
		}
	}

	if len(pruned) == 0 && len(candidates) > 0 {
		// Fallback: keep top 5
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].Relevance > candidates[j].Relevance
		})
		topN := 5
		if len(candidates) < topN {
			topN = len(candidates)
		}
		pruned = candidates[:topN]
	}

	return pruned
}

func (uc *RecallMemory) applyReranker(ctx context.Context, query string, candidates []*entities.Candidate) []*entities.Candidate {
	// Sort by current relevance to pick top-k
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Relevance > candidates[j].Relevance
	})

	topK := uc.rerankerTopK
	if topK > len(candidates) {
		topK = len(candidates)
	}
	topCandidates := candidates[:topK]

	contents := make([]string, len(topCandidates))
	for i, c := range topCandidates {
		contents[i] = c.Verbatim.Content
	}

	scores, err := uc.reranker.Rerank(ctx, query, contents)
	if err != nil {
		return candidates
	}

	// Blend rerank score with semantic relevance
	for i, c := range topCandidates {
		if i < len(scores) {
			c.Relevance = 0.7*c.Relevance + 0.3*scores[i]
			if c.Relevance > 1.0 {
				c.Relevance = 1.0
			}
		}
	}

	// Re-sort by blended relevance
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Relevance > candidates[j].Relevance
	})

	return candidates
}

func (uc *RecallMemory) selectGreedy(ctx context.Context, candidates []*entities.Candidate, budget int, sessionID *string) []*valueobjects.SelectedMemory {
	if len(candidates) == 0 {
		return nil
	}

	// Copy candidates for heap
	h := &candidateHeap{candidates: make([]*entities.Candidate, len(candidates))}
	copy(h.candidates, candidates)

	var selected []*valueobjects.SelectedMemory
	tokensUsed := 0
	selectedEmbeddings := make([][]float32, 0)
	selectedIDs := make(map[uuid.UUID]bool)
	selectedTimes := make([]time.Time, 0)
	coveredSubjects := make(map[string]bool)

	// Retrieve session memory IDs for multi-turn boost
	var sessionMemoryIDs map[uuid.UUID]bool
	if sessionID != nil && *sessionID != "" {
		uc.sessionCacheMu.RLock()
		if entry, ok := uc.sessionCache[*sessionID]; ok {
			if time.Now().Before(entry.expires) {
				sessionMemoryIDs = make(map[uuid.UUID]bool, len(entry.ids))
				for _, id := range entry.ids {
					sessionMemoryIDs[id] = true
				}
			}
		}
		uc.sessionCacheMu.RUnlock()
	}

	// Pre-compute adaptive threshold
	greedyThresholdScores := make([]float64, 0, len(candidates))
	for _, c := range candidates {
		greedyThresholdScores = append(greedyThresholdScores, c.Relevance)
	}
	greedyThreshold := uc.adaptiveThreshold(greedyThresholdScores)

	// needsScore tracks candidates whose overlap/causal/session scores have not yet
	// been lazily computed against the current selected set. Using an explicit set
	// avoids the previous sentinel bug: MaxOverlap == 0 is ambiguous because a
	// candidate with no overlap against selected items also has MaxOverlap == 0,
	// causing an infinite push-pop loop when selected is empty.
	needsScore := make(map[uuid.UUID]bool, len(candidates))

	// Initialize scores for all candidates
	for _, c := range h.candidates {
		initialScore := c.Relevance * c.Density * c.Recency
		if uc.sessionMemoryBoost > 0 && sessionMemoryIDs != nil && sessionMemoryIDs[c.ID()] {
			initialScore *= uc.sessionMemoryBoost
		}
		maxPossibleScore := initialScore * 1.0 * 1.0 * (1.0 + uc.sessionBoostBeta) * (1.0 + uc.diversityBoostAlpha)
		if maxPossibleScore < greedyThreshold {
			c.Score = maxPossibleScore
			c.MaxOverlap = 0
			c.CausalPenalty = 1.0
			c.SessionBoost = 1.0
			// Pre-scored: does not need lazy evaluation
		} else {
			c.Score = initialScore
			needsScore[c.ID()] = true // Needs overlap/causal/session scoring on first pop
		}
	}
	heap.Init(h)

	for h.Len() > 0 && budget-tokensUsed >= 50 {
		// Extract best from heap (O(log n))
		c := heap.Pop(h).(*entities.Candidate)

		// Lazily compute overlap and update scores on first pop.
		// We use needsScore instead of (MaxOverlap == 0 && Score > 0) because
		// MaxOverlap is genuinely 0 when there are no selected items yet, which
		// caused an infinite re-push loop under the old sentinel approach.
		if needsScore[c.ID()] {
			delete(needsScore, c.ID())
			// Overlap with selected items
			maxOverlap := 0.0
			for i, sel := range selected {
				selID := sel.CandidateID
				var overlap float64
				if uc.overlapCache != nil {
					if cached, ok := uc.overlapCache.Get(ctx, c.ID(), selID); ok {
						overlap = cached
					} else {
						overlap = util.CosineSimilarity(c.Embedding, selectedEmbeddings[i])
						uc.overlapCache.Set(ctx, c.ID(), selID, overlap)
					}
				} else {
					overlap = util.CosineSimilarity(c.Embedding, selectedEmbeddings[i])
				}
				if overlap > maxOverlap {
					maxOverlap = overlap
				}
			}
			c.MaxOverlap = maxOverlap

			// Causal penalty
			causalCount := 0
			for _, sel := range selected {
				if uc.causalGraph.HasEdge(ctx, sel.CandidateID, c.ID()) ||
					uc.causalGraph.HasEdge(ctx, c.ID(), sel.CandidateID) {
					causalCount++
				}
			}
			c.CausalPenalty = math.Exp(-uc.causalPenaltyAlpha * float64(causalCount))

			// Session boost
			sessionWindow := float64(uc.sessionWindowSeconds)
			sessionBoost := 1.0
			for _, t := range selectedTimes {
				if math.Abs(c.Verbatim.CreatedAt.Sub(t).Seconds()) < sessionWindow {
					sessionBoost = math.Min(1.0+uc.sessionBoostBeta, uc.sessionBoostMax)
					break
				}
			}
			c.SessionBoost = sessionBoost

			// Diversity boost
			diversityBoost := 1.0
			if c.Memory != nil && len(c.Memory.Subjects) > 0 {
				newSubjects := 0
				for _, s := range c.Memory.Subjects {
					if !coveredSubjects[strings.ToLower(s)] {
						newSubjects++
					}
				}
				diversityBoost = 1.0 + uc.diversityBoostAlpha*float64(newSubjects)/float64(len(c.Memory.Subjects))
			}

			initialScore := c.Relevance * c.Density * c.Recency
			if uc.sessionMemoryBoost > 0 && sessionMemoryIDs != nil && sessionMemoryIDs[c.ID()] {
				initialScore *= uc.sessionMemoryBoost
			}
			c.Score = initialScore * (1.0 - c.MaxOverlap) * c.CausalPenalty * c.SessionBoost * diversityBoost

			// Push back with updated score
			heap.Push(h, c)
			continue
		}

		if selectedIDs[c.ID()] {
			continue
		}

		// Determine render mode
		remainingBudget := budget - tokensUsed
		mode := uc.determineRenderMode(remainingBudget)
		tokenCost := uc.calculateTokenCost(c, mode)

		// Check budget and try downgrades
		// Tier order: Verbatim → Compressed (if available) → Fingerprint → Header
		if tokensUsed+tokenCost > budget {
			if mode == valueobjects.ModeVerbatim {
				if c.Verbatim.HasSummary() {
					mode = valueobjects.ModeCompressed
					tokenCost = uc.calculateTokenCost(c, mode)
					if tokensUsed+tokenCost > budget {
						mode = valueobjects.ModeFingerprint
						tokenCost = c.Memory.TokenEstimate
						if tokensUsed+tokenCost > budget {
							mode = valueobjects.ModeHeader
							tokenCost = 5
						}
					}
				} else {
					mode = valueobjects.ModeFingerprint
					tokenCost = c.Memory.TokenEstimate
					if tokensUsed+tokenCost > budget {
						mode = valueobjects.ModeHeader
						tokenCost = 5
					}
				}
			} else if mode == valueobjects.ModeFingerprint {
				mode = valueobjects.ModeHeader
				tokenCost = 5
			}

			if tokensUsed+tokenCost > budget {
				continue
			}
		}

		// Render
		rendered := uc.render(c, mode)

		sel := valueobjects.NewSelectedMemory(c.ID(), c.Verbatim.ID, mode, tokenCost, rendered, c.Score)
		selected = append(selected, sel)
		selectedEmbeddings = append(selectedEmbeddings, c.Embedding)
		selectedIDs[c.ID()] = true
		selectedTimes = append(selectedTimes, c.Verbatim.CreatedAt)
		if c.Memory != nil {
			for _, s := range c.Memory.Subjects {
				coveredSubjects[strings.ToLower(s)] = true
			}
		}
		tokensUsed += tokenCost
	}

	return selected
}

func (uc *RecallMemory) determineRenderMode(remainingBudget int) valueobjects.RenderMode {
	switch {
	case remainingBudget < 100:
		return valueobjects.ModeHeader
	case remainingBudget < 1000:
		return valueobjects.ModeFingerprint
	default:
		return valueobjects.ModeVerbatim
	}
}

func (uc *RecallMemory) calculateTokenCost(c *entities.Candidate, mode valueobjects.RenderMode) int {
	switch mode {
	case valueobjects.ModeHeader:
		return 5
	case valueobjects.ModeFingerprint:
		return c.Memory.TokenEstimate
	case valueobjects.ModeCompressed:
		if c.Verbatim.SummaryTokenCount > 0 {
			return c.Verbatim.SummaryTokenCount
		}
		// Fallback estimate: ~40% of verbatim
		return c.Verbatim.TokenCount * 40 / 100
	case valueobjects.ModeVerbatim:
		return c.Verbatim.TokenCount
	default:
		return 0
	}
}

func (uc *RecallMemory) render(c *entities.Candidate, mode valueobjects.RenderMode) string {
	switch mode {
	case valueobjects.ModeHeader:
		return uc.renderer.RenderHeader(c)
	case valueobjects.ModeFingerprint:
		return uc.renderer.RenderFingerprint(c)
	case valueobjects.ModeCompressed:
		if c.Verbatim.Summary != nil {
			return *c.Verbatim.Summary
		}
		return c.Verbatim.Content // fallback if summary absent
	case valueobjects.ModeVerbatim:
		return c.Verbatim.Content
	default:
		return ""
	}
}

// cleanupSessionCache removes expired session cache entries.
// It is called from Execute to prevent unbounded memory growth.
func (uc *RecallMemory) cleanupSessionCache() {
	if uc.sessionCacheTTLSeconds <= 0 {
		return
	}
	now := time.Now()
	uc.sessionCacheMu.Lock()
	for key, entry := range uc.sessionCache {
		if now.After(entry.expires) {
			delete(uc.sessionCache, key)
		}
	}
	uc.sessionCacheMu.Unlock()
}
