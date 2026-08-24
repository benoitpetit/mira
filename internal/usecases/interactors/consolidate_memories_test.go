package interactors

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// mockConsolidateRepository embeds mockStoreRepository and overrides the methods
// used by ConsolidateMemories so we can inject test data without a real database.
type mockConsolidateRepository struct {
	*mockStoreRepository
	getTimelineFunc  func(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string, limit int, cursor *string) ([]*valueobjects.TimelineItem, error)
	getVerbatimFunc  func(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error)
	getEmbeddingFunc func(ctx context.Context, id uuid.UUID) (*entities.Embedding, error)
	clearByIDsFunc   func(ctx context.Context, ids []uuid.UUID) (int, error)
}

func (m *mockConsolidateRepository) GetTimeline(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string, limit int, cursor *string) ([]*valueobjects.TimelineItem, error) {
	if m.getTimelineFunc != nil {
		return m.getTimelineFunc(ctx, wing, room, memType, since, until, limit, cursor)
	}
	return nil, nil
}

func (m *mockConsolidateRepository) GetVerbatimByID(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error) {
	if m.getVerbatimFunc != nil {
		return m.getVerbatimFunc(ctx, id)
	}
	return m.mockStoreRepository.GetVerbatimByID(ctx, id)
}

func (m *mockConsolidateRepository) GetEmbeddingByID(ctx context.Context, id uuid.UUID) (*entities.Embedding, error) {
	if m.getEmbeddingFunc != nil {
		return m.getEmbeddingFunc(ctx, id)
	}
	return m.mockStoreRepository.GetEmbeddingByID(ctx, id)
}

func (m *mockConsolidateRepository) ClearByIDs(ctx context.Context, ids []uuid.UUID) (int, error) {
	if m.clearByIDsFunc != nil {
		return m.clearByIDsFunc(ctx, ids)
	}
	return len(ids), nil
}

// Add missing methods to satisfy the full Repository interface

func (m *mockConsolidateRepository) DB() *sql.DB { return nil }

func (m *mockConsolidateRepository) Close() error { return nil }

func (m *mockConsolidateRepository) GetCandidatesWithEmbeddings(ctx context.Context, ids []uuid.UUID, wing, room *string) ([]*entities.Candidate, error) {
	return nil, nil
}

func (m *mockConsolidateRepository) GetAllEmbeddings(ctx context.Context) ([]*entities.Embedding, error) {
	return nil, nil
}

func (m *mockConsolidateRepository) SearchLexical(ctx context.Context, query string, limit int, wing, room *string) ([]*entities.Candidate, error) {
	return nil, nil
}

func (m *mockConsolidateRepository) UpdateVerbatimSummary(_ context.Context, _ uuid.UUID, _ string, _ int) error {
	return nil
}

var _ ports.Repository = (*mockConsolidateRepository)(nil)

// mockConsolidateFingerprintExtractor is a minimal FingerprintExtractor for tests.
type mockConsolidateFingerprintExtractor struct{}

func (m *mockConsolidateFingerprintExtractor) ExtractPipeline(ctx context.Context, verbatim *entities.Verbatim, forcedType *valueobjects.MemoryType) (*entities.Fingerprint, *entities.Embedding, error) {
	memType := valueobjects.TypeFact
	if forcedType != nil {
		memType = *forcedType
	}
	fp := entities.NewFingerprint(verbatim.ID, memType, "test-model")
	emb := entities.NewEmbedding(verbatim.ID, "test-model", make([]float32, 4))
	return fp, emb, nil
}

func (m *mockConsolidateFingerprintExtractor) ModelHash() string { return "test-model" }

func (m *mockConsolidateFingerprintExtractor) DetectCausalRelations(ctx context.Context, newFp *entities.Fingerprint, recentFps []*entities.Fingerprint, verbatimContent string) ([]*entities.CausalEdge, error) {
	return nil, nil
}

func (m *mockConsolidateFingerprintExtractor) Encode(ctx context.Context, text string) ([]float32, error) {
	return make([]float32, 4), nil
}

func (m *mockConsolidateFingerprintExtractor) Summarize(ctx context.Context, texts []string) (string, error) {
	return "consolidated fact", nil
}

