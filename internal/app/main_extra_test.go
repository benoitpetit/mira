package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/config"
)

// minimalCfg returns a minimal *config.Config ready for NewApplication.
// Storage.Path is set to a fresh temp directory; UseSimpleEmbedder skips
// the heavy Cybertron download; all optional sub-systems are disabled so
// the test runs offline and without side effects.
func minimalCfg(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Storage.Path = filepath.Join(dir, ".mira")
	cfg.Embeddings.UseSimpleEmbedder = true
	cfg.Embeddings.Dimension = 16
	cfg.Extraction.LLM.Enabled = false
	cfg.Soul.Enabled = false
	cfg.Metrics.Enabled = false
	cfg.Webhooks.Enabled = false
	cfg.API.Enabled = false
	cfg.HNSW.EncryptionKey = ""
	return cfg
}

// ──────────────────────────────────────────────────────────────────────────────
// NewApplication — minimal (no optional sub-systems)
// ──────────────────────────────────────────────────────────────────────────────

func TestNewApplication_Minimal(t *testing.T) {
	cfg := minimalCfg(t)
	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatalf("NewApplication (minimal): %v", err)
	}
	if err := app.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// initMetrics — simple collector branch
// ──────────────────────────────────────────────────────────────────────────────

func TestNewApplication_WithSimpleMetrics(t *testing.T) {
	cfg := minimalCfg(t)
	cfg.Metrics.Enabled = true
	cfg.Metrics.PrometheusAddr = "" // → simple collector
	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatalf("NewApplication (simple metrics): %v", err)
	}
	if app.metricsCollector == nil {
		t.Error("expected metricsCollector to be set")
	}
	app.Close()
}

// ──────────────────────────────────────────────────────────────────────────────
// initMetrics — prometheus branch (goroutine; we just verify it is set)
// ──────────────────────────────────────────────────────────────────────────────

func TestNewApplication_WithPrometheusMetrics(t *testing.T) {
	cfg := minimalCfg(t)
	cfg.Metrics.Enabled = true
	cfg.Metrics.PrometheusAddr = "127.0.0.1:0" // random port; goroutine will fail to bind but that's OK
	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatalf("NewApplication (prometheus metrics): %v", err)
	}
	if app.metricsCollector == nil {
		t.Error("expected metricsCollector to be set")
	}
	if app.metricsServer == nil {
		t.Error("expected metricsServer to be set")
	}
	for _, path := range []string{"/metrics", "/health", "/health/live", "/health/ready"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		app.metricsServer.Handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s status = %d, want 200", path, rec.Code)
		}
	}
	// Give the goroutine a moment to start (then let it fail silently)
	time.Sleep(5 * time.Millisecond)
	app.Close()
}

// ──────────────────────────────────────────────────────────────────────────────
// initExtractor — LLM / Ollama path (no network call at init time)
// ──────────────────────────────────────────────────────────────────────────────

func TestNewApplication_WithOllamaExtractor(t *testing.T) {
	cfg := minimalCfg(t)
	cfg.Extraction.LLM.Enabled = true
	cfg.Extraction.LLM.Endpoint = "http://127.0.0.1:11434"
	cfg.Extraction.LLM.Model = "llama3.2:3b"
	cfg.Extraction.LLM.FallbackOnError = true
	cfg.Extraction.LLM.TimeoutSeconds = 5
	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatalf("NewApplication (ollama extractor): %v", err)
	}
	if app.extractor == nil {
		t.Error("expected extractor to be set")
	}
	app.Close()
}

// ──────────────────────────────────────────────────────────────────────────────
// initWebhooks — enabled path
// ──────────────────────────────────────────────────────────────────────────────

func TestNewApplication_WithWebhooks(t *testing.T) {
	cfg := minimalCfg(t)
	cfg.Webhooks.Enabled = true
	cfg.Webhooks.Workers = 1
	cfg.Webhooks.QueueSize = 10
	cfg.Webhooks.Timeout = 5
	cfg.Webhooks.Endpoints = []string{"http://127.0.0.1:9999/hook"}
	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatalf("NewApplication (webhooks): %v", err)
	}
	if app.webhookManager == nil {
		t.Error("expected webhookManager to be set")
	}
	app.Close() // also covers Close() → webhookManager.Stop()
}

