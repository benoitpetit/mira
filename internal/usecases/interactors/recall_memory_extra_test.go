package interactors

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// ── TagRepository mock ────────────────────────────────────────────────────────

type mockTagRepo struct {
	ids []uuid.UUID
	err error
}

func (m *mockTagRepo) StoreTags(_ context.Context, _ uuid.UUID, _ []string, _ string) error {
	return nil
}
func (m *mockTagRepo) GetVerbatimsByTags(_ context.Context, _ []string, _ int) ([]uuid.UUID, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.ids, nil
}
func (m *mockTagRepo) GetTagsForVerbatim(_ context.Context, _ uuid.UUID) ([]string, error) {
	return nil, nil
}

var _ ports.TagRepository = (*mockTagRepo)(nil)

// ── Metrics spy ───────────────────────────────────────────────────────────────

type recallMetricsSpy struct {
	recallCalls int
	resultCalls int
}

func (m *recallMetricsSpy) IsEnabled() bool                             { return true }
func (m *recallMetricsSpy) RecordStore(_ time.Duration)                 {}
func (m *recallMetricsSpy) RecordRecall(_ time.Duration)                { m.recallCalls++ }
func (m *recallMetricsSpy) RecordSearch(_ time.Duration, _ bool)        {}
func (m *recallMetricsSpy) RecordEmbed(_ time.Duration)                 {}
func (m *recallMetricsSpy) RecordError()                                {}
func (m *recallMetricsSpy) RecordStoreResult(_ int)                     {}
func (m *recallMetricsSpy) RecordRecallResult(_ int, _ float64)         { m.resultCalls++ }
func (m *recallMetricsSpy) UpdateMemoryCount(_ int)                     {}
func (m *recallMetricsSpy) UpdateVectorCount(_ int)                     {}
func (m *recallMetricsSpy) GetReport(_ context.Context) ports.MetricsReport {
	return ports.MetricsReport{}
}

var _ ports.MetricsCollector = (*recallMetricsSpy)(nil)

// ── helper ────────────────────────────────────────────────────────────────────

func makeCandidate(tokens int) *entities.Candidate {
	uid := uuid.New()
	emb := make([]float32, 384)
	emb[0] = 0.9
	return &entities.Candidate{
		Memory: &entities.Fingerprint{
			ID:            uid,
			FactCount:     5,
			TokenEstimate: tokens / 2,
			Type:          valueobjects.TypeSessionNote,
		},
		Verbatim: &entities.Verbatim{
			ID:         uid,
			TokenCount: tokens,
			CreatedAt:  time.Now(),
		},
		Embedding:  emb,
		Relevance:  0.9,
	}
}

// ── getQueryEmbedding ─────────────────────────────────────────────────────────

func TestGetQueryEmbedding_CacheHit(t *testing.T) {
	interactor := createTestInteractor(nil)

	ctx := context.Background()
	// First call — populates cache.
	v1, err := interactor.getQueryEmbedding(ctx, "cached query")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	// Second call — must hit cache (returns same slice).
	v2, err := interactor.getQueryEmbedding(ctx, "cached query")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if len(v1) != len(v2) {
		t.Errorf("cache hit returned different-length vector: %d vs %d", len(v1), len(v2))
	}
}

func TestGetQueryEmbedding_ExpansionDisabled(t *testing.T) {
	vectorStore := &mockRecallVectorStore{}
	embedder := &mockRecallEmbedder{}
	config := DefaultRecallMemoryConfig()
	config.QueryExpansionEnabled = false
	interactor := NewRecallMemory(vectorStore, &mockRecallOverlapCache{}, &mockRecallCausalGraph{},
		embedder, &mockRecallRenderer{}, config, nil, nil)

	vec, err := interactor.getQueryEmbedding(context.Background(), "test query no expansion")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vec) == 0 {
		t.Error("expected non-empty vector")
	}
}

