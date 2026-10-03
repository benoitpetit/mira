package interactors

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// MockCausalGraphRepository for tests
type mockCausalGraphRepository struct {
	getChainFunc        func(ctx context.Context, id uuid.UUID, maxDepth, maxNodes int) ([]*entities.CausalNode, bool, error)
	getConsequencesFunc func(ctx context.Context, id uuid.UUID, maxDepth, maxNodes int) ([]*entities.CausalNode, bool, error)
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

func (m *mockCausalGraphRepository) GetChain(ctx context.Context, id uuid.UUID, maxDepth, maxNodes int) ([]*entities.CausalNode, bool, error) {
	if m.getChainFunc != nil {
		return m.getChainFunc(ctx, id, maxDepth, maxNodes)
	}
	return nil, false, nil
}

func (m *mockCausalGraphRepository) GetConsequences(ctx context.Context, id uuid.UUID, maxDepth, maxNodes int) ([]*entities.CausalNode, bool, error) {
	if m.getConsequencesFunc != nil {
		return m.getConsequencesFunc(ctx, id, maxDepth, maxNodes)
	}
	return nil, false, nil
}

func (m *mockCausalGraphRepository) GetParents(ctx context.Context, nodeID uuid.UUID, relations ...valueobjects.RelationType) ([]*entities.CausalNode, error) {
	return nil, nil
}

func (m *mockCausalGraphRepository) GetChildren(ctx context.Context, nodeID uuid.UUID, relations ...valueobjects.RelationType) ([]*entities.CausalNode, error) {
	return nil, nil
}

// TestGetCausalChain_Execute test with a simple causal chain
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
		getChainFunc: func(ctx context.Context, id uuid.UUID, maxDepth, maxNodes int) ([]*entities.CausalNode, bool, error) {
			if id == testID {
				return expectedChain, false, nil
			}
			return nil, false, nil
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

	// Verify that nodes are returned in the correct order
	if output.Chain[0].Summary != "Root cause" {
		t.Errorf("Expected first node summary 'Root cause', got '%s'", output.Chain[0].Summary)
	}
	if output.Chain[1].Summary != "Current node" {
		t.Errorf("Expected second node summary 'Current node', got '%s'", output.Chain[1].Summary)
	}

	// Verify that consequences are not included
	if len(output.Consequences) != 0 {
		t.Errorf("Expected no consequences, got %d", len(output.Consequences))
	}
}

// TestGetCausalChain_WithConsequences test with includeConsequences=true
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
		getChainFunc: func(ctx context.Context, id uuid.UUID, maxDepth, maxNodes int) ([]*entities.CausalNode, bool, error) {
			return expectedChain, false, nil
		},
		getConsequencesFunc: func(ctx context.Context, id uuid.UUID, maxDepth, maxNodes int) ([]*entities.CausalNode, bool, error) {
			return expectedConsequences, false, nil
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

	// Check the consequences
	if output.Consequences[0].Summary != "Child 1" {
		t.Errorf("Expected first consequence 'Child 1', got '%s'", output.Consequences[0].Summary)
	}
	if output.Consequences[1].Summary != "Child 2" {
		t.Errorf("Expected second consequence 'Child 2', got '%s'", output.Consequences[1].Summary)
	}
}

// TestGetCausalChain_MaxDepth test that maxDepth is respected
func TestGetCausalChain_MaxDepth(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	testID := uuid.New()

	callCount := 0
	capturedMaxDepth := 0

	mockRepo := &mockCausalGraphRepository{
		getChainFunc: func(ctx context.Context, id uuid.UUID, maxDepth, maxNodes int) ([]*entities.CausalNode, bool, error) {
			callCount++
			capturedMaxDepth = maxDepth
			return []*entities.CausalNode{
				{ID: uuid.New(), Type: "fact", Summary: "Node", Timestamp: now, Wing: "test-wing"},
			}, false, nil
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

// TestGetCausalChain_NotFound test with non-existent ID
func TestGetCausalChain_NotFound(t *testing.T) {
	ctx := context.Background()
	testID := uuid.New()

	mockRepo := &mockCausalGraphRepository{
		getChainFunc: func(ctx context.Context, id uuid.UUID, maxDepth, maxNodes int) ([]*entities.CausalNode, bool, error) {
			// Return empty chain for unknown ID
			return []*entities.CausalNode{}, false, nil
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

// TestGetCausalChain_RepositoryError test repository error
func TestGetCausalChain_RepositoryError(t *testing.T) {
	ctx := context.Background()
	testID := uuid.New()

	mockRepo := &mockCausalGraphRepository{
		getChainFunc: func(ctx context.Context, id uuid.UUID, maxDepth, maxNodes int) ([]*entities.CausalNode, bool, error) {
			return nil, false, errors.New("database connection failed")
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

func TestGetCausalChain_DefaultDepthAndNodeBudget(t *testing.T) {
	var gotDepth, gotNodes int
	chainNode := &entities.CausalNode{ID: uuid.New()}
	repo := &mockCausalGraphRepository{
		getChainFunc: func(_ context.Context, _ uuid.UUID, depth, nodes int) ([]*entities.CausalNode, bool, error) {
			gotDepth, gotNodes = depth, nodes
			return []*entities.CausalNode{chainNode}, false, nil
		},
	}
	out, err := NewGetCausalChain(repo).Execute(context.Background(), GetCausalChainInput{ID: uuid.New()})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotDepth != valueobjects.DefaultCausalMaxDepth || gotNodes != valueobjects.MaxCausalNodes {
		t.Fatalf("repository limits = depth %d, nodes %d; want %d and %d", gotDepth, gotNodes, valueobjects.DefaultCausalMaxDepth, valueobjects.MaxCausalNodes)
	}
	if out.Truncated {
		t.Fatal("Truncated = true for a response below the cap")
	}
}

func TestGetCausalChainRejectsDepthOutsideApprovedRange(t *testing.T) {
	for _, depth := range []int{-1, valueobjects.MaxCausalDepth + 1} {
		t.Run(fmt.Sprintf("depth_%d", depth), func(t *testing.T) {
			called := false
			repo := &mockCausalGraphRepository{getChainFunc: func(context.Context, uuid.UUID, int, int) ([]*entities.CausalNode, bool, error) {
				called = true
				return nil, false, nil
			}}
			_, err := NewGetCausalChain(repo).Execute(context.Background(), GetCausalChainInput{ID: uuid.New(), MaxDepth: depth})
			if err == nil {
				t.Fatalf("Execute() accepted depth %d", depth)
			}
			if called {
				t.Fatal("repository called for invalid depth")
			}
		})
	}
}

func TestGetCausalChainAcceptsMaximumDepth(t *testing.T) {
	var gotDepth int
	repo := &mockCausalGraphRepository{getChainFunc: func(_ context.Context, _ uuid.UUID, depth, _ int) ([]*entities.CausalNode, bool, error) {
		gotDepth = depth
		return nil, false, nil
	}}
	if _, err := NewGetCausalChain(repo).Execute(context.Background(), GetCausalChainInput{ID: uuid.New(), MaxDepth: valueobjects.MaxCausalDepth}); err != nil {
		t.Fatalf("Execute() rejected max depth %d: %v", valueobjects.MaxCausalDepth, err)
	}
	if gotDepth != valueobjects.MaxCausalDepth {
		t.Fatalf("repository max depth = %d, want %d", gotDepth, valueobjects.MaxCausalDepth)
	}
}

func TestGetCausalChainSharesNodeBudgetAndPropagatesConsequenceErrors(t *testing.T) {
	var consequenceBudget int
	chain := make([]*entities.CausalNode, 200)
	for i := range chain {
		chain[i] = &entities.CausalNode{ID: uuid.New()}
	}
	repo := &mockCausalGraphRepository{
		getChainFunc: func(context.Context, uuid.UUID, int, int) ([]*entities.CausalNode, bool, error) {
			return chain, false, nil
		},
		getConsequencesFunc: func(_ context.Context, _ uuid.UUID, _ int, nodes int) ([]*entities.CausalNode, bool, error) {
			consequenceBudget = nodes
			return []*entities.CausalNode{{ID: uuid.New()}}, true, nil
		},
	}
	out, err := NewGetCausalChain(repo).Execute(context.Background(), GetCausalChainInput{ID: uuid.New(), IncludeConsequences: true})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if consequenceBudget != valueobjects.MaxCausalNodes-len(chain) {
		t.Fatalf("consequence budget = %d, want %d", consequenceBudget, valueobjects.MaxCausalNodes-len(chain))
	}
	if !out.Truncated {
		t.Fatal("Truncated = false, want true from consequence traversal")
	}

	repo.getConsequencesFunc = func(context.Context, uuid.UUID, int, int) ([]*entities.CausalNode, bool, error) {
		return nil, false, errors.New("consequence query failed")
	}
	if _, err := NewGetCausalChain(repo).Execute(context.Background(), GetCausalChainInput{ID: uuid.New(), IncludeConsequences: true}); err == nil {
		t.Fatal("Execute() swallowed consequence repository error")
	}
}

// Ensure interface is implemented
var _ ports.CausalGraphRepository = (*mockCausalGraphRepository)(nil)
