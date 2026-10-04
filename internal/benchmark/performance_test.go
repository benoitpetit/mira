package benchmark

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/benoitpetit/mira/internal/adapters/storage"
)

func TestSummarizeLatency(t *testing.T) {
	tests := []struct {
		name    string
		samples []float64
		want    LatencySummary
	}{
		{"empty", nil, LatencySummary{}},
		{"single", []float64{7}, LatencySummary{P50: 7, P95: 7, P99: 7, Median: 7, Spread: 0}},
		{"known samples", []float64{5, 1, 4, 2, 3}, LatencySummary{P50: 3, P95: 4.8, P99: 4.96, Median: 3, Spread: 4}},
		{"percentile boundaries", []float64{0, 10}, LatencySummary{P50: 5, P95: 9.5, P99: 9.9, Median: 5, Spread: 10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarizeLatency(tt.samples)
			if got != tt.want {
				t.Fatalf("summary = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestBuildVectorFixtureAndSearchReadiness(t *testing.T) {
	repo, err := storage.NewSQLiteRepository(filepath.Join(t.TempDir(), "fixture.db"), storage.DefaultSQLiteOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := buildVectorFixture(context.Background(), repo, 100, 32, 42, "test-model-hash"); err != nil {
		t.Fatal(err)
	}
	embeddings, err := repo.GetAllEmbeddings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(embeddings) != 100 {
		t.Fatalf("fixture has %d embeddings, want 100", len(embeddings))
	}
	seen := map[string]bool{}
	for _, embedding := range embeddings {
		if len(embedding.Vector) != 32 {
			t.Fatalf("embedding dimension = %d, want 32", len(embedding.Vector))
		}
		id := embedding.ID.String()
		if seen[id] {
			t.Fatalf("duplicate fixture ID %q", id)
		}
		seen[id] = true
	}
}

func TestPerformanceConfigRejectsInvalidProfile(t *testing.T) {
	if _, err := RunPerformanceTrack(context.Background(), PerformanceConfig{CorpusSizes: []int{0}, Repetitions: 1}); err == nil {
		t.Fatal("zero corpus size accepted")
	}
	if _, err := RunPerformanceTrack(context.Background(), PerformanceConfig{CorpusSizes: []int{10}, Repetitions: 1, Concurrency: 2}); err == nil {
		t.Fatal("unsupported concurrency accepted")
	}
}

func TestRunPerformanceTrackSmoke(t *testing.T) {
	track, err := RunPerformanceTrack(context.Background(), PerformanceConfig{CorpusSizes: []int{100}, Dimension: 32, Seed: 42, Warmups: 1, Repetitions: 1, Backend: "sqlite", Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	if track.Status != TrackAvailable || track.Backend != "sqlite" || track.Concurrency != 1 {
		t.Fatalf("track provenance = %+v", track)
	}
	byName := map[string]LatencyCase{}
	for _, c := range track.Cases {
		byName[c.Name] = c
	}
	for _, name := range []string{"sqlite_fixture_setup_100", "hnsw_rebuild_100", "hnsw_warm_search_100", "sql_fallback_search_100", "sqlite_full_recall_100"} {
		if _, ok := byName[name]; !ok {
			t.Errorf("missing case %s", name)
		}
	}
	for _, name := range []string{"hnsw_rebuild_100", "hnsw_warm_search_100", "sql_fallback_search_100"} {
		c := byName[name]
		if c.Status == TrackAvailable && (len(c.Samples) != 1 || c.Samples[0] < 0) {
			t.Errorf("invalid %s samples: %+v", name, c)
		}
	}
	if byName["sqlite_full_recall_100"].Status != TrackUnavailable {
		t.Errorf("full recall should explain missing pinned model: %+v", byName["sqlite_full_recall_100"])
	}
}

func TestRunPerformanceTrackPinnedModel(t *testing.T) {
	modelDir := os.Getenv("MIRA_BENCH_MODEL_DIR")
	if modelDir == "" {
		t.Skip("set MIRA_BENCH_MODEL_DIR to exercise production full-recall performance")
	}
	lockFile, err := os.ReadFile(filepath.Join("..", "..", "benchmarks", "models", "all-MiniLM-L6-v2.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	var lock EmbeddingModelLock
	if err := json.Unmarshal(lockFile, &lock); err != nil {
		t.Fatal(err)
	}
	track, err := RunPerformanceTrack(context.Background(), PerformanceConfig{CorpusSizes: []int{100, 1000}, Dimension: 384, Seed: 42, Warmups: 1, Repetitions: 1, Concurrency: 1, Backend: "sqlite", ModelDir: modelDir, ModelLock: lock})
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{100, 1000} {
		found := false
		for _, c := range track.Cases {
			if c.Name == fmt.Sprintf("sqlite_full_recall_%d", size) {
				found = true
				if c.Status != TrackAvailable || len(c.Samples) != 1 {
					t.Fatalf("invalid full recall output: %+v", c)
				}
			}
		}
		if !found {
			t.Errorf("missing full recall case at size %d", size)
		}
	}
}
