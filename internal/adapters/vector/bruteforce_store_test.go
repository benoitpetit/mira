package vector

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/adapters/storage"
	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

type bruteForceSource struct {
	embeddings []*entities.Embedding
	candidates []*entities.Candidate
}

func (s *bruteForceSource) GetAllEmbeddings(context.Context) ([]*entities.Embedding, error) {
	return s.embeddings, nil
}

func (s *bruteForceSource) GetCandidatesWithEmbeddings(context.Context, []uuid.UUID, *string, *string) ([]*entities.Candidate, error) {
	return s.candidates, nil
}

func (s *bruteForceSource) SearchLexical(context.Context, string, int, *string, *string) ([]*entities.Candidate, error) {
	return s.candidates, nil
}

var _ ports.EmbeddingSource = (*bruteForceSource)(nil)

func TestBruteForceVectorStoreSearchesPortableSource(t *testing.T) {
	near := entities.NewVerbatim("near", "wing", nil)
	far := entities.NewVerbatim("far", "wing", nil)
	nearCandidate := entities.NewCandidate(&entities.Fingerprint{ID: near.ID, VerbatimID: near.ID}, near, []float32{1, 0})
	farCandidate := entities.NewCandidate(&entities.Fingerprint{ID: far.ID, VerbatimID: far.ID}, far, []float32{0, 1})
	source := &bruteForceSource{
		embeddings: []*entities.Embedding{{ID: near.ID, Vector: []float32{1, 0}}, {ID: far.ID, Vector: []float32{0, 1}}},
		candidates: []*entities.Candidate{farCandidate, nearCandidate},
	}
	store := NewBruteForceVectorStore(source)
	results, err := store.Search(context.Background(), []float32{1, 0}, 1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID() != near.ID {
		t.Fatalf("unexpected brute-force result: %+v", results)
	}
}

func TestSQLOverlapCacheWorksWithSQLite(t *testing.T) {
	repo, err := storage.NewSQLiteRepository(filepath.Join(t.TempDir(), "mira.db"), storage.DefaultSQLiteOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	cache := NewSQLOverlapCache(repo.DB(), "sqlite")
	a, b := uuid.New(), uuid.New()
	cache.Set(context.Background(), a, b, 0.42)
	value, ok := cache.Get(context.Background(), b, a)
	if !ok || value != 0.42 {
		t.Fatalf("overlap cache round trip failed: value=%v ok=%v", value, ok)
	}
}

func TestSQLSessionCacheWorksWithSQLite(t *testing.T) {
	repo, err := storage.NewSQLiteRepository(filepath.Join(t.TempDir(), "mira.db"), storage.DefaultSQLiteOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	cache := NewSQLSessionCache(repo.DB(), "sqlite")
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	expiresAt := time.Now().Add(time.Minute)
	if err := cache.Save(context.Background(), "session-1", ids, expiresAt); err != nil {
		t.Fatal(err)
	}
	loaded, err := cache.Load(context.Background(), "session-1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != len(ids) {
		t.Fatalf("session cache returned %d IDs, want %d", len(loaded), len(ids))
	}

	if err := cache.PurgeExpired(context.Background(), expiresAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	loaded, err = cache.Load(context.Background(), "session-1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Fatalf("expired session cache still returned %d IDs", len(loaded))
	}
}
