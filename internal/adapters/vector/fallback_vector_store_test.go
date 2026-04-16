package vector

import (
	"context"
	"errors"
	"testing"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

type mockPrimaryStore struct {
	searchErr        error
	searchLexicalErr error
}

func (m *mockPrimaryStore) Search(ctx context.Context, vector []float32, limit int, wing, room *string) ([]*entities.Candidate, error) {
	return nil, m.searchErr
}

func (m *mockPrimaryStore) SearchLexical(ctx context.Context, query string, limit int, wing, room *string) ([]*entities.Candidate, error) {
	return nil, m.searchLexicalErr
}

func (m *mockPrimaryStore) AddCandidate(ctx context.Context, candidate *entities.Candidate) error { return nil }
func (m *mockPrimaryStore) Delete(ctx context.Context, id uuid.UUID) error                       { return nil }
func (m *mockPrimaryStore) ClearAll(ctx context.Context) error                                    { return nil }
func (m *mockPrimaryStore) ClearByRoom(ctx context.Context, wing string, room *string) error     { return nil }

type mockFallbackStore struct {
	candidates []*entities.Candidate
}

func (m *mockFallbackStore) Search(ctx context.Context, vector []float32, limit int, wing, room *string) ([]*entities.Candidate, error) {
	return m.candidates, nil
}

func (m *mockFallbackStore) SearchLexical(ctx context.Context, query string, limit int, wing, room *string) ([]*entities.Candidate, error) {
	return m.candidates, nil
}

func (m *mockFallbackStore) AddCandidate(ctx context.Context, candidate *entities.Candidate) error { return nil }
func (m *mockFallbackStore) Delete(ctx context.Context, id uuid.UUID) error                       { return nil }
func (m *mockFallbackStore) ClearAll(ctx context.Context) error                                    { return nil }
func (m *mockFallbackStore) ClearByRoom(ctx context.Context, wing string, room *string) error     { return nil }

func TestFallbackVectorStore_Search_FallbackOnNotReady(t *testing.T) {
	v := entities.NewVerbatim("test", "w", nil)
	fp := &entities.Fingerprint{ID: v.ID, VerbatimID: v.ID, Type: valueobjects.TypeFact}
	expected := []*entities.Candidate{entities.NewCandidate(fp, v, []float32{1, 0})}

	primary := &mockPrimaryStore{searchErr: errors.New("HNSW index not ready")}
	fallback := &mockFallbackStore{candidates: expected}
	store := NewFallbackVectorStore(primary, fallback)

	res, err := store.Search(context.Background(), []float32{1}, 10, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 1 || res[0].ID() != v.ID {
		t.Errorf("expected fallback result, got %v", res)
	}
}

func TestFallbackVectorStore_Search_NoFallbackOnOtherError(t *testing.T) {
	primary := &mockPrimaryStore{searchErr: errors.New("some other error")}
	fallback := &mockFallbackStore{candidates: nil}
	store := NewFallbackVectorStore(primary, fallback)

	_, err := store.Search(context.Background(), []float32{1}, 10, nil, nil)
	if err == nil || err.Error() != "some other error" {
		t.Errorf("expected original error, got %v", err)
	}
}

func TestFallbackVectorStore_SearchLexical_FallbackOnNotReady(t *testing.T) {
	v := entities.NewVerbatim("test", "w", nil)
	fp := &entities.Fingerprint{ID: v.ID, VerbatimID: v.ID, Type: valueobjects.TypeFact}
	expected := []*entities.Candidate{entities.NewCandidate(fp, v, []float32{1, 0})}

	primary := &mockPrimaryStore{searchLexicalErr: errors.New("HNSW index not ready")}
	fallback := &mockFallbackStore{candidates: expected}
	store := NewFallbackVectorStore(primary, fallback)

	res, err := store.SearchLexical(context.Background(), "query", 10, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 1 || res[0].ID() != v.ID {
		t.Errorf("expected fallback result, got %v", res)
	}
}

func TestIsNotReady(t *testing.T) {
	if !isNotReady(errors.New("HNSW index not ready")) {
		t.Error("expected true for not ready")
	}
	if !isNotReady(errors.New("HNSW vector store is not supported on Windows")) {
		t.Error("expected true for not supported")
	}
	if isNotReady(errors.New("database locked")) {
		t.Error("expected false for unrelated error")
	}
	if isNotReady(nil) {
		t.Error("expected false for nil error")
	}
}
