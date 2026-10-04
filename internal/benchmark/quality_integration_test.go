package benchmark

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	mira "github.com/benoitpetit/mira/pkg/mira"
)

func TestRunQualityTrack(t *testing.T) {
	modelDir := os.Getenv("MIRA_BENCH_MODEL_DIR")
	if modelDir == "" {
		t.Skip("set MIRA_BENCH_MODEL_DIR to the locked local model directory to run production retrieval integration")
	}
	lockBytes, err := os.ReadFile(filepath.Join("..", "..", "benchmarks", "models", "all-MiniLM-L6-v2.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	var lock EmbeddingModelLock
	if err := json.Unmarshal(lockBytes, &lock); err != nil {
		t.Fatal(err)
	}
	if err := VerifyEmbeddingModel(modelDir, lock); err != nil {
		t.Fatal(err)
	}
	dataset, err := GenerateSyntheticV1(42)
	if err != nil {
		t.Fatal(err)
	}
	storage := t.TempDir()
	cfg := mira.DefaultConfig()
	cfg.Storage.Type = "sqlite"
	cfg.Storage.Path = filepath.Join(storage, ".mira")
	cfg.Embeddings.CurrentModel = lock.ModelID
	cfg.Embeddings.UseSimpleEmbedder = false
	cfg.Embeddings.Dimension = 384
	cfg.Extraction.LLM.Enabled = false
	cfg.Metrics.Enabled = false
	cfg.Webhooks.Enabled = false
	cfg.API.Enabled = false
	modelTarget := filepath.Join(cfg.Storage.Path, "models", lock.ModelID)
	if err := os.MkdirAll(filepath.Dir(modelTarget), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(modelDir, modelTarget); err != nil {
		t.Fatalf("isolate model cache: %v", err)
	}
	app, err := mira.NewApplication(cfg)
	if err != nil {
		t.Fatalf("initialize MIRA with pinned model: %v", err)
	}
	defer app.Close()
	track, err := RunQualityTrack(context.Background(), app, dataset, 1)
	if err != nil {
		t.Fatal(err)
	}
	if track.Status != TrackAvailable || track.FailedQueries != 0 || len(track.Results) != len(dataset.Queries) {
		t.Fatalf("incomplete quality result: %+v", track)
	}
	if _, err := os.Stat(filepath.Join(cfg.Storage.Path, "mira.db")); err != nil {
		t.Fatalf("benchmark did not use isolated database: %v", err)
	}
	if _, err := os.Stat(filepath.Join(storage, "mira.db")); !os.IsNotExist(err) {
		t.Fatalf("unexpected database written outside isolated storage: %v", err)
	}
	for _, result := range track.Results {
		if got, want := result.Metrics, EvaluateRanking(judgmentsFor(dataset, result.QueryID), result.RankedIDs); got != want {
			t.Errorf("%s metrics = %+v, recomputed = %+v", result.QueryID, got, want)
		}
		for _, id := range result.RankedIDs {
			if _, ok := memoryByID(dataset, id); !ok {
				t.Errorf("%s returned unknown stable memory ID %q", result.QueryID, id)
			}
		}
	}
}

func judgmentsFor(d Dataset, queryID string) []Judgment {
	var out []Judgment
	for _, j := range d.Judgments {
		if j.QueryID == queryID {
			out = append(out, j)
		}
	}
	return out
}

func memoryByID(d Dataset, id string) (Memory, bool) {
	for _, m := range d.Memories {
		if m.ID == id {
			return m, true
		}
	}
	return Memory{}, false
}
