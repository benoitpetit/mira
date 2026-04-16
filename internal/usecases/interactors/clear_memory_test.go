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

// mockStatsRepositoryForClear mocks StatsRepository for ClearMemory tests
type mockStatsRepositoryForClear struct {
	clearAllFunc    func(ctx context.Context) error
	clearByRoomFunc func(ctx context.Context, wing string, room *string) (int, error)
}

func (m *mockStatsRepositoryForClear) GetStats(ctx context.Context) (*valueobjects.Stats, error) {
	return nil, nil
}

func (m *mockStatsRepositoryForClear) GetTimeline(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string, limit int, cursor *string) ([]*valueobjects.TimelineItem, error) {
	return nil, nil
}

func (m *mockStatsRepositoryForClear) ArchiveOldMemories(ctx context.Context) (*valueobjects.ArchiveResult, error) {
	return nil, nil
}

func (m *mockStatsRepositoryForClear) ClearAll(ctx context.Context) error {
	if m.clearAllFunc != nil {
		return m.clearAllFunc(ctx)
	}
	return nil
}

func (m *mockStatsRepositoryForClear) ClearByRoom(ctx context.Context, wing string, room *string) (int, error) {
	if m.clearByRoomFunc != nil {
		return m.clearByRoomFunc(ctx, wing, room)
	}
	return 0, nil
}

// mockVectorStoreForClear mocks VectorStore for ClearMemory tests
type mockVectorStoreForClear struct {
	clearAllFunc    func(ctx context.Context) error
	clearByRoomFunc func(ctx context.Context, wing string, room *string) error
}

func (m *mockVectorStoreForClear) Search(ctx context.Context, vector []float32, limit int, wing, room *string) ([]*entities.Candidate, error) {
	return nil, nil
}

func (m *mockVectorStoreForClear) SearchLexical(ctx context.Context, query string, limit int, wing, room *string) ([]*entities.Candidate, error) {
	return nil, nil
}

func (m *mockVectorStoreForClear) AddCandidate(ctx context.Context, candidate *entities.Candidate) error {
	return nil
}

func (m *mockVectorStoreForClear) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockVectorStoreForClear) ClearAll(ctx context.Context) error {
	if m.clearAllFunc != nil {
		return m.clearAllFunc(ctx)
	}
	return nil
}

func (m *mockVectorStoreForClear) ClearByRoom(ctx context.Context, wing string, room *string) error {
	if m.clearByRoomFunc != nil {
		return m.clearByRoomFunc(ctx, wing, room)
	}
	return nil
}

// TestClearMemory_Global tests clearing all memories globally
func TestClearMemory_Global(t *testing.T) {
	ctx := context.Background()

	repoCalled := false
	vectorCalled := false

	mockRepo := &mockStatsRepositoryForClear{
		clearAllFunc: func(ctx context.Context) error {
			repoCalled = true
			return nil
		},
	}
	mockVector := &mockVectorStoreForClear{
		clearAllFunc: func(ctx context.Context) error {
			vectorCalled = true
			return nil
		},
	}

	interactor := NewClearMemory(mockRepo, mockVector)
	output, err := interactor.Execute(ctx, ClearMemoryInput{Mode: "global"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !repoCalled {
		t.Error("Expected ClearAll to be called on repository")
	}
	if !vectorCalled {
		t.Error("Expected ClearAll to be called on vector store")
	}
	if output.Mode != "global" {
		t.Errorf("Expected mode 'global', got '%s'", output.Mode)
	}
}

// TestClearMemory_Room tests clearing memories by room
func TestClearMemory_Room(t *testing.T) {
	ctx := context.Background()
	room := "decisions"

	repoCalled := false
	vectorCalled := false

	mockRepo := &mockStatsRepositoryForClear{
		clearByRoomFunc: func(ctx context.Context, wing string, r *string) (int, error) {
			repoCalled = true
			if wing != "auth-service" {
				t.Errorf("Expected wing 'auth-service', got '%s'", wing)
			}
			if r == nil || *r != room {
				t.Errorf("Expected room '%s'", room)
			}
			return 42, nil
		},
	}
	mockVector := &mockVectorStoreForClear{
		clearByRoomFunc: func(ctx context.Context, wing string, r *string) error {
			vectorCalled = true
			return nil
		},
	}

	interactor := NewClearMemory(mockRepo, mockVector)
	output, err := interactor.Execute(ctx, ClearMemoryInput{Mode: "room", Wing: "auth-service", Room: &room})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !repoCalled {
		t.Error("Expected ClearByRoom to be called on repository")
	}
	if !vectorCalled {
		t.Error("Expected ClearByRoom to be called on vector store")
	}
	if output.Mode != "room" {
		t.Errorf("Expected mode 'room', got '%s'", output.Mode)
	}
	if output.DeletedCount != 42 {
		t.Errorf("Expected deleted count 42, got %d", output.DeletedCount)
	}
}

// TestClearMemory_InvalidMode tests invalid mode input
func TestClearMemory_InvalidMode(t *testing.T) {
	ctx := context.Background()
	interactor := NewClearMemory(&mockStatsRepositoryForClear{}, &mockVectorStoreForClear{})
	_, err := interactor.Execute(ctx, ClearMemoryInput{Mode: "invalid"})
	if err == nil {
		t.Fatal("Expected error for invalid mode")
	}
}

// TestClearMemory_RepoError tests repository failure
func TestClearMemory_RepoError(t *testing.T) {
	ctx := context.Background()
	mockRepo := &mockStatsRepositoryForClear{
		clearAllFunc: func(ctx context.Context) error {
			return errors.New("db failure")
		},
	}
	interactor := NewClearMemory(mockRepo, &mockVectorStoreForClear{})
	_, err := interactor.Execute(ctx, ClearMemoryInput{Mode: "global"})
	if err == nil {
		t.Fatal("Expected error when repository fails")
	}
}

// TestClearMemory_VectorError tests vector store failure
func TestClearMemory_VectorError(t *testing.T) {
	ctx := context.Background()
	mockVector := &mockVectorStoreForClear{
		clearAllFunc: func(ctx context.Context) error {
			return errors.New("vector index failure")
		},
	}
	interactor := NewClearMemory(&mockStatsRepositoryForClear{}, mockVector)
	_, err := interactor.Execute(ctx, ClearMemoryInput{Mode: "global"})
	if err == nil {
		t.Fatal("Expected error when vector store fails")
	}
}

// Ensure interfaces are implemented
var _ ports.StatsRepository = (*mockStatsRepositoryForClear)(nil)
var _ ports.VectorStore = (*mockVectorStoreForClear)(nil)
