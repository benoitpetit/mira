package storage

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

func TestPostgreSQLIntegrationEmbeddingRoundTrip(t *testing.T) {
	databaseURL := os.Getenv("MIRA_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("set MIRA_POSTGRES_URL to a disposable PostgreSQL test database")
	}

	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse MIRA_POSTGRES_URL: %v", err)
	}
	if !strings.HasSuffix(parsedURL.Path, "_test") {
		t.Fatalf("refusing to run integration test against database %q: database name must end in _test", parsedURL.Path)
	}

	repo, err := NewPostgreSQLRepository(PostgreSQLOptions{URL: databaseURL, MaxConns: 2, MinConns: 1})
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	model := entities.NewEmbeddingModel("mira-postgres-integration", 3)
	if err := repo.RegisterModel(ctx, model); err != nil {
		t.Fatalf("register embedding model: %v", err)
	}

	verbatim := entities.NewVerbatim("PostgreSQL embedding round trip", "postgres-integration", nil)
	if err := repo.StoreVerbatim(ctx, verbatim); err != nil {
		t.Fatalf("store verbatim: %v", err)
	}
	defer func() {
		if err := repo.DeleteVerbatimByID(ctx, verbatim.ID); err != nil {
			t.Errorf("clean up integration verbatim: %v", err)
		}
	}()
	fingerprint := entities.NewFingerprint(verbatim.ID, valueobjects.TypeFact, model.ModelHash)
	if err := repo.StoreFingerprint(ctx, fingerprint); err != nil {
		t.Fatalf("store fingerprint: %v", err)
	}

	want := entities.NewEmbedding(verbatim.ID, model.ModelHash, []float32{0.25, -0.5, 0.75})
	want.Normalized = true
	if err := repo.StoreEmbedding(ctx, want); err != nil {
		t.Fatalf("store embedding: %v", err)
	}

	got, err := repo.GetEmbeddingByID(ctx, verbatim.ID)
	if err != nil {
		t.Fatalf("get embedding: %v", err)
	}
	if got.Normalized != want.Normalized {
		t.Errorf("normalized = %t, want %t", got.Normalized, want.Normalized)
	}
	if got.Dim != want.Dim {
		t.Errorf("dimension = %d, want %d", got.Dim, want.Dim)
	}
	if len(got.Vector) != len(want.Vector) {
		t.Fatalf("vector length = %d, want %d", len(got.Vector), len(want.Vector))
	}
	for i := range want.Vector {
		if got.Vector[i] != want.Vector[i] {
			t.Errorf("vector[%d] = %f, want %f", i, got.Vector[i], want.Vector[i])
		}
	}

	all, err := repo.GetAllEmbeddings(ctx)
	if err != nil {
		t.Fatalf("list embeddings for HNSW rebuild: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("embeddings returned for HNSW rebuild = %d, want 1", len(all))
	}
	if len(all[0].Vector) != len(want.Vector) {
		t.Fatalf("rebuild vector length = %d, want %d", len(all[0].Vector), len(want.Vector))
	}

	candidates, err := repo.GetCandidatesWithEmbeddings(ctx, []uuid.UUID{verbatim.ID}, nil, nil)
	if err != nil {
		t.Fatalf("hydrate PostgreSQL candidate for recall: %v", err)
	}
	if len(candidates) != 1 || len(candidates[0].Embedding) != len(want.Vector) {
		t.Fatalf("hydrated candidates = %+v, want one 3-dimensional candidate", candidates)
	}
}
