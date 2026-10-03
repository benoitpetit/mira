package storage

import (
	"context"
	"testing"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

func TestTraverseCausalNodesDeduplicatesCyclesAndDiamonds(t *testing.T) {
	root := uuid.New()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	graph := map[uuid.UUID][]uuid.UUID{
		root: {a, b},
		a:    {c},
		b:    {c},
		c:    {root},
	}

	nodes, truncated, err := traverseCausalNodes(context.Background(), root, 5, 10, false, func(_ context.Context, frontier, visited []uuid.UUID, limit int) ([]*entities.CausalNode, error) {
		seen := make(map[uuid.UUID]struct{}, len(visited))
		for _, id := range visited {
			seen[id] = struct{}{}
		}
		var result []*entities.CausalNode
		for _, parent := range frontier {
			for _, child := range graph[parent] {
				if _, exists := seen[child]; exists {
					continue
				}
				seen[child] = struct{}{}
				result = append(result, &entities.CausalNode{ID: child})
			}
		}
		if len(result) > limit {
			result = result[:limit]
		}
		return result, nil
	})
	if err != nil {
		t.Fatalf("traverseCausalNodes() error = %v", err)
	}
	if truncated {
		t.Fatal("traverseCausalNodes() marked a fully visited graph truncated")
	}
	if len(nodes) != 3 {
		t.Fatalf("got %d unique nodes, want 3", len(nodes))
	}
}

func TestTraverseCausalNodesMarksGlobalNodeCap(t *testing.T) {
	root := uuid.New()
	children := make([]uuid.UUID, 4)
	for i := range children {
		children[i] = uuid.New()
	}
	nodes, truncated, err := traverseCausalNodes(context.Background(), root, 1, 3, false, func(_ context.Context, _, _ []uuid.UUID, limit int) ([]*entities.CausalNode, error) {
		if limit != 4 {
			t.Fatalf("lookup limit = %d, want node budget plus sentinel (4)", limit)
		}
		result := make([]*entities.CausalNode, 0, len(children))
		for _, id := range children {
			result = append(result, &entities.CausalNode{ID: id})
		}
		return result, nil
	})
	if err != nil {
		t.Fatalf("traverseCausalNodes() error = %v", err)
	}
	if len(nodes) != 3 || !truncated {
		t.Fatalf("got %d nodes, truncated=%v; want 3 nodes and truncation", len(nodes), truncated)
	}
}

func TestTraverseCausalNodesExactBudgetIsNotTruncated(t *testing.T) {
	root := uuid.New()
	children := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	nodes, truncated, err := traverseCausalNodes(context.Background(), root, 1, len(children), false, func(_ context.Context, _, _ []uuid.UUID, limit int) ([]*entities.CausalNode, error) {
		if limit != len(children)+1 {
			t.Fatalf("lookup limit = %d, want exact budget plus sentinel (%d)", limit, len(children)+1)
		}
		result := make([]*entities.CausalNode, 0, len(children))
		for _, id := range children {
			result = append(result, &entities.CausalNode{ID: id})
		}
		return result, nil
	})
	if err != nil {
		t.Fatalf("traverseCausalNodes() error = %v", err)
	}
	if len(nodes) != len(children) || truncated {
		t.Fatalf("got %d nodes, truncated=%v; want %d and false", len(nodes), truncated, len(children))
	}
}

func TestTraverseCausalNodesBatchesWideFrontiers(t *testing.T) {
	root := uuid.New()
	children := make([]uuid.UUID, valueobjects.CausalNeighborBatchSize+1)
	for i := range children {
		children[i] = uuid.New()
	}
	maxFrontier := 0
	nodes, truncated, err := traverseCausalNodes(context.Background(), root, 2, valueobjects.MaxCausalNodes, false, func(_ context.Context, frontier, visited []uuid.UUID, limit int) ([]*entities.CausalNode, error) {
		if len(frontier) > maxFrontier {
			maxFrontier = len(frontier)
		}
		if len(frontier) == 1 && frontier[0] == root {
			result := make([]*entities.CausalNode, 0, len(children))
			for _, id := range children {
				result = append(result, &entities.CausalNode{ID: id})
			}
			return result, nil
		}
		if len(frontier) > valueobjects.CausalNeighborBatchSize {
			t.Fatalf("frontier size %d exceeds batch size %d", len(frontier), valueobjects.CausalNeighborBatchSize)
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("traverseCausalNodes() error = %v", err)
	}
	if truncated || len(nodes) != len(children) {
		t.Fatalf("got %d nodes, truncated=%v; want %d and false", len(nodes), truncated, len(children))
	}
	if maxFrontier != valueobjects.CausalNeighborBatchSize {
		t.Fatalf("largest frontier batch = %d, want %d", maxFrontier, valueobjects.CausalNeighborBatchSize)
	}
}
