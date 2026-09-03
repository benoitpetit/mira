// SearchSemantic use case
package interactors

import (
	"context"
	"fmt"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/benoitpetit/mira/internal/util"
	"github.com/google/uuid"
)

// SearchSemanticInput contains the input for semantic search
type SearchSemanticInput struct {
	Query     string
	TopK      int
	Threshold float64
	Kind      *valueobjects.MemoryKind
}

// SearchSemanticResult represents a single semantic search result
type SearchSemanticResult struct {
	ID            uuid.UUID `json:"id"`
	FingerprintID uuid.UUID `json:"fingerprint_id"`
	Content       string    `json:"content"`
	Similarity    float64   `json:"similarity"`
	Type          string    `json:"type"`
	Kind          string    `json:"kind"`
	Wing          string    `json:"wing"`
	Room          *string   `json:"room,omitempty"`
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

	// The vector store enforces its limit before we apply the business-role
	// filter. Fetch a wider candidate set so `kind=user`, for example, still
	// returns up to TopK user memories even when project memories rank first.
	searchLimit := input.TopK
	if input.Kind != nil {
		searchLimit *= 5
	}
	candidates, err := uc.vectorStore.Search(ctx, vector, searchLimit, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	var results []*SearchSemanticResult
	for _, c := range candidates {
		if input.Kind != nil && c.Verbatim.Kind != *input.Kind {
			continue
		}
		sim := util.CosineSimilarity(vector, c.Embedding)
		if sim >= input.Threshold {
			results = append(results, &SearchSemanticResult{
				ID:            c.Verbatim.ID,
				FingerprintID: c.Memory.ID,
				Content:       c.Verbatim.Content,
				Similarity:    sim,
				Type:          string(c.Memory.Type),
				Kind:          string(c.Verbatim.Kind),
				Wing:          c.Verbatim.Wing,
				Room:          c.Verbatim.Room,
			})
			if len(results) >= input.TopK {
				break
			}
		}
	}

	return results, nil
}