func TestGetQueryEmbedding_MultiVariantAveraging(t *testing.T) {
	// "the best way to learn programming" has enough words to generate multiple variants.
	vectorStore := &mockRecallVectorStore{}
	embedder := &mockRecallEmbedder{}
	config := DefaultRecallMemoryConfig()
	config.QueryExpansionEnabled = true
	config.QueryExpansionNumVariants = 3
	interactor := NewRecallMemory(vectorStore, &mockRecallOverlapCache{}, &mockRecallCausalGraph{},
		embedder, &mockRecallRenderer{}, config, nil, nil)

	vec, err := interactor.getQueryEmbedding(context.Background(), "the best way to learn programming effectively")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vec) == 0 {
		t.Error("expected non-empty averaged vector")
	}
}

func TestGetQueryEmbedding_EncodeError(t *testing.T) {
	vectorStore := &mockRecallVectorStore{}
	embedder := &mockRecallEmbedder{
		encodeFunc: func(_ context.Context, _ string) ([]float32, error) {
			return nil, errors.New("encode error")
		},
	}
	config := DefaultRecallMemoryConfig()
	config.QueryExpansionEnabled = false
	interactor := NewRecallMemory(vectorStore, &mockRecallOverlapCache{}, &mockRecallCausalGraph{},
		embedder, &mockRecallRenderer{}, config, nil, nil)

	_, err := interactor.getQueryEmbedding(context.Background(), "error query")
	if err == nil {
		t.Error("expected error from Encode")
	}
}

func TestGetQueryEmbedding_NilVecError(t *testing.T) {
	// expandQuery with multi-variant, but encoder always errors → sum == nil → nil vec error
	vectorStore := &mockRecallVectorStore{}
	embedder := &mockRecallEmbedder{
		encodeFunc: func(_ context.Context, _ string) ([]float32, error) {
			return nil, errors.New("all encodings fail")
		},
	}
	config := DefaultRecallMemoryConfig()
	config.QueryExpansionEnabled = true
	config.QueryExpansionNumVariants = 3
	interactor := NewRecallMemory(vectorStore, &mockRecallOverlapCache{}, &mockRecallCausalGraph{},
		embedder, &mockRecallRenderer{}, config, nil, nil)

	_, err := interactor.getQueryEmbedding(context.Background(), "the best way to learn programming effectively")
	if err == nil {
		t.Error("expected error when all variant encodings fail")
	}
}

// ── getTagBoostIDs ────────────────────────────────────────────────────────────

func TestGetTagBoostIDs_NilTagRepo(t *testing.T) {
	interactor := createTestInteractor(nil)
	// tagRepo is nil by default in createTestInteractor
	result := interactor.getTagBoostIDs(context.Background(), "some query with tags")
	if result != nil {
		t.Error("expected nil when tagRepo is nil")
	}
}

func TestGetTagBoostIDs_WithResults(t *testing.T) {
	id1, id2 := uuid.New(), uuid.New()
	tagRepo := &mockTagRepo{ids: []uuid.UUID{id1, id2}}

	config := DefaultRecallMemoryConfig()
	config.TagRepo = tagRepo
	interactor := NewRecallMemory(&mockRecallVectorStore{}, &mockRecallOverlapCache{}, &mockRecallCausalGraph{},
		&mockRecallEmbedder{}, &mockRecallRenderer{}, config, nil, nil)

	result := interactor.getTagBoostIDs(context.Background(), "query with matching tags here")
	if len(result) != 2 {
		t.Errorf("expected 2 tag boost IDs, got %d", len(result))
	}
	if !result[id1] || !result[id2] {
		t.Error("expected both IDs in result set")
	}
}

func TestGetTagBoostIDs_RepoError(t *testing.T) {
	tagRepo := &mockTagRepo{err: errors.New("db error")}

	config := DefaultRecallMemoryConfig()
	config.TagRepo = tagRepo
	interactor := NewRecallMemory(&mockRecallVectorStore{}, &mockRecallOverlapCache{}, &mockRecallCausalGraph{},
		&mockRecallEmbedder{}, &mockRecallRenderer{}, config, nil, nil)

	result := interactor.getTagBoostIDs(context.Background(), "some query with tags here")
	if result != nil {
		t.Error("expected nil on repo error")
	}
}

