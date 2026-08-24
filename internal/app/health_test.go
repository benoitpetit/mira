package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/config"
)

// createTestApp creates a test application for health check tests
func createTestApp(t *testing.T) (*Application, func()) {
	tempDir, err := os.MkdirTemp("", "mira_health_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	cfg := &config.Config{
		System: config.SystemConfig{
			Version: "0.4.0",
		},
		Storage: config.StorageConfig{
			Path: tempDir,
		},
		Embeddings: config.EmbeddingsConfig{
			CurrentModel:      "sentence-transformers/all-MiniLM-L6-v2",
			Dimension:         384,
			BatchSize:         32,
			CacheSize:         1000,
			UseSimpleEmbedder: true, // Use SimpleEmbedder to avoid race conditions in tests
		},
		Allocator: config.AllocatorConfig{
			DefaultBudget:         4000,
			MaxCandidates:         100,
			EarlyPruningThreshold: 0.6,
			SessionWindowSeconds:  7200,
			SessionBoostBeta:      0.2,
			CausalPenaltyAlpha:    0.15,
			DensitySigmoid: config.DensitySigmoidConfig{
				K:  2.0,
				Mu: 0.3,
			},
		},
		ArchiveThresholds: map[string]float64{
			"session_note": 30,
			"debug_log":    7,
		},
		Extraction: config.ExtractionConfig{
			MinEntityLength: 2,
		},
		MCP: config.MCPConfig{
			Name:      "mira",
			Version:   "0.4.0",
			Transport: "stdio",
		},
		HNSW: config.HNSWConfig{
			M:              16,
			Ml:             0.25,
			EfConstruction: 200,
			EfSearch:       50,
		},
		Metrics: config.MetricsConfig{
			Enabled:        false,
			PrometheusAddr: ":9090",
			ReportInterval: 60,
		},
		Webhooks: config.WebhooksConfig{
			Enabled:   false,
			Workers:   3,
			QueueSize: 1000,
			Timeout:   30,
		},
	}

	app, err := NewApplication(cfg)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("NewApplication failed: %v", err)
	}

	cleanup := func() {
		app.Close()
		os.RemoveAll(tempDir)
	}

	return app, cleanup
}

func TestHealthChecker_Check(t *testing.T) {
	app, cleanup := createTestApp(t)
	defer cleanup()

	checker := NewHealthChecker(app, "1.0.0")
	status := checker.Check(context.Background())

	// Should be healthy or degraded (HNSW might not be ready yet)
	if status.Status != "healthy" && status.Status != "degraded" {
		t.Errorf("Expected healthy or degraded, got %s", status.Status)
	}

	if status.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", status.Version)
	}

	if status.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}

	// Should have all checks
	if _, ok := status.Checks["database"]; !ok {
		t.Error("Database check should be present")
	}
	if _, ok := status.Checks["vector_store"]; !ok {
		t.Error("Vector store check should be present")
	}
	if _, ok := status.Checks["embedder"]; !ok {
		t.Error("Embedder check should be present")
	}
}

func TestHealthChecker_Check_UninitializedRepository(t *testing.T) {
	// Create an app with minimal setup
	app := &Application{
		config: &config.Config{
			MCP: config.MCPConfig{
				Version: "0.4.0",
			},
		},
	}

	checker := NewHealthChecker(app, "1.0.0")
	status := checker.Check(context.Background())

	if status.Status != "unhealthy" {
		t.Errorf("Expected unhealthy when repository is nil, got %s", status.Status)
	}

	if status.Checks["database"].Status != "fail" {
		t.Error("Database check should fail")
	}
}

func TestHealthChecker_Handler(t *testing.T) {
	app, cleanup := createTestApp(t)
	defer cleanup()

	checker := NewHealthChecker(app, "1.0.0")

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	checker.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	var status HealthStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if status.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", status.Version)
	}
}

func TestHealthChecker_LivenessHandler(t *testing.T) {
	app, cleanup := createTestApp(t)
	defer cleanup()

	checker := NewHealthChecker(app, "1.0.0")

	req := httptest.NewRequest("GET", "/health/live", nil)
	rec := httptest.NewRecorder()

	checker.LivenessHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["status"] != "alive" {
		t.Errorf("Expected status 'alive', got %s", response["status"])
	}
}

func TestHealthChecker_ReadinessHandler(t *testing.T) {
	app, cleanup := createTestApp(t)
	defer cleanup()

	checker := NewHealthChecker(app, "1.0.0")

	req := httptest.NewRequest("GET", "/health/ready", nil)
	rec := httptest.NewRecorder()

	checker.ReadinessHandler().ServeHTTP(rec, req)

	// Should return 200 for healthy or degraded
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if _, ok := response["ready"]; !ok {
		t.Error("Response should contain 'ready' field")
	}

	if _, ok := response["checks"]; !ok {
		t.Error("Response should contain 'checks' field")
	}
}

func TestHealthChecker_ReadinessHandler_Unhealthy(t *testing.T) {
	// Create an app with nil dependencies
	app := &Application{
		config: &config.Config{
			MCP: config.MCPConfig{
				Version: "0.4.0",
			},
		},
	}

	checker := NewHealthChecker(app, "1.0.0")

	req := httptest.NewRequest("GET", "/health/ready", nil)
	rec := httptest.NewRecorder()

	checker.ReadinessHandler().ServeHTTP(rec, req)

	// Should return 503 for unhealthy
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", rec.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if ready, ok := response["ready"].(bool); !ok || ready {
		t.Error("Expected ready to be false")
	}
}

func TestApplication_Health(t *testing.T) {
	app, cleanup := createTestApp(t)
	defer cleanup()

	status := app.Health(context.Background())

	// Should be healthy or degraded
	if status.Status != "healthy" && status.Status != "degraded" {
		t.Errorf("Expected healthy or degraded, got %s", status.Status)
	}

	if status.Version != "0.4.0" {
		t.Errorf("Expected version 0.4.0, got %s", status.Version)
	}
}

func TestApplication_IsHNSWReady(t *testing.T) {
	app, cleanup := createTestApp(t)
	defer cleanup()

	// Give some time for HNSW to potentially initialize
	time.Sleep(100 * time.Millisecond)

	// This may be true or false depending on initialization timing
	_ = app.IsHNSWReady()
}

func TestApplication_GetVectorStoreStats(t *testing.T) {
	app, cleanup := createTestApp(t)
	defer cleanup()

	stats := app.GetVectorStoreStats()

	if _, ok := stats["type"]; !ok {
		t.Error("Stats should contain 'type' field")
	}

	// Type should be hnsw since that's the default
	if stats["type"] != "hnsw" {
		t.Errorf("Expected type 'hnsw', got %v", stats["type"])
	}
}

func TestApplication_GetVectorStoreStats_NotInitialized(t *testing.T) {
	app := &Application{
		config: &config.Config{},
	}

	stats := app.GetVectorStoreStats()

	if stats["type"] != "unknown" {
		t.Errorf("Expected type 'unknown', got %v", stats["type"])
	}

	if stats["status"] != "not_initialized" {
		t.Errorf("Expected status 'not_initialized', got %v", stats["status"])
	}
}

func TestHealthChecker_StartTime(t *testing.T) {
	app, cleanup := createTestApp(t)
	defer cleanup()

	checker := NewHealthChecker(app, "1.0.0")

	// Start time should be set
	if checker.startTime.IsZero() {
		t.Error("Start time should not be zero")
	}

	// Start time should be in the past (or very close to now)
	if time.Since(checker.startTime) > time.Minute {
		t.Error("Start time should be recent")
	}
}
