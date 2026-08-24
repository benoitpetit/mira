package interactors

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// MockCausalGraphRepository pour les tests
type mockCausalGraphRepository struct {
	getChainFunc        func(ctx context.Context, id uuid.UUID, maxDepth int) ([]*entities.CausalNode, error)
	getConsequencesFunc func(ctx context.Context, id uuid.UUID, maxDepth int) ([]*entities.CausalNode, error)
}

func (m *mockCausalGraphRepository) AddNode(ctx context.Context, node *entities.CausalNode) error {
	return nil
}

func (m *mockCausalGraphRepository) AddNodeTx(ctx context.Context, tx *sql.Tx, node *entities.CausalNode) error {
	return nil
}

func (m *mockCausalGraphRepository) AddEdge(ctx context.Context, edge *entities.CausalEdge) error {
	return nil
}

func (m *mockCausalGraphRepository) AddEdgeTx(ctx context.Context, tx *sql.Tx, edge *entities.CausalEdge) error {
	return nil
}

func (m *mockCausalGraphRepository) HasEdge(ctx context.Context, fromID, toID uuid.UUID) bool {
	return false
}

func (m *mockCausalGraphRepository) GetChain(ctx context.Context, id uuid.UUID, maxDepth int) ([]*entities.CausalNode, error) {
	if m.getChainFunc != nil {
		return m.getChainFunc(ctx, id, maxDepth)
	}
	return nil, nil
}

func (m *mockCausalGraphRepository) GetConsequences(ctx context.Context, id uuid.UUID, maxDepth int) ([]*entities.CausalNode, error) {
	if m.getConsequencesFunc != nil {
		return m.getConsequencesFunc(ctx, id, maxDepth)
	}
	return nil, nil
}

func (m *mockCausalGraphRepository) GetParents(ctx context.Context, nodeID uuid.UUID, relations ...valueobjects.RelationType) ([]*entities.CausalNode, error) {
	return nil, nil
}

func (m *mockCausalGraphRepository) GetChildren(ctx context.Context, nodeID uuid.UUID, relations ...valueobjects.RelationType) ([]*entities.CausalNode, error) {
	return nil, nil
}

// TestGetCausalChain_Execute test avec une chaîne causale simple
func TestGetCausalChain_Execute(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	testID := uuid.New()
	room := "test-room"

	expectedChain := []*entities.CausalNode{
		{ID: uuid.New(), Type: "decision", Summary: "Root cause", Timestamp: now.Add(-2 * time.Hour), Wing: "test-wing", Room: &room},
		{ID: testID, Type: "fact", Summary: "Current node", Timestamp: now.Add(-1 * time.Hour), Wing: "test-wing", Room: &room},
	}

	mockRepo := &mockCausalGraphRepository{
		getChainFunc: func(ctx context.Context, id uuid.UUID, maxDepth int) ([]*entities.CausalNode, error) {
			if id == testID {
				return expectedChain, nil
			}
			return nil, nil
		},
	}

	interactor := NewGetCausalChain(mockRepo)
	input := GetCausalChainInput{
		ID:       testID,
		MaxDepth: 3,
	}

	output, err := interactor.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if output == nil {
		t.Fatal("Expected output, got nil")
	}

	if len(output.Chain) != 2 {
		t.Errorf("Expected 2 nodes in chain, got %d", len(output.Chain))
	}

	// Vérifier que les nœuds sont retournés dans le bon ordre
	if output.Chain[0].Summary != "Root cause" {
		t.Errorf("Expected first node summary 'Root cause', got '%s'", output.Chain[0].Summary)
	}
	if output.Chain[1].Summary != "Current node" {
		t.Errorf("Expected second node summary 'Current node', got '%s'", output.Chain[1].Summary)
	}

	// Vérifier que les conséquences ne sont pas incluses
	if len(output.Consequences) != 0 {
		t.Errorf("Expected no consequences, got %d", len(output.Consequences))
	}
}

