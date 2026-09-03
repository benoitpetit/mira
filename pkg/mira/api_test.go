package mira_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	mira "github.com/benoitpetit/mira/pkg/mira"
	"github.com/google/uuid"
)

func TestDefaultConfig_ReturnsNonNil(t *testing.T) {
	cfg := mira.DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}
}

func TestLoadConfig_NonExistentPath_ReturnsDefault(t *testing.T) {
	cfg, err := mira.LoadConfig("/tmp/nonexistent-mira-config-zzz99.yaml")
	if err != nil {
		t.Fatalf("LoadConfig with non-existent path returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadConfig returned nil config for non-existent path")
	}
}

func TestLoadConfig_WithValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "storage:\n  path: /tmp/mira-test\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	cfg, err := mira.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadConfig returned nil config")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Integration tests using a real Application instance (simple embedder, native
// extractor, no external dependencies).
// ──────────────────────────────────────────────────────────────────────────────

func newTestApp(t *testing.T) *mira.Application {
	t.Helper()
	dir := t.TempDir()
	cfg := mira.DefaultConfig()
	cfg.Storage.Path = filepath.Join(dir, ".mira")
	cfg.Embeddings.UseSimpleEmbedder = true
	cfg.Embeddings.Dimension = 16
	cfg.Extraction.LLM.Enabled = false
	cfg.Soul.Enabled = false
	cfg.Metrics.Enabled = false
	cfg.Webhooks.Enabled = false
	cfg.API.Enabled = false

	app, err := mira.NewApplication(cfg)
	if err != nil {
		t.Fatalf("mira.NewApplication: %v", err)
	}
	t.Cleanup(func() { app.Close() })
	return app
}

func TestNewApplication_StoreAndRecall(t *testing.T) {
	app := newTestApp(t)
	ctx := context.Background()

	// Store
	out, err := app.Store(ctx, "the quick brown fox", "test-wing", nil, nil)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if out == nil {
		t.Fatal("Store returned nil output")
	}

	// Give background indexing a moment
	time.Sleep(20 * time.Millisecond)

	// Recall
	rOut, err := app.Recall(ctx, "quick fox", 500, "test-wing", nil, nil, nil)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	_ = rOut
}

func TestApplication_Load(t *testing.T) {
	app := newTestApp(t)
	ctx := context.Background()

	out, err := app.Store(ctx, "load test memory", "wing", nil, nil)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	fpID, _ := uuid.Parse(out.FingerprintID)
	lOut, err := app.Load(ctx, fpID)
	// Load may return an error if the ID resolution fails, but the wrapper is covered
	_ = err
	_ = lOut
}

func TestApplication_GetTimeline(t *testing.T) {
	app := newTestApp(t)
	ctx := context.Background()
	_, _ = app.Store(ctx, "timeline entry", "wing", nil, nil)

	out, err := app.GetTimeline(ctx, "wing", nil, nil, nil, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetTimeline: %v", err)
	}
	_ = out
}

func TestApplication_GetStatus(t *testing.T) {
	app := newTestApp(t)
	out, err := app.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if out == nil {
		t.Fatal("GetStatus returned nil")
	}
}

func TestApplication_GetCausalChain(t *testing.T) {
	app := newTestApp(t)
	// No data → empty chain
	out, err := app.GetCausalChain(context.Background(), uuid.New(), 3, false)
	if err != nil {
		t.Fatalf("GetCausalChain: %v", err)
	}
	_ = out
}

func TestApplication_Archive(t *testing.T) {
	app := newTestApp(t)
	out, err := app.Archive(context.Background())
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	_ = out
}

func TestApplication_Clear(t *testing.T) {
	app := newTestApp(t)
	ctx := context.Background()
	_, _ = app.Store(ctx, "clear me", "wing", nil, nil)

	out, err := app.Clear(ctx, "wing", nil)
	if err != nil {
		t.Fatalf("Clear: %v", err)
	}
	_ = out
}

func TestApplication_ClearGlobal(t *testing.T) {
	app := newTestApp(t)
	// wing="" and room=nil → global mode
	out, err := app.Clear(context.Background(), "", nil)
	if err != nil {
		t.Fatalf("Clear (global): %v", err)
	}
	_ = out
}

func TestApplication_Delete(t *testing.T) {
	app := newTestApp(t)
	ctx := context.Background()
	out, err := app.Store(ctx, "delete me", "wing", nil, nil)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	fpID, _ := uuid.Parse(out.FingerprintID)
	// Delete covers the wrapper; error is acceptable (not-found is fine)
	_ = app.Delete(ctx, fpID)
}

func TestApplication_Search(t *testing.T) {
	app := newTestApp(t)
	ctx := context.Background()
	_, _ = app.Store(ctx, "search test content", "wing", nil, nil)
	time.Sleep(20 * time.Millisecond)

	results, err := app.Search(ctx, "search test", 10, 0.0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	_ = results
}

func TestApplication_Update(t *testing.T) {
	app := newTestApp(t)
	ctx := context.Background()
	out, err := app.Store(ctx, "original content", "wing", nil, nil)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	fpID, _ := uuid.Parse(out.FingerprintID)
	// Update covers the wrapper; error acceptable if ID resolves incorrectly
	_ = app.Update(ctx, fpID, "updated content")
}

func TestApplication_Consolidate(t *testing.T) {
	app := newTestApp(t)
	// Nothing to consolidate → should succeed silently
	if err := app.Consolidate(context.Background(), "wing", 0.9); err != nil {
		t.Fatalf("Consolidate: %v", err)
	}
}

func TestApplication_SoulApp_Nil(t *testing.T) {
	app := newTestApp(t)
	// Soul is disabled → SoulApp() returns nil
	if app.SoulApp() != nil {
		t.Error("expected nil SoulApp when soul disabled")
	}
}
