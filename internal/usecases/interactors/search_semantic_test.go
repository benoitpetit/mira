package interactors

import (
	"context"
	"errors"
	"testing"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// mockSemanticEmbedder is a minimal Embedder for SearchSemantic tests.
type mockSemanticEmbedder struct {
	encodeFunc func(ctx context.Context, text string) ([]float32, error)
}

func (m *mockSemanticEmbedder) Encode(ctx context.Context, text string) ([]float32, error) {
	if m.encodeFunc != nil {
		return m.encodeFunc(ctx, text)
	}
	// Return a unit vector with a single non-zero component so cosine similarity
	// between the query and identical candidate vectors equals 1.0.
	v := make([]float32, 4)
	v[0] = 1.0
	return v, nil
}

// mockSemanticVectorStore is a minimal VectorStore for SearchSemantic tests.
type mockSemanticVectorStore struct {
	searchFunc func(ctx context.Context, vector []float32, limit int, wing, room *string) ([]*entities.Candidate, error)
}

func (m *mockSemanticVectorStore) Search(ctx context.Context, vector []float32, limit int, wing, room *string) ([]*entities.Candidate, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, vector, limit, wing, room)
	}
	return nil, nil
}
func (m *mockSemanticVectorStore) SearchLexical(ctx context.Context, query string, limit int, wing, room *string) ([]*entities.Candidate, error) {
	return nil, nil
}
func (m *mockSemanticVectorStore) AddCandidate(ctx context.Context, c *entities.Candidate) error {
	return nil
}
func (m *mockSemanticVectorStore) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (m *mockSemanticVectorStore) ClearAll(ctx context.Context) error             { return nil }
func (m *mockSemanticVectorStore) ClearByRoom(ctx context.Context, wing string, room *string) error {
	return nil
}

var _ ports.VectorStore = (*mockSemanticVectorStore)(nil)
var _ ports.Embedder = (*mockSemanticEmbedder)(nil)

// buildSemanticCandidate creates a test Candidate whose embedding is the provided vector.
func buildSemanticCandidate(id uuid.UUID, content string, memType valueobjects.MemoryType, emb []float32) *entities.Candidate {
	verbatim := &entities.Verbatim{
		ID:      id,
		Content: content,
		Wing:    "test-wing",
	}
	fp := entities.NewFingerprint(id, memType, "test-model")
	return entities.NewCandidate(fp, verbatim, emb)
}