func TestGetTagBoostIDs_AllShortWords(t *testing.T) {
	// All words are < 4 chars or stop words → tags slice stays empty → returns nil
	tagRepo := &mockTagRepo{ids: []uuid.UUID{uuid.New()}}

	config := DefaultRecallMemoryConfig()
	config.TagRepo = tagRepo
	interactor := NewRecallMemory(&mockRecallVectorStore{}, &mockRecallOverlapCache{}, &mockRecallCausalGraph{},
		&mockRecallEmbedder{}, &mockRecallRenderer{}, config, nil, nil)

	// "the a is" → all stop words / short
	result := interactor.getTagBoostIDs(context.Background(), "the a is")
	if result != nil {
		t.Error("expected nil when no usable tags extracted")
	}
}

// ── Execute paths ─────────────────────────────────────────────────────────────

func TestRecallMemory_FallbackWings(t *testing.T) {
	callCount := 0
	vs := &mockRecallVectorStore{
		searchFunc: func(_ context.Context, _ []float32, _ int, wing, _ *string) ([]*entities.Candidate, error) {
			callCount++
			// Primary wing returns empty; fallback wing returns a candidate.
			if wing != nil && *wing == "fallback-wing" {
				return []*entities.Candidate{makeCandidate(50)}, nil
			}
			return nil, nil
		},
	}

	config := DefaultRecallMemoryConfig()
	config.EarlyPruningThreshold = 0.0
	config.ThresholdFloor = 0.0
	interactor := NewRecallMemory(vs, &mockRecallOverlapCache{}, &mockRecallCausalGraph{},
		&mockRecallEmbedder{}, &mockRecallRenderer{}, config, nil, nil)

	primaryWing := "primary-wing"
	out, err := interactor.Execute(context.Background(), RecallMemoryInput{
		Query:         "find something",
		Budget:        1000,
		Wing:          &primaryWing,
		FallbackWings: []string{"fallback-wing"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
}

func TestRecallMemory_SessionIDCacheUpdated(t *testing.T) {
	uid := uuid.New()
	emb := make([]float32, 384)
	emb[0] = 0.9
	candidate := &entities.Candidate{
		Memory: &entities.Fingerprint{
			ID:            uid,
			FactCount:     3,
			TokenEstimate: 20,
			Type:          valueobjects.TypeSessionNote,
		},
		Verbatim: &entities.Verbatim{
			ID:         uid,
			TokenCount: 40,
			CreatedAt:  time.Now(),
		},
		Embedding: emb,
		Relevance: 0.95,
	}

	interactor := createTestInteractor([]*entities.Candidate{candidate})
	interactor.earlyPruningThreshold = 0.0
	interactor.thresholdFloor = 0.0

	sessionID := "test-session-123"
	_, err := interactor.Execute(context.Background(), RecallMemoryInput{
		Query:     "test query for session",
		Budget:    500,
		SessionID: &sessionID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Calling again with the same session should work without error.
	_, err = interactor.Execute(context.Background(), RecallMemoryInput{
		Query:     "second query same session",
		Budget:    500,
		SessionID: &sessionID,
	})
	if err != nil {
		t.Fatalf("second call with same session failed: %v", err)
	}

	interactor.sessionCacheMu.RLock()
	ids := interactor.sessionCache[sessionID].ids
	interactor.sessionCacheMu.RUnlock()
	if len(ids) == 0 {
		t.Error("expected session cache to be populated after Execute")
	}
}

func TestRecallMemory_RerankerEnabledNilReranker(t *testing.T) {
	candidate := makeCandidate(50)
	vs := &mockRecallVectorStore{candidates: []*entities.Candidate{candidate}}

	config := DefaultRecallMemoryConfig()
	config.RerankerEnabled = true
	config.Reranker = nil // nil → NewHeuristicReranker() should be called automatically
	config.EarlyPruningThreshold = 0.0
	config.ThresholdFloor = 0.0
	interactor := NewRecallMemory(vs, &mockRecallOverlapCache{}, &mockRecallCausalGraph{},
		&mockRecallEmbedder{}, &mockRecallRenderer{}, config, nil, nil)

	out, err := interactor.Execute(context.Background(), RecallMemoryInput{
		Query:  "reranker test query with words",
		Budget: 500,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if interactor.reranker == nil {
		t.Error("expected HeuristicReranker to be created automatically")
	}
	_ = out
}

func TestRecallMemory_MetricsRecorded(t *testing.T) {
	mc := &recallMetricsSpy{}
	candidate := makeCandidate(50)
	vs := &mockRecallVectorStore{candidates: []*entities.Candidate{candidate}}

	config := DefaultRecallMemoryConfig()
	config.EarlyPruningThreshold = 0.0
	config.ThresholdFloor = 0.0
	interactor := NewRecallMemory(vs, &mockRecallOverlapCache{}, &mockRecallCausalGraph{},
		&mockRecallEmbedder{}, &mockRecallRenderer{}, config, mc, nil)

	_, err := interactor.Execute(context.Background(), RecallMemoryInput{
		Query:  "metrics test query",
		Budget: 500,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mc.recallCalls != 1 {
		t.Errorf("expected 1 RecordRecall call, got %d", mc.recallCalls)
	}
	if mc.resultCalls != 1 {
		t.Errorf("expected 1 RecordRecallResult call, got %d", mc.resultCalls)
	}
}

// ── reciprocalRankFusion tie-break ────────────────────────────────────────────

func TestReciprocalRankFusion_TieBreak(t *testing.T) {
	// Build candidates such that two have equal RRF scores.
	// With k=1:
	//   cf at dense rank 0 (score 1/(1+1) = 0.5, only in dense)
	//   ce at dense rank 2 + lexical rank 2 (score 1/(1+3) + 1/(1+3) = 0.5, in both)
	// ce and cf tie on score=0.5; ce wins tie-break (in both lists).
	uidA := uuid.New()
	uidB := uuid.New()
	uidPad1 := uuid.New()
	uidPad2 := uuid.New()

	newC := func(id uuid.UUID) *entities.Candidate {
		return &entities.Candidate{
			Memory:  &entities.Fingerprint{ID: id, Type: valueobjects.TypeSessionNote},
			Verbatim: &entities.Verbatim{ID: id, CreatedAt: time.Now()},
			Embedding: []float32{1},
		}
	}

	cA := newC(uidA) // will be in both lists at rank 3 each
	cB := newC(uidB) // will be in dense only at rank 1
	pad1 := newC(uidPad1)
	pad2 := newC(uidPad2)

	// dense:   [cB(rank1), pad1(rank2), cA(rank3)]
	// lexical: [pad2(rank1), pad2(rank2) ... actually needs different items]
	// Simplify: lexical: [pad1(rank1), pad2(rank2), cA(rank3)]
	dense := []*entities.Candidate{cB, pad1, cA}
	lexical := []*entities.Candidate{pad1, pad2, cA}

	fused := reciprocalRankFusion(dense, lexical, 1)
	if len(fused) == 0 {
		t.Fatal("expected non-empty fused result")
	}

	// Find positions of cA and cB
	posA, posB := -1, -1
	for i, c := range fused {
		if c.ID() == uidA {
			posA = i
		}
		if c.ID() == uidB {
			posB = i
		}
	}

	if posA == -1 || posB == -1 {
		t.Fatalf("expected both cA and cB in fused result (posA=%d, posB=%d)", posA, posB)
	}
	// cA is in both lists → should rank before or equal to cB (in one list only)
	// at equal RRF scores, cA wins tie-break
	if posA > posB {
		t.Errorf("expected cA (both lists) to rank before cB (one list only), got posA=%d posB=%d", posA, posB)
	}
}

// ── newEmbeddingCache with zero size ──────────────────────────────────────────

func TestNewEmbeddingCache_ZeroSize(t *testing.T) {
	c := newEmbeddingCache(0)
	if c.maxSize != 128 {
		t.Errorf("expected default maxSize=128 for zero input, got %d", c.maxSize)
	}
	// set and get should work
	c.set("key", []float32{1, 2, 3})
	vec, ok := c.get("key")
	if !ok {
		t.Error("expected cache hit")
	}
	if len(vec) != 3 {
		t.Errorf("expected len=3, got %d", len(vec))
	}
}