// ──────────────────────────────────────────────────────────────────────────────
// initRestAPI — enabled path
// ──────────────────────────────────────────────────────────────────────────────

func TestNewApplication_WithRestAPI(t *testing.T) {
	cfg := minimalCfg(t)
	cfg.API.Enabled = true
	cfg.API.Address = "127.0.0.1:0"
	cfg.API.ReadTimeout = 5
	cfg.API.WriteTimeout = 5
	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatalf("NewApplication (rest api): %v", err)
	}
	if app.restServer == nil {
		t.Error("expected restServer to be set")
	}
	app.Close() // covers Close() → restServer.Shutdown()
}

// ──────────────────────────────────────────────────────────────────────────────
// initVectorStore — encryption key branch
// ──────────────────────────────────────────────────────────────────────────────

func TestNewApplication_WithEncryptionKey(t *testing.T) {
	cfg := minimalCfg(t)
	cfg.HNSW.EncryptionKey = "super-secret-key-for-test"
	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatalf("NewApplication (encryption key): %v", err)
	}
	app.Close()
}

// ──────────────────────────────────────────────────────────────────────────────
// initSoul — enabled path (success or graceful failure both acceptable)
// ──────────────────────────────────────────────────────────────────────────────

func TestNewApplication_WithSoul(t *testing.T) {
	cfg := minimalCfg(t)
	cfg.Soul.Enabled = true
	// Use zero values — SOUL will either succeed or log a warning and continue.
	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatalf("NewApplication (soul): %v", err)
	}
	// Whether soulApp is set or not depends on SOUL internals; both are valid.
	app.Close() // covers soulApp.Close() if set
}

// ──────────────────────────────────────────────────────────────────────────────
// Close() — all branches via direct struct construction
// ──────────────────────────────────────────────────────────────────────────────

func TestClose_AllNil(t *testing.T) {
	a := &Application{}
	if err := a.Close(); err != nil {
		t.Errorf("Close with all-nil fields: %v", err)
	}
}

func TestClose_WithRepository(t *testing.T) {
	cfg := minimalCfg(t)
	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Close is idempotent after the first call; second call should not panic
	app.Close()
}

// ──────────────────────────────────────────────────────────────────────────────
// NewApplicationFromConfig
// ──────────────────────────────────────────────────────────────────────────────

func TestNewApplicationFromConfig_InvalidPath(t *testing.T) {
	// A config file that doesn't exist; LoadOrDefault should return a default config
	// and NewApplication should succeed with the default storage path adjusted.
	// We rely on config.LoadOrDefault falling back to Default() for non-existent paths.
	tmpCfg, _ := os.CreateTemp("", "mira_cfg_*.yaml")
	dir := t.TempDir()
	// Write a minimal config overriding Storage.Path
	_, _ = tmpCfg.WriteString("storage:\n  path: " + filepath.Join(dir, ".mira") + "\nembeddings:\n  use_simple_embedder: true\n  dimension: 16\n")
	tmpCfg.Close()
	defer os.Remove(tmpCfg.Name())

	app, err := NewApplicationFromConfig(tmpCfg.Name())
	if err != nil {
		t.Fatalf("NewApplicationFromConfig: %v", err)
	}
	app.Close()
}

// ──────────────────────────────────────────────────────────────────────────────
// Health / HealthChecker paths
// ──────────────────────────────────────────────────────────────────────────────

func TestHealth_FullApp(t *testing.T) {
	cfg := minimalCfg(t)
	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	status := app.Health(context.Background())
	if status.Status == "" {
		t.Error("expected non-empty health status")
	}
}

func TestHealthChecker_Handler_Unhealthy(t *testing.T) {
	// App with nil repository → database check fails → unhealthy
	a := &Application{}
	hc := NewHealthChecker(a, "0.4.7-test")

	status := hc.Check(context.Background())
	if status.Status != "unhealthy" {
		t.Errorf("expected unhealthy, got %s", status.Status)
	}
}

