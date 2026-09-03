package interactors

import (
	"context"
	"errors"
	"testing"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// mockDeleteRepo wraps mockStoreRepository and allows overriding DeleteVerbatimByID.
type mockDeleteRepo struct {
	*mockStoreRepository
	deleteFunc func(ctx context.Context, id uuid.UUID) error
}

func (m *mockDeleteRepo) DeleteVerbatimByID(ctx context.Context, id uuid.UUID) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return m.mockStoreRepository.DeleteVerbatimByID(ctx, id)
}

// mockDeleteVectorStore is a minimal VectorStore for DeleteMemory tests.
type mockDeleteVectorStore struct {
	deleteFunc func(ctx context.Context, id uuid.UUID) error
}

func (m *mockDeleteVectorStore) Search(ctx context.Context, vector []float32, limit int, wing, room *string) ([]*entities.Candidate, error) {
	return nil, nil
}
func (m *mockDeleteVectorStore) SearchLexical(ctx context.Context, query string, limit int, wing, room *string) ([]*entities.Candidate, error) {
	return nil, nil
}
func (m *mockDeleteVectorStore) AddCandidate(ctx context.Context, c *entities.Candidate) error {
	return nil
}
func (m *mockDeleteVectorStore) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return nil
}
func (m *mockDeleteVectorStore) ClearAll(ctx context.Context) error { return nil }
func (m *mockDeleteVectorStore) ClearByRoom(ctx context.Context, wing string, room *string) error {
	return nil
}

var _ ports.VectorStore = (*mockDeleteVectorStore)(nil)

// TestDeleteMemory_Success verifies that Execute removes the verbatim from both stores.
func TestDeleteMemory_Success(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()

	repo := &mockDeleteRepo{mockStoreRepository: newMockStoreRepository()}
	// Pre-populate so we can verify deletion
	repo.verbatims[id] = &entities.Verbatim{ID: id, Content: "hello", Wing: "w"}

	vectorDeleteCalled := false
	vs := &mockDeleteVectorStore{
		deleteFunc: func(_ context.Context, gotID uuid.UUID) error {
			vectorDeleteCalled = true
			if gotID != id {
				t.Errorf("vector Delete: expected id %s, got %s", id, gotID)
			}
			return nil
		},
	}

	uc := NewDeleteMemory(repo, vs)
	if err := uc.Execute(ctx, DeleteMemoryInput{ID: id}); err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}

	if _, exists := repo.verbatims[id]; exists {
		t.Error("verbatim should have been removed from repository")
	}
	if !vectorDeleteCalled {
		t.Error("vector store Delete should have been called")
	}
}

// TestDeleteMemory_RepoError verifies that a repository failure is propagated.
func TestDeleteMemory_RepoError(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	want := errors.New("db failure")

	repo := &mockDeleteRepo{
		mockStoreRepository: newMockStoreRepository(),
		deleteFunc:          func(_ context.Context, _ uuid.UUID) error { return want },
	}

	uc := NewDeleteMemory(repo, &mockDeleteVectorStore{})
	err := uc.Execute(ctx, DeleteMemoryInput{ID: id})
	if err == nil {
		t.Fatal("expected error when repository returns an error")
	}
	if !errors.Is(err, want) {
		t.Errorf("expected error wrapping %q, got %q", want, err)
	}
}

// TestDeleteMemory_VectorErrorNonFatal verifies that a vector store error does not
// cause Execute to return an error (the comment in delete_memory.go marks it non-fatal).
func TestDeleteMemory_VectorErrorNonFatal(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()

	repo := &mockDeleteRepo{mockStoreRepository: newMockStoreRepository()}
	vs := &mockDeleteVectorStore{
		deleteFunc: func(_ context.Context, _ uuid.UUID) error {
			return errors.New("vector index unavailable")
		},
	}

	uc := NewDeleteMemory(repo, vs)
	if err := uc.Execute(ctx, DeleteMemoryInput{ID: id}); err != nil {
		t.Fatalf("Execute should succeed even when vector store fails, got: %v", err)
	}
}

// TestDeleteMemory_VectorReceivesCorrectID verifies the correct UUID is forwarded.
func TestDeleteMemory_VectorReceivesCorrectID(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()

	repo := &mockDeleteRepo{mockStoreRepository: newMockStoreRepository()}
	var gotID uuid.UUID
	vs := &mockDeleteVectorStore{
		deleteFunc: func(_ context.Context, receivedID uuid.UUID) error {
			gotID = receivedID
			return nil
		},
	}

	uc := NewDeleteMemory(repo, vs)
	_ = uc.Execute(ctx, DeleteMemoryInput{ID: id})

	if gotID != id {
		t.Errorf("vector store received wrong id: want %s, got %s", id, gotID)
	}
}

// compile-time interface checks
var _ ports.Repository = (*mockDeleteRepo)(nil)
var _ ports.VectorStore = (*mockDeleteVectorStore)(nil)
