package vector

import (
	"context"
	"sort"
	"strings"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/benoitpetit/mira/internal/util"
	"github.com/google/uuid"
)

// BruteForceVectorStore is the portable vector fallback used while HNSW is
// unavailable. It deliberately depends on EmbeddingSource instead of SQLite
// SQL, so the same fallback works with SQLite and PostgreSQL repositories.
// It is O(n) and therefore a safety net, not the preferred production index.
type BruteForceVectorStore struct {
	source ports.EmbeddingSource
}

func NewBruteForceVectorStore(source ports.EmbeddingSource) *BruteForceVectorStore {
	return &BruteForceVectorStore{source: source}
}

func (s *BruteForceVectorStore) Search(ctx context.Context, query []float32, limit int, wing, room *string) ([]*entities.Candidate, error) {
	if s == nil || s.source == nil || limit <= 0 {
		return nil, nil
	}
	embeddings, err := s.source.GetAllEmbeddings(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(embeddings))
	for _, embedding := range embeddings {
		if embedding != nil && len(embedding.Vector) > 0 {
			ids = append(ids, embedding.ID)
		}
	}
	candidates, err := s.source.GetCandidatesWithEmbeddings(ctx, ids, wing, room)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return util.CosineSimilarity(candidates[i].Embedding, query) > util.CosineSimilarity(candidates[j].Embedding, query)
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, nil
}

func (s *BruteForceVectorStore) SearchLexical(ctx context.Context, query string, limit int, wing, room *string) ([]*entities.Candidate, error) {
	if s == nil || s.source == nil {
		return nil, nil
	}
	return s.source.SearchLexical(ctx, query, limit, wing, room)
}

func (s *BruteForceVectorStore) SearchExact(ctx context.Context, query string, limit int, wing, room *string) ([]*entities.Candidate, error) {
	candidates, err := s.SearchLexical(ctx, query, limit*5, wing, room)
	if err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	exact := make([]*entities.Candidate, 0, limit)
	for _, candidate := range candidates {
		if candidate != nil && candidate.Verbatim != nil && strings.EqualFold(strings.TrimSpace(candidate.Verbatim.Content), query) {
			exact = append(exact, candidate)
			if len(exact) >= limit {
				break
			}
		}
	}
	return exact, nil
}

func (s *BruteForceVectorStore) AddCandidate(context.Context, *entities.Candidate) error { return nil }
func (s *BruteForceVectorStore) Delete(context.Context, uuid.UUID) error                 { return nil }
func (s *BruteForceVectorStore) ClearAll(context.Context) error                          { return nil }
func (s *BruteForceVectorStore) ClearByRoom(context.Context, string, *string) error      { return nil }

var _ ports.VectorStore = (*BruteForceVectorStore)(nil)