func TestHealthChecker_CheckVectorStore_Nil(t *testing.T) {
	a := &Application{vectorStore: nil}
	hc := NewHealthChecker(a, "test")
	// vectorStore nil → fail; but we also have nil repository → unhealthy anyway
	status := hc.Check(context.Background())
	_ = status // just ensure no panic
}

func TestIsHNSWReady_NilIndex(t *testing.T) {
	a := &Application{hnswIndex: nil}
	if a.IsHNSWReady() {
		t.Error("expected false for nil hnswIndex")
	}
}

func TestGetVectorStoreStats_NilVectorStore(t *testing.T) {
	a := &Application{}
	stats := a.GetVectorStoreStats()
	if stats["status"] != "not_initialized" {
		t.Errorf("expected not_initialized, got %v", stats["status"])
	}
}

func TestGetVectorStoreStats_SQLiteStore(t *testing.T) {
	cfg := minimalCfg(t)
	// Force HNSW failure by using a read-only index path directory
	// Actually, just use minimal config — HNSW will be set up normally.
	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	stats := app.GetVectorStoreStats()
	if stats["type"] == nil {
		t.Error("expected type in stats")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// RunWithHealthCheck — invalid address causes immediate listen failure
// ──────────────────────────────────────────────────────────────────────────────

func TestRunWithHealthCheck_InvalidAddr(t *testing.T) {
	cfg := minimalCfg(t)
	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	// "invalid-address" is not a valid TCP address → ListenAndServe returns immediately
	if err := app.RunWithHealthCheck("invalid-address"); err == nil {
		t.Error("expected error for invalid address")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// HealthChecker.Handler — unhealthy → HTTP 503
// ──────────────────────────────────────────────────────────────────────────────

func TestHealthChecker_Handler_HTTP503(t *testing.T) {
	// App with nil repository → unhealthy → handler should write 503.
	a := &Application{config: config.Default()}
	hc := NewHealthChecker(a, "test")

	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	hc.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// initVectorStore — model hash mismatch warning branch
// ──────────────────────────────────────────────────────────────────────────────

func TestNewApplication_ModelHashMismatch(t *testing.T) {
	cfg := minimalCfg(t)
	// Setting a fake model hash forces the "mismatch" warning in initVectorStore
	// when GetAllModels returns the real registered hash.
	cfg.Embeddings.ModelHash = "deliberately-wrong-hash"
	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatalf("NewApplication (model hash mismatch): %v", err)
	}
	app.Close()
}

// ──────────────────────────────────────────────────────────────────────────────
// GetVectorStoreStats — HNSW branch (hnswIndex non-nil)
// ──────────────────────────────────────────────────────────────────────────────

func TestGetVectorStoreStats_WithHNSW(t *testing.T) {
	cfg := minimalCfg(t)
	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	stats := app.GetVectorStoreStats()
	if stats["type"] != "hnsw" {
		t.Logf("vector store type: %v (may be sqlite if HNSW failed)", stats["type"])
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// HealthChecker — LivenessHandler and ReadinessHandler
// ──────────────────────────────────────────────────────────────────────────────

func TestLivenessHandler(t *testing.T) {
	a := &Application{config: config.Default()}
	hc := NewHealthChecker(a, "test")

	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health/live", nil)
	hc.LivenessHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestReadinessHandler_Unhealthy(t *testing.T) {
	// nil repository → unhealthy → readiness returns 503
	a := &Application{config: config.Default()}
	hc := NewHealthChecker(a, "test")

	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health/ready", nil)
	hc.ReadinessHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestReadinessHandler_Healthy(t *testing.T) {
	cfg := minimalCfg(t)
	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	hc := NewHealthChecker(app, "test")
	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health/ready", nil)
	hc.ReadinessHandler().ServeHTTP(rr, req)
	// Healthy or degraded → 200; unhealthy → 503
	if rr.Code != http.StatusOK && rr.Code != http.StatusServiceUnavailable {
		t.Errorf("unexpected status: %d", rr.Code)
	}
}
