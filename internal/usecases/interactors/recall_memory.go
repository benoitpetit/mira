// RecallMemory use case
package interactors

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
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
}

// RecallMemoryOutput contains the output of recalling memories
type RecallMemoryOutput struct {
	Memories    []*valueobjects.SelectedMemory
	TotalTokens int
	BudgetUsed  float64
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
		c.cache[key] = vec
		c.order = append(c.order, key)
		return
	}

	if len(c.cache) >= c.maxSize {
		// LRU eviction
		oldest := c.order[0]
		delete(c.cache, oldest)
		c.order = c.order[1:]
	}

	c.cache[key] = vec
	c.order = append(c.order, key)
}

// RecallMemory implements the context budget allocation use case
type RecallMemory struct {
	vectorStore      ports.VectorStore
	overlapCache     ports.OverlapCache
	causalGraph      ports.CausalGraph
	embedder         ports.Embedder
	renderer         ports.FingerprintRenderer
	cache            *embeddingCache
	metricsCollector ports.MetricsCollector

	// Configuration
	defaultBudget         int
	maxCandidates         int
	earlyPruningThreshold float64
	sessionWindowSeconds  int
	sessionBoostBeta      float64
	causalPenaltyAlpha    float64
	densitySigmoidK       float64
	densitySigmoidMu      float64
}

// RecallMemoryConfig configures the recall interactor
type RecallMemoryConfig struct {
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

// DefaultRecallMemoryConfig returns default configuration
func DefaultRecallMemoryConfig() RecallMemoryConfig {
	return RecallMemoryConfig{
		DefaultBudget:         4000,
		MaxCandidates:         100,
		EarlyPruningThreshold: 0.6,
		SessionWindowSeconds:  7200,
		SessionBoostBeta:      0.2,
		CausalPenaltyAlpha:    0.15,
		DensitySigmoidK:       2.0,
		DensitySigmoidMu:      0.3,
		EmbeddingCacheSize:    1000,
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
) *RecallMemory {
	cacheSize := config.EmbeddingCacheSize
	if cacheSize <= 0 {
		cacheSize = 1000
	}

	return &RecallMemory{
		vectorStore:           vectorStore,
		overlapCache:          overlapCache,
		causalGraph:           causalGraph,
		embedder:              embedder,
		renderer:              renderer,
		cache:                 newEmbeddingCache(cacheSize),
		metricsCollector:      metricsCollector,
		defaultBudget:         config.DefaultBudget,
		maxCandidates:         config.MaxCandidates,
		earlyPruningThreshold: config.EarlyPruningThreshold,
		sessionWindowSeconds:  config.SessionWindowSeconds,
		sessionBoostBeta:      config.SessionBoostBeta,
		causalPenaltyAlpha:    config.CausalPenaltyAlpha,
		densitySigmoidK:       config.DensitySigmoidK,
		densitySigmoidMu:      config.DensitySigmoidMu,
	}
}

// Execute performs context budget allocation
func (uc *RecallMemory) Execute(ctx context.Context, input RecallMemoryInput) (*RecallMemoryOutput, error) {
	start := time.Now()

	budget := input.Budget
	if budget <= 0 {
		budget = uc.defaultBudget
	}

	// 1. Get query embedding with cache
	queryVec, err := uc.getQueryEmbedding(ctx, input.Query)
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}

	// 2. Vector search
	candidates, err := uc.vectorStore.Search(ctx, queryVec, uc.maxCandidates, input.Wing, input.Room)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	// 3. Score and prune candidates
	scored := uc.scoreCandidates(candidates, queryVec)
	pruned := uc.pruneCandidates(scored)

	// 3b. Fallback wings if primary wing yielded nothing useful
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
			fbScored := uc.scoreCandidates(fbCandidates, queryVec)
			fbPruned := uc.pruneCandidates(fbScored)
			for _, c := range fbPruned {
				if !seen[c.ID()] {
					pruned = append(pruned, c)
					seen[c.ID()] = true
				}
			}
		}
	}

	// 4. Greedy selection
	selected := uc.selectGreedy(ctx, pruned, budget)

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

	vec, err := uc.embedder.Encode(ctx, query)
	if err != nil {
		return nil, err
	}

	uc.cache.set(query, vec)
	return vec, nil
}

