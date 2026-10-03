// GetCausalChain use case
package interactors

import (
	"context"
	"fmt"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// GetCausalChainInput contains the input for getting causal chain
type GetCausalChainInput struct {
	ID                  uuid.UUID
	MaxDepth            int
	IncludeConsequences bool
}

// GetCausalChainOutput contains the output of getting causal chain
type GetCausalChainOutput struct {
	Chain        []*entities.CausalNode `json:"chain"`
	Consequences []*entities.CausalNode `json:"consequences,omitempty"`
	Truncated    bool                   `json:"truncated"`
}

// GetCausalChain implements the get causal chain use case
type GetCausalChain struct {
	causalRepo ports.CausalGraphRepository
}

// NewGetCausalChain creates a new get causal chain interactor
func NewGetCausalChain(causalRepo ports.CausalGraphRepository) *GetCausalChain {
	return &GetCausalChain{
		causalRepo: causalRepo,
	}
}

// Execute retrieves the causal chain
func (uc *GetCausalChain) Execute(ctx context.Context, input GetCausalChainInput) (*GetCausalChainOutput, error) {
	maxDepth := input.MaxDepth
	if maxDepth == 0 {
		maxDepth = valueobjects.DefaultCausalMaxDepth
	}
	if maxDepth < 1 || maxDepth > valueobjects.MaxCausalDepth {
		return nil, fmt.Errorf("max_depth must be between 1 and %d", valueobjects.MaxCausalDepth)
	}

	chain, truncated, err := uc.causalRepo.GetChain(ctx, input.ID, maxDepth, valueobjects.MaxCausalNodes)
	if err != nil {
		return nil, fmt.Errorf("failed to get causal chain: %w", err)
	}
	if len(chain) > valueobjects.MaxCausalNodes {
		chain = chain[:valueobjects.MaxCausalNodes]
		truncated = true
	}

	output := &GetCausalChainOutput{
		Chain:     chain,
		Truncated: truncated,
	}

	if input.IncludeConsequences && !output.Truncated {
		remaining := valueobjects.MaxCausalNodes - len(chain)
		consequences, consequencesTruncated, err := uc.causalRepo.GetConsequences(ctx, input.ID, maxDepth, remaining)
		if err != nil {
			return nil, fmt.Errorf("failed to get causal consequences: %w", err)
		}
		if len(consequences) > remaining {
			consequences = consequences[:remaining]
			consequencesTruncated = true
		}
		output.Consequences = consequences
		output.Truncated = consequencesTruncated
	}

	return output, nil
}