var _ ports.Extractor = (*mockConsolidateFingerprintExtractor)(nil)

// buildConsolidateNote creates a TimelineItem, Verbatim, and Embedding tuple with the
// given embedding vector for use in consolidation tests.
func buildConsolidateNote(id uuid.UUID, content string, emb []float32) (
	item *valueobjects.TimelineItem, verbatim *entities.Verbatim, embedding *entities.Embedding,
) {
	item = &valueobjects.TimelineItem{
		ID:      id.String(),
		Summary: "summary of " + content,
		Type:    valueobjects.TypeSessionNote,
	}
	verbatim = &entities.Verbatim{
		ID:      id,
		Content: content,
		Wing:    "test-wing",
	}
	embedding = entities.NewEmbedding(id, "test-model", emb)
	return
}

// TestConsolidate_FewerThanTwoNotes verifies the early-return path when there is
// nothing to consolidate.
func TestConsolidate_FewerThanTwoNotes(t *testing.T) {
	ctx := context.Background()

	id1 := uuid.New()
	item1, v1, e1 := buildConsolidateNote(id1, "only one note", []float32{1, 0, 0, 0})

	repo := &mockConsolidateRepository{
		mockStoreRepository: newMockStoreRepository(),
		getTimelineFunc: func(_ context.Context, _ string, _ *string, _ *valueobjects.MemoryType, _, _ *string, _ int, _ *string) ([]*valueobjects.TimelineItem, error) {
			return []*valueobjects.TimelineItem{item1}, nil
		},
		getVerbatimFunc: func(_ context.Context, id uuid.UUID) (*entities.Verbatim, error) {
			if id == id1 {
				return v1, nil
			}
			return nil, errors.New("not found")
		},
		getEmbeddingFunc: func(_ context.Context, id uuid.UUID) (*entities.Embedding, error) {
			if id == id1 {
				return e1, nil
			}
			return nil, errors.New("not found")
		},
	}

	uc := NewConsolidateMemories(repo, &mockStoreVectorStore{}, nil, &mockConsolidateFingerprintExtractor{})
	out, err := uc.Execute(ctx, ConsolidateMemoriesInput{Wing: "test-wing"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out.ConsolidatedCount != 0 {
		t.Errorf("ConsolidatedCount: want 0, got %d", out.ConsolidatedCount)
	}
	if out.RemovedCount != 0 {
		t.Errorf("RemovedCount: want 0, got %d", out.RemovedCount)
	}
}

// TestConsolidate_SimilarNotesAreMerged verifies that two notes with identical
// embeddings (cosine similarity = 1.0 ≥ default threshold 0.92) are consolidated.
func TestConsolidate_SimilarNotesAreMerged(t *testing.T) {
	ctx := context.Background()

	id1, id2 := uuid.New(), uuid.New()
	// Both notes have the same embedding direction → cosine similarity = 1.0
	sameVec := []float32{1, 0, 0, 0}
	item1, v1, e1 := buildConsolidateNote(id1, "first similar note", sameVec)
	item2, v2, e2 := buildConsolidateNote(id2, "second similar note", sameVec)

	verbatims := map[uuid.UUID]*entities.Verbatim{id1: v1, id2: v2}
	embeddings := map[uuid.UUID]*entities.Embedding{id1: e1, id2: e2}

	addCandidateCalled := false
	vs := &mockStoreVectorStore{}

	var removedIDs []uuid.UUID

	repo := &mockConsolidateRepository{
		mockStoreRepository: newMockStoreRepository(),
		getTimelineFunc: func(_ context.Context, _ string, _ *string, _ *valueobjects.MemoryType, _, _ *string, _ int, _ *string) ([]*valueobjects.TimelineItem, error) {
			return []*valueobjects.TimelineItem{item1, item2}, nil
		},
		getVerbatimFunc: func(_ context.Context, id uuid.UUID) (*entities.Verbatim, error) {
			if v, ok := verbatims[id]; ok {
				return v, nil
			}
			return nil, errors.New("not found")
		},
		getEmbeddingFunc: func(_ context.Context, id uuid.UUID) (*entities.Embedding, error) {
			if e, ok := embeddings[id]; ok {
				return e, nil
			}
			return nil, errors.New("not found")
		},
		clearByIDsFunc: func(_ context.Context, ids []uuid.UUID) (int, error) {
			removedIDs = append(removedIDs, ids...)
			return len(ids), nil
		},
	}

	// Use a capturing vector store to verify AddCandidate is called.
	capturingVS := &mockConsolidateVectorStore{
		addFunc: func(_ context.Context, _ *entities.Candidate) error {
			addCandidateCalled = true
			return nil
		},
	}
	_ = vs

	uc := NewConsolidateMemories(repo, capturingVS, nil, &mockConsolidateFingerprintExtractor{})
	out, err := uc.Execute(ctx, ConsolidateMemoriesInput{Wing: "test-wing"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if out.ConsolidatedCount != 1 {
		t.Errorf("ConsolidatedCount: want 1, got %d", out.ConsolidatedCount)
	}
	if out.RemovedCount != 2 {
		t.Errorf("RemovedCount: want 2, got %d", out.RemovedCount)
	}
	if !addCandidateCalled {
		t.Error("AddCandidate should have been called on vector store")
	}
	if len(removedIDs) != 2 {
		t.Errorf("expected 2 IDs removed, got %d", len(removedIDs))
	}
}

// TestConsolidate_DissimilarNotesAreNotMerged verifies that orthogonal notes
// (cosine similarity = 0.0) do not form a cluster.
func TestConsolidate_DissimilarNotesAreNotMerged(t *testing.T) {
	ctx := context.Background()

	id1, id2 := uuid.New(), uuid.New()
	// Orthogonal vectors → cosine similarity = 0.0 < default threshold 0.92
	item1, v1, e1 := buildConsolidateNote(id1, "note about topic A", []float32{1, 0, 0, 0})
	item2, v2, e2 := buildConsolidateNote(id2, "note about topic B", []float32{0, 1, 0, 0})

	verbatims := map[uuid.UUID]*entities.Verbatim{id1: v1, id2: v2}
	embeddings := map[uuid.UUID]*entities.Embedding{id1: e1, id2: e2}

	repo := &mockConsolidateRepository{
		mockStoreRepository: newMockStoreRepository(),
		getTimelineFunc: func(_ context.Context, _ string, _ *string, _ *valueobjects.MemoryType, _, _ *string, _ int, _ *string) ([]*valueobjects.TimelineItem, error) {
			return []*valueobjects.TimelineItem{item1, item2}, nil
		},
		getVerbatimFunc: func(_ context.Context, id uuid.UUID) (*entities.Verbatim, error) {
			if v, ok := verbatims[id]; ok {
				return v, nil
			}
			return nil, errors.New("not found")
		},
		getEmbeddingFunc: func(_ context.Context, id uuid.UUID) (*entities.Embedding, error) {
			if e, ok := embeddings[id]; ok {
				return e, nil
			}
			return nil, errors.New("not found")
		},
	}

	uc := NewConsolidateMemories(repo, &mockStoreVectorStore{}, nil, &mockConsolidateFingerprintExtractor{})
	out, err := uc.Execute(ctx, ConsolidateMemoriesInput{Wing: "test-wing"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if out.ConsolidatedCount != 0 {
		t.Errorf("ConsolidatedCount: want 0, got %d", out.ConsolidatedCount)
	}
	if out.RemovedCount != 0 {
		t.Errorf("RemovedCount: want 0, got %d", out.RemovedCount)
	}
}

// TestConsolidate_GetTimelineError verifies that a repository error on GetTimeline
// is propagated as an error from Execute.
func TestConsolidate_GetTimelineError(t *testing.T) {
	ctx := context.Background()
	want := errors.New("timeline unavailable")

	repo := &mockConsolidateRepository{
		mockStoreRepository: newMockStoreRepository(),
		getTimelineFunc: func(_ context.Context, _ string, _ *string, _ *valueobjects.MemoryType, _, _ *string, _ int, _ *string) ([]*valueobjects.TimelineItem, error) {
			return nil, want
		},
	}

	uc := NewConsolidateMemories(repo, &mockStoreVectorStore{}, nil, &mockConsolidateFingerprintExtractor{})
	_, err := uc.Execute(ctx, ConsolidateMemoriesInput{Wing: "test-wing"})
	if err == nil {
		t.Fatal("expected error when GetTimeline fails")
	}
	if !errors.Is(err, want) {
		t.Errorf("expected error wrapping %q, got %q", want, err)
	}
}

// TestConsolidate_CustomThreshold verifies that a custom similarity threshold is respected.
func TestConsolidate_CustomThreshold(t *testing.T) {
	ctx := context.Background()

	id1, id2 := uuid.New(), uuid.New()
	// Cosine similarity ≈ 0.71 (45° apart) — above 0.5 but below 0.92 (default)
	item1, v1, e1 := buildConsolidateNote(id1, "note A", []float32{1, 1, 0, 0})
	item2, v2, e2 := buildConsolidateNote(id2, "note B", []float32{1, 0, 0, 0})

	verbatims := map[uuid.UUID]*entities.Verbatim{id1: v1, id2: v2}
	embeddings := map[uuid.UUID]*entities.Embedding{id1: e1, id2: e2}

	repo := &mockConsolidateRepository{
		mockStoreRepository: newMockStoreRepository(),
		getTimelineFunc: func(_ context.Context, _ string, _ *string, _ *valueobjects.MemoryType, _, _ *string, _ int, _ *string) ([]*valueobjects.TimelineItem, error) {
			return []*valueobjects.TimelineItem{item1, item2}, nil
		},
		getVerbatimFunc: func(_ context.Context, id uuid.UUID) (*entities.Verbatim, error) {
			if v, ok := verbatims[id]; ok {
				return v, nil
			}
			return nil, errors.New("not found")
		},
		getEmbeddingFunc: func(_ context.Context, id uuid.UUID) (*entities.Embedding, error) {
			if e, ok := embeddings[id]; ok {
				return e, nil
			}
			return nil, errors.New("not found")
		},
	}

	ucLow := NewConsolidateMemories(repo, &mockStoreVectorStore{}, nil, &mockConsolidateFingerprintExtractor{})
	outLow, err := ucLow.Execute(ctx, ConsolidateMemoriesInput{Wing: "test-wing", SimilarityThreshold: 0.5})
	if err != nil {
		t.Fatalf("Execute (low threshold) failed: %v", err)
	}
	// At threshold 0.5, the two notes SHOULD cluster.
	if outLow.ConsolidatedCount != 1 {
		t.Errorf("threshold=0.5: ConsolidatedCount want 1, got %d", outLow.ConsolidatedCount)
	}

	ucHigh := NewConsolidateMemories(repo, &mockStoreVectorStore{}, nil, &mockConsolidateFingerprintExtractor{})
	outHigh, err := ucHigh.Execute(ctx, ConsolidateMemoriesInput{Wing: "test-wing", SimilarityThreshold: 0.99})
	if err != nil {
		t.Fatalf("Execute (high threshold) failed: %v", err)
	}
	// At threshold 0.99, cosine ≈ 0.71 is below → no cluster.
	if outHigh.ConsolidatedCount != 0 {
		t.Errorf("threshold=0.99: ConsolidatedCount want 0, got %d", outHigh.ConsolidatedCount)
	}
}

// mockConsolidateVectorStore captures AddCandidate calls for consolidation tests.
type mockConsolidateVectorStore struct {
	addFunc func(ctx context.Context, c *entities.Candidate) error
}

func (m *mockConsolidateVectorStore) Search(ctx context.Context, vector []float32, limit int, wing, room *string) ([]*entities.Candidate, error) {
	return nil, nil
}
func (m *mockConsolidateVectorStore) SearchLexical(ctx context.Context, query string, limit int, wing, room *string) ([]*entities.Candidate, error) {
	return nil, nil
}
func (m *mockConsolidateVectorStore) AddCandidate(ctx context.Context, c *entities.Candidate) error {
	if m.addFunc != nil {
		return m.addFunc(ctx, c)
	}
	return nil
}
func (m *mockConsolidateVectorStore) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (m *mockConsolidateVectorStore) ClearAll(ctx context.Context) error             { return nil }
func (m *mockConsolidateVectorStore) ClearByRoom(ctx context.Context, wing string, room *string) error {
	return nil
}

var _ ports.VectorStore = (*mockConsolidateVectorStore)(nil)