func (uc *RecallMemory) scoreCandidates(candidates []*entities.Candidate, queryVec []float32) []*entities.Candidate {
	now := time.Now()

	for _, c := range candidates {
		// ρ: semantic relevance [0,1]
		c.Relevance = util.CosineSimilarity(c.Embedding, queryVec)
		// Normalize from [-1,1] to [0,1] only if needed
		// If vectors are pre-normalized (L2), similarity is already in [0,1]
		if c.Relevance < 0 {
			c.Relevance = (1 + c.Relevance) / 2 // [-1,0) → [0,0.5)
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
		lambda := c.Memory.Type.DecayRate()
		c.Recency = math.Exp(-lambda * ageDays)

		// Initial score (without overlap/causal/session)
		c.Score = c.Relevance * c.Density * c.Recency
	}

	return candidates
}

// adaptiveThreshold lowers the bar for small corpora so that queries on
// databases with fewer than 10 memories still return results.
func (uc *RecallMemory) adaptiveThreshold(candidateCount int) float64 {
	if candidateCount < 10 {
		return 0.3
	}
	return uc.earlyPruningThreshold
}

func (uc *RecallMemory) pruneCandidates(candidates []*entities.Candidate) []*entities.Candidate {
	threshold := uc.adaptiveThreshold(len(candidates))
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

func (uc *RecallMemory) selectGreedy(ctx context.Context, candidates []*entities.Candidate, budget int) []*valueobjects.SelectedMemory {
	if len(candidates) == 0 {
		return nil
	}

	// Work on a copy of candidates that we can modify
	remainingCandidates := make([]*entities.Candidate, len(candidates))
	copy(remainingCandidates, candidates)

	var selected []*valueobjects.SelectedMemory
	tokensUsed := 0
	selectedEmbeddings := make([][]float32, 0)
	selectedIDs := make(map[uuid.UUID]bool)
	selectedTimes := make([]time.Time, 0)

	for len(remainingCandidates) > 0 && budget-tokensUsed >= 50 {
		// Recalculate scores for all remaining candidates
		for _, c := range remainingCandidates {
			// Initial score (without overlap) for early pruning
			initialScore := c.Relevance * c.Density * c.Recency

			// Early pruning: if the initial score is too low, skip expensive overlap calculation
			// We compute a theoretical max score (if overlap=0, causal=1, session=1+boost)
			maxPossibleScore := initialScore * 1.0 * 1.0 * (1.0 + uc.sessionBoostBeta)
			if maxPossibleScore < uc.adaptiveThreshold(len(remainingCandidates)) {
				c.Score = maxPossibleScore // Set a low score so it gets sorted to the end
				c.MaxOverlap = 0
				c.CausalPenalty = 1.0
				c.SessionBoost = 1.0
				continue
			}

			// Compute max overlap with already selected items (expensive operation)
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

			// Compute causal penalty
			causalCount := 0
			for _, sel := range selected {
				if uc.causalGraph.HasEdge(ctx, sel.CandidateID, c.ID()) ||
					uc.causalGraph.HasEdge(ctx, c.ID(), sel.CandidateID) {
					causalCount++
				}
			}
			c.CausalPenalty = math.Exp(-uc.causalPenaltyAlpha * float64(causalCount))

			// Compute session boost
			sessionWindow := float64(uc.sessionWindowSeconds)
			sessionBoost := 1.0
			for _, t := range selectedTimes {
				if math.Abs(c.Verbatim.CreatedAt.Sub(t).Seconds()) < sessionWindow {
					sessionBoost = 1.0 + uc.sessionBoostBeta
					break
				}
			}
			c.SessionBoost = sessionBoost

			// Recalculate score
			c.Score = initialScore * (1.0 - c.MaxOverlap) * c.CausalPenalty * c.SessionBoost
		}

		// Sort by score descending
		sort.Slice(remainingCandidates, func(i, j int) bool {
			return remainingCandidates[i].Score > remainingCandidates[j].Score
		})

		// Select the best candidate
		c := remainingCandidates[0]
		remainingCandidates = remainingCandidates[1:]

		if selectedIDs[c.ID()] {
			continue
		}

		// Determine render mode
		remainingBudget := budget - tokensUsed
		mode := uc.determineRenderMode(remainingBudget)
		tokenCost := uc.calculateTokenCost(c, mode)

		// Check budget and try downgrades
		if tokensUsed+tokenCost > budget {
			if mode == valueobjects.ModeVerbatim {
				mode = valueobjects.ModeFingerprint
				tokenCost = c.Memory.TokenEstimate
				if tokensUsed+tokenCost > budget {
					mode = valueobjects.ModeHeader
					tokenCost = 5
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

		sel := valueobjects.NewSelectedMemory(c.ID(), mode, tokenCost, rendered)
		selected = append(selected, sel)
		selectedEmbeddings = append(selectedEmbeddings, c.Embedding)
		selectedIDs[c.ID()] = true
		selectedTimes = append(selectedTimes, c.Verbatim.CreatedAt)
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
	case valueobjects.ModeVerbatim:
		return c.Verbatim.Content
	default:
		return ""
	}
}


