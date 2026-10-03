package storage

import (
	"context"
	"fmt"
	"sort"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

type causalNeighborLookup func(context.Context, []uuid.UUID, []uuid.UUID, int) ([]*entities.CausalNode, error)

func normalizeCausalQueryLimits(maxDepth, maxNodes int) (int, int, error) {
	if maxDepth == 0 {
		maxDepth = valueobjects.DefaultCausalMaxDepth
	}
	if maxDepth < 1 || maxDepth > valueobjects.MaxCausalDepth {
		return 0, 0, fmt.Errorf("causal max depth must be between 1 and %d", valueobjects.MaxCausalDepth)
	}
	if maxNodes < 0 {
		return 0, 0, fmt.Errorf("causal max nodes cannot be negative")
	}
	if maxNodes > valueobjects.MaxCausalNodes {
		maxNodes = valueobjects.MaxCausalNodes
	}
	return maxDepth, maxNodes, nil
}

// traverseCausalNodes performs a breadth-first traversal with a global visited
// set. Each adapter query is limited to the remaining node budget plus one
// sentinel, so cycles and multiple paths do not multiply result rows.
func traverseCausalNodes(
	ctx context.Context,
	root uuid.UUID,
	maxDepth, maxNodes int,
	ancestors bool,
	lookup causalNeighborLookup,
) ([]*entities.CausalNode, bool, error) {
	maxDepth, maxNodes, err := normalizeCausalQueryLimits(maxDepth, maxNodes)
	if err != nil {
		return nil, false, err
	}

	visited := map[uuid.UUID]struct{}{root: {}}
	visitedIDs := []uuid.UUID{root}
	frontier := []uuid.UUID{root}
	levels := make([][]*entities.CausalNode, 0, maxDepth)
	truncated := false

	for depth := 0; depth < maxDepth && len(frontier) > 0; depth++ {
		next := make([]*entities.CausalNode, 0)
		for start := 0; start < len(frontier); start += valueobjects.CausalNeighborBatchSize {
			end := min(start+valueobjects.CausalNeighborBatchSize, len(frontier))
			remaining := maxNodes - len(visitedIDs) + 1 // visited includes the root
			if remaining < 0 {
				remaining = 0
			}
			neighbors, err := lookup(ctx, frontier[start:end], visitedIDs, remaining+1)
			if err != nil {
				return nil, false, err
			}
			for _, node := range neighbors {
				if node == nil {
					continue
				}
				if _, exists := visited[node.ID]; exists {
					continue
				}
				if len(visitedIDs)-1 >= maxNodes {
					truncated = true
					break
				}
				visited[node.ID] = struct{}{}
				visitedIDs = append(visitedIDs, node.ID)
				next = append(next, node)
			}
			if truncated {
				break
			}
		}
		if len(next) == 0 {
			break
		}
		sort.Slice(next, func(i, j int) bool { return next[i].ID.String() < next[j].ID.String() })
		levels = append(levels, next)
		frontier = make([]uuid.UUID, len(next))
		for i, node := range next {
			frontier[i] = node.ID
		}
		if truncated {
			break
		}
	}

	var nodes []*entities.CausalNode
	if ancestors {
		for i := len(levels) - 1; i >= 0; i-- {
			nodes = append(nodes, levels[i]...)
		}
	} else {
		for _, level := range levels {
			nodes = append(nodes, level...)
		}
	}
	return nodes, truncated, nil
}
