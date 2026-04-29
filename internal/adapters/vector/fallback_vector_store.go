package vector

import (
	"context"
	"strings"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// fallbackVectorStore wraps a primary VectorStore and transparently falls back
// to a secondary store when the primary is unavailable (e.g. HNSW not ready).
type fallbackVectorStore struct {
	primary  ports.VectorStore
	fallback ports.VectorStore
}

// NewFallbackVectorStore creates a new fallback-aware vector store.
func NewFallbackVectorStore(primary, fallback ports.VectorStore) *fallbackVectorStore {
	return &fallbackVectorStore{
		primary:  primary,
		fallback: fallback,
	}
}

// Search tries the primary store first and falls back if it reports "not ready".
func (f *fallbackVectorStore) Search(ctx context.Context, vector []float32, limit int, wing, room *string) ([]*entities.Candidate, error) {
	res, err := f.primary.Search(ctx, vector, limit, wing, room)
	if err != nil && f.fallback != nil && isNotReady(err) {
		return f.fallback.Search(ctx, vector, limit, wing, room)
	}
	return res, err
}

// SearchLexical tries the primary store first and falls back if it reports "not ready".
func (f *fallbackVectorStore) SearchLexical(ctx context.Context, query string, limit int, wing, room *string) ([]*entities.Candidate, error) {
	res, err := f.primary.SearchLexical(ctx, query, limit, wing, room)
	if err != nil && f.fallback != nil && isNotReady(err) {
		return f.fallback.SearchLexical(ctx, query, limit, wing, room)
	}
	return res, err
}

// AddCandidate delegates to the primary store.
func (f *fallbackVectorStore) AddCandidate(ctx context.Context, candidate *entities.Candidate) error {
	return f.primary.AddCandidate(ctx, candidate)
}

// Delete delegates to the primary store.
func (f *fallbackVectorStore) Delete(ctx context.Context, id uuid.UUID) error {
	return f.primary.Delete(ctx, id)
}

// ClearAll delegates to the primary store.
func (f *fallbackVectorStore) ClearAll(ctx context.Context) error {
	return f.primary.ClearAll(ctx)
}

// ClearByRoom delegates to the primary store.
func (f *fallbackVectorStore) ClearByRoom(ctx context.Context, wing string, room *string) error {
	return f.primary.ClearByRoom(ctx, wing, room)
}

// SearchExact performs an exact content match via the fallback store.
// It uses lexical search with a generous limit and filters for verbatim equality.
func (f *fallbackVectorStore) SearchExact(ctx context.Context, query string, limit int, wing, room *string) ([]*entities.Candidate, error) {
	if f.fallback == nil {
		return nil, nil
	}
	candidates, err := f.fallback.SearchLexical(ctx, query, limit*5, wing, room)
	if err != nil {
		return nil, err
	}
	var exact []*entities.Candidate
	trimmedQuery := strings.TrimSpace(query)
	for _, c := range candidates {
		if c.Verbatim != nil && strings.EqualFold(strings.TrimSpace(c.Verbatim.Content), trimmedQuery) {
			exact = append(exact, c)
			if len(exact) >= limit {
				break
			}
		}
	}
	return exact, nil
}

// isNotReady heuristically detects "index not ready" errors from HNSW.
func isNotReady(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not ready") || strings.Contains(msg, "not supported")
}

var _ ports.VectorStore = (*fallbackVectorStore)(nil)