// TestSearchSemantic_ReturnsMatchesAboveThreshold verifies that candidates whose
// cosine similarity to the query vector is above the threshold are returned.
func TestSearchSemantic_ReturnsMatchesAboveThreshold(t *testing.T) {
	ctx := context.Background()

	queryVec := []float32{1, 0, 0, 0}
	// Same direction → cosine similarity = 1.0 (above any reasonable threshold)
	highSimVec := []float32{1, 0, 0, 0}
	// Orthogonal → cosine similarity = 0.0 (below threshold 0.5)
	lowSimVec := []float32{0, 1, 0, 0}

	idHigh := uuid.New()
	idLow := uuid.New()

	candidates := []*entities.Candidate{
		buildSemanticCandidate(idHigh, "high similarity", valueobjects.TypeFact, highSimVec),
		buildSemanticCandidate(idLow, "low similarity", valueobjects.TypeFact, lowSimVec),
	}

	embedder := &mockSemanticEmbedder{
		encodeFunc: func(_ context.Context, _ string) ([]float32, error) { return queryVec, nil },
	}
	vs := &mockSemanticVectorStore{
		searchFunc: func(_ context.Context, _ []float32, _ int, _, _ *string) ([]*entities.Candidate, error) {
			return candidates, nil
		},
	}

	uc := NewSearchSemantic(vs, embedder)
	results, err := uc.Execute(ctx, SearchSemanticInput{
		Query:     "test query",
		TopK:      10,
		Threshold: 0.5,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result above threshold, got %d", len(results))
	}
	if results[0].ID != idHigh {
		t.Errorf("expected result id %s, got %s", idHigh, results[0].ID)
	}
}

// TestSearchSemantic_DefaultTopK verifies TopK defaults to 10 when not set.
func TestSearchSemantic_DefaultTopK(t *testing.T) {
	ctx := context.Background()
	var receivedLimit int

	embedder := &mockSemanticEmbedder{}
	vs := &mockSemanticVectorStore{
		searchFunc: func(_ context.Context, _ []float32, limit int, _, _ *string) ([]*entities.Candidate, error) {
			receivedLimit = limit
			return nil, nil
		},
	}

	uc := NewSearchSemantic(vs, embedder)
	_, _ = uc.Execute(ctx, SearchSemanticInput{Query: "q", TopK: 0, Threshold: 0.0})

	if receivedLimit != 10 {
		t.Errorf("expected default TopK=10, got %d", receivedLimit)
	}
}

// TestSearchSemantic_EmbedError verifies that an encoder failure is propagated.
func TestSearchSemantic_EmbedError(t *testing.T) {
	ctx := context.Background()
	want := errors.New("encoder down")

	embedder := &mockSemanticEmbedder{
		encodeFunc: func(_ context.Context, _ string) ([]float32, error) { return nil, want },
	}
	uc := NewSearchSemantic(&mockSemanticVectorStore{}, embedder)
	_, err := uc.Execute(ctx, SearchSemanticInput{Query: "q", TopK: 5})
	if err == nil {
		t.Fatal("expected error from embedder")
	}
	if !errors.Is(err, want) {
		t.Errorf("expected error wrapping %q, got %q", want, err)
	}
}

// TestSearchSemantic_VectorStoreError verifies that a vector store failure is propagated.
func TestSearchSemantic_VectorStoreError(t *testing.T) {
	ctx := context.Background()
	want := errors.New("vector store down")

	embedder := &mockSemanticEmbedder{}
	vs := &mockSemanticVectorStore{
		searchFunc: func(_ context.Context, _ []float32, _ int, _, _ *string) ([]*entities.Candidate, error) {
			return nil, want
		},
	}

	uc := NewSearchSemantic(vs, embedder)
	_, err := uc.Execute(ctx, SearchSemanticInput{Query: "q", TopK: 5})
	if err == nil {
		t.Fatal("expected error from vector store")
	}
	if !errors.Is(err, want) {
		t.Errorf("expected error wrapping %q, got %q", want, err)
	}
}

// TestSearchSemantic_ZeroThreshold verifies that with threshold=0, all candidates pass.
func TestSearchSemantic_ZeroThreshold(t *testing.T) {
	ctx := context.Background()

	candidates := []*entities.Candidate{
		buildSemanticCandidate(uuid.New(), "a", valueobjects.TypeFact, []float32{1, 0, 0, 0}),
		buildSemanticCandidate(uuid.New(), "b", valueobjects.TypeFact, []float32{0, 1, 0, 0}),
		buildSemanticCandidate(uuid.New(), "c", valueobjects.TypeFact, []float32{0, 0, 1, 0}),
	}

	embedder := &mockSemanticEmbedder{}
	vs := &mockSemanticVectorStore{
		searchFunc: func(_ context.Context, _ []float32, _ int, _, _ *string) ([]*entities.Candidate, error) {
			return candidates, nil
		},
	}

	uc := NewSearchSemantic(vs, embedder)
	results, err := uc.Execute(ctx, SearchSemanticInput{Query: "q", TopK: 10, Threshold: 0.0})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// queryVec = [1,0,0,0]; candidates b and c are orthogonal (sim=0) which is >= 0.0
	// Only "a" has sim=1.0; "b" has sim=0.0, "c" has sim=0.0; 0.0 >= 0.0 is true → all 3 pass
	if len(results) != 3 {
		t.Errorf("expected 3 results with threshold 0.0, got %d", len(results))
	}
}

// TestSearchSemantic_ResultFields verifies that result fields are correctly populated.
func TestSearchSemantic_ResultFields(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	room := "decisions"

	verbatim := &entities.Verbatim{
		ID:      id,
		Content: "important decision",
		Wing:    "auth-service",
		Room:    &room,
	}
	fp := entities.NewFingerprint(id, valueobjects.TypeDecision, "test-model")
	c := entities.NewCandidate(fp, verbatim, []float32{1, 0, 0, 0})

	embedder := &mockSemanticEmbedder{
		encodeFunc: func(_ context.Context, _ string) ([]float32, error) {
			return []float32{1, 0, 0, 0}, nil
		},
	}
	vs := &mockSemanticVectorStore{
		searchFunc: func(_ context.Context, _ []float32, _ int, _, _ *string) ([]*entities.Candidate, error) {
			return []*entities.Candidate{c}, nil
		},
	}

	uc := NewSearchSemantic(vs, embedder)
	results, err := uc.Execute(ctx, SearchSemanticInput{Query: "q", TopK: 5, Threshold: 0.5})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.ID != id {
		t.Errorf("ID: want %s, got %s", id, r.ID)
	}
	if r.Content != "important decision" {
		t.Errorf("Content: want %q, got %q", "important decision", r.Content)
	}
	if r.Type != string(valueobjects.TypeDecision) {
		t.Errorf("Type: want %q, got %q", valueobjects.TypeDecision, r.Type)
	}
	if r.Wing != "auth-service" {
		t.Errorf("Wing: want %q, got %q", "auth-service", r.Wing)
	}
	if r.Room == nil || *r.Room != room {
		t.Errorf("Room: want %q, got %v", room, r.Room)
	}
	if r.Similarity < 0.99 {
		t.Errorf("Similarity: expected ~1.0, got %f", r.Similarity)
	}
}
