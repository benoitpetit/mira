// GetCausalChain use case
package interactors

import (
	"context"
	"fmt"

	"github.com/benoitpetit/mira/internal/domain/entities"
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
	Chain        []*entities.CausalNode
	Consequences []*entities.CausalNode
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
	chain, err := uc.causalRepo.GetChain(ctx, input.ID, input.MaxDepth)
	if err != nil {
		return nil, fmt.Errorf("failed to get causal chain: %w", err)
	}

	output := &GetCausalChainOutput{
		Chain: chain,
	}

	if input.IncludeConsequences {
		consequences, err := uc.causalRepo.GetConsequences(ctx, input.ID, input.MaxDepth)
		if err == nil {
			output.Consequences = consequences
		}
	}

	return output, nil
}