// TestGetCausalChain_WithConsequences test avec includeConsequences=true
func TestGetCausalChain_WithConsequences(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	testID := uuid.New()
	room := "test-room"

	expectedChain := []*entities.CausalNode{
		{ID: uuid.New(), Type: "decision", Summary: "Parent", Timestamp: now.Add(-2 * time.Hour), Wing: "test-wing", Room: &room},
	}

	expectedConsequences := []*entities.CausalNode{
		{ID: uuid.New(), Type: "fact", Summary: "Child 1", Timestamp: now.Add(1 * time.Hour), Wing: "test-wing", Room: &room},
		{ID: uuid.New(), Type: "preference", Summary: "Child 2", Timestamp: now.Add(2 * time.Hour), Wing: "test-wing", Room: &room},
	}

	mockRepo := &mockCausalGraphRepository{
		getChainFunc: func(ctx context.Context, id uuid.UUID, maxDepth int) ([]*entities.CausalNode, error) {
			return expectedChain, nil
		},
		getConsequencesFunc: func(ctx context.Context, id uuid.UUID, maxDepth int) ([]*entities.CausalNode, error) {
			return expectedConsequences, nil
		},
	}

	interactor := NewGetCausalChain(mockRepo)
	input := GetCausalChainInput{
		ID:                  testID,
		MaxDepth:            3,
		IncludeConsequences: true,
	}

	output, err := interactor.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if output == nil {
		t.Fatal("Expected output, got nil")
	}

	if len(output.Chain) != 1 {
		t.Errorf("Expected 1 node in chain, got %d", len(output.Chain))
	}

	if len(output.Consequences) != 2 {
		t.Errorf("Expected 2 consequences, got %d", len(output.Consequences))
	}

	// Vérifier les conséquences
	if output.Consequences[0].Summary != "Child 1" {
		t.Errorf("Expected first consequence 'Child 1', got '%s'", output.Consequences[0].Summary)
	}
	if output.Consequences[1].Summary != "Child 2" {
		t.Errorf("Expected second consequence 'Child 2', got '%s'", output.Consequences[1].Summary)
	}
}

// TestGetCausalChain_MaxDepth test que maxDepth est respecté
func TestGetCausalChain_MaxDepth(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	testID := uuid.New()

	callCount := 0
	capturedMaxDepth := 0

	mockRepo := &mockCausalGraphRepository{
		getChainFunc: func(ctx context.Context, id uuid.UUID, maxDepth int) ([]*entities.CausalNode, error) {
			callCount++
			capturedMaxDepth = maxDepth
			return []*entities.CausalNode{
				{ID: uuid.New(), Type: "fact", Summary: "Node", Timestamp: now, Wing: "test-wing"},
			}, nil
		},
	}

	interactor := NewGetCausalChain(mockRepo)
	input := GetCausalChainInput{
		ID:       testID,
		MaxDepth: 5,
	}

	_, err := interactor.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("Expected GetChain to be called once, called %d times", callCount)
	}

	if capturedMaxDepth != 5 {
		t.Errorf("Expected MaxDepth to be 5, got %d", capturedMaxDepth)
	}
}

// TestGetCausalChain_NotFound test avec ID inexistant
func TestGetCausalChain_NotFound(t *testing.T) {
	ctx := context.Background()
	testID := uuid.New()

	mockRepo := &mockCausalGraphRepository{
		getChainFunc: func(ctx context.Context, id uuid.UUID, maxDepth int) ([]*entities.CausalNode, error) {
			// Return empty chain for unknown ID
			return []*entities.CausalNode{}, nil
		},
	}

	interactor := NewGetCausalChain(mockRepo)
	input := GetCausalChainInput{
		ID:       testID,
		MaxDepth: 3,
	}

	output, err := interactor.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute should not fail for empty chain: %v", err)
	}

	if output == nil {
		t.Fatal("Expected output, got nil")
	}

	if len(output.Chain) != 0 {
		t.Errorf("Expected empty chain, got %d nodes", len(output.Chain))
	}
}

// TestGetCausalChain_RepositoryError test erreur du repository
func TestGetCausalChain_RepositoryError(t *testing.T) {
	ctx := context.Background()
	testID := uuid.New()

	mockRepo := &mockCausalGraphRepository{
		getChainFunc: func(ctx context.Context, id uuid.UUID, maxDepth int) ([]*entities.CausalNode, error) {
			return nil, errors.New("database connection failed")
		},
	}

	interactor := NewGetCausalChain(mockRepo)
	input := GetCausalChainInput{
		ID:       testID,
		MaxDepth: 3,
	}

	output, err := interactor.Execute(ctx, input)
	if err == nil {
		t.Error("Expected error for repository failure, got nil")
	}

	if output != nil {
		t.Error("Expected nil output on error")
	}
}

// Ensure interface is implemented
var _ ports.CausalGraphRepository = (*mockCausalGraphRepository)(nil)
