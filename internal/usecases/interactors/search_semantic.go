// SearchSemantic use case
package interactors

import (
	"context"
	"fmt"

	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/benoitpetit/mira/internal/util"
	"github.com/google/uuid"
)

// SearchSemanticInput contains the input for semantic search
type SearchSemanticInput struct {
	Query     string
	TopK      int
	Threshold float64
}

// SearchSemanticResult represents a single semantic search result
type SearchSemanticResult struct {
	ID         uuid.UUID
	Content    string
	Similarity float64
	Type       string
	Wing       string
	Room       *string
}

// SearchSemantic implements pure vector search (without CBA)
type SearchSemantic struct {
	vectorStore ports.VectorStore
	embedder    ports.Embedder
}

// NewSearchSemantic creates a new semantic search interactor
func NewSearchSemantic(vectorStore ports.VectorStore, embedder ports.Embedder) *SearchSemantic {
	return &SearchSemantic{
		vectorStore: vectorStore,
		embedder:    embedder,
	}
}

// Execute performs a vector search for the query and filters by similarity threshold.
func (uc *SearchSemantic) Execute(ctx context.Context, input SearchSemanticInput) ([]*SearchSemanticResult, error) {
	if input.TopK <= 0 {
		input.TopK = 10
	}

	vector, err := uc.embedder.Encode(ctx, input.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to encode query: %w", err)
	}

	candidates, err := uc.vectorStore.Search(ctx, vector, input.TopK, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	var results []*SearchSemanticResult
	for _, c := range candidates {
		sim := util.CosineSimilarity(vector, c.Embedding)
		if sim >= input.Threshold {
			results = append(results, &SearchSemanticResult{
				ID:         c.Verbatim.ID,
				Content:    c.Verbatim.Content,
				Similarity: sim,
				Type:       string(c.Memory.Type),
				Wing:       c.Verbatim.Wing,
				Room:       c.Verbatim.Room,
			})
		}
	}

	return results, nil
}
