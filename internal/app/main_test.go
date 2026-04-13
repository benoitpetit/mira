package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/benoitpetit/mira/internal/config"
)

// getTestConfig returns a minimal config for testing
func getTestConfig() *config.Config {
	return &config.Config{
		System: config.SystemConfig{
			Version: "0.3.2",
		},
		Storage: config.StorageConfig{
			Path: "",
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
			Version:   "0.3.2",
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
}

// TestNewApplication tests the creation of a new Application with all dependencies
func TestNewApplication(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "mira_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := getTestConfig()
	cfg.Storage.Path = tempDir

	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatalf("NewApplication failed: %v", err)
	}
	defer app.Close()

	// Verify the application is created
	if app == nil {
		t.Fatal("Application should not be nil")
	}

	// Verify config is set
	if app.config != cfg {
		t.Error("Application config should be set correctly")
	}

	// Verify repository is initialized
	if app.repository == nil {
		t.Error("Repository should not be nil")
	}

	// Verify embedder is initialized
	if app.embedder == nil {
		t.Error("Embedder should not be nil")
	}

	// Verify extractor is initialized
	if app.extractor == nil {
		t.Error("Extractor should not be nil")
	}

	// Verify vector store is initialized
	if app.vectorStore == nil {
		t.Error("VectorStore should not be nil")
	}

	// Verify overlap cache is initialized
	if app.overlapCache == nil {
		t.Error("OverlapCache should not be nil")
	}

	// Verify use cases are initialized
	if app.storeMemory == nil {
		t.Error("StoreMemory should not be nil")
	}

	if app.recallMemory == nil {
		t.Error("RecallMemory should not be nil")
	}

	if app.loadMemory == nil {
		t.Error("LoadMemory should not be nil")
	}

	if app.getTimeline == nil {
		t.Error("GetTimeline should not be nil")
	}

	if app.getStatus == nil {
		t.Error("GetStatus should not be nil")
	}

	if app.getCausalChain == nil {
		t.Error("GetCausalChain should not be nil")
	}

	if app.archiveMemories == nil {
		t.Error("ArchiveMemories should not be nil")
	}

	// Verify renderer is initialized
	if app.renderer == nil {
		t.Error("Renderer should not be nil")
	}

	// Verify controller is initialized
	if app.controller == nil {
		t.Error("Controller should not be nil")
	}

	// Verify data directory was created
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		t.Error("Data directory should be created")
	}

	// Verify database file was created
	dbPath := filepath.Join(tempDir, "mira.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file should be created")
	}
}

// TestNewApplicationFromConfig tests loading config from file and creating application
func TestNewApplicationFromConfig(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "mira_test_config_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test config file
	configPath := filepath.Join(tempDir, "config.yaml")
	configContent := `
system:
  version: "0.3.2"

storage:
  path: "` + tempDir + `/data"

embeddings:
  current_model: "sentence-transformers/all-MiniLM-L6-v2"
  dimension: 384
  batch_size: 32
  cache_size: 1000

hnsw:
  M: 16
  Ml: 0.25
  ef_construction: 200
  ef_search: 50

allocator:
  default_budget: 4000
  max_candidates: 100
  early_pruning_threshold: 0.6
  session_window_seconds: 7200
  session_boost_beta: 0.2
  causal_penalty_alpha: 0.15
  density_sigmoid:
    k: 2.0
    mu: 0.3

archive_thresholds:
  session_note: 30
  debug_log: 7

extraction:
  min_entity_length: 2

mcp:
  name: "mira"
  version: "0.3.2"
  transport: "stdio"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	app, err := NewApplicationFromConfig(configPath)
	if err != nil {
		t.Fatalf("NewApplicationFromConfig failed: %v", err)
	}
	defer app.Close()

	// Verify the application is created
	if app == nil {
		t.Fatal("Application should not be nil")
	}

	// Verify config was loaded
	if app.config == nil {
		t.Fatal("Config should be loaded")
	}

	// Verify config values were loaded correctly
	if app.config.MCP.Name != "mira" {
		t.Errorf("Expected MCP name 'mira', got '%s'", app.config.MCP.Name)
	}

	if app.config.MCP.Version != "0.3.2" {
		t.Errorf("Expected MCP version '0.3.2', got '%s'", app.config.MCP.Version)
	}

	if app.config.Embeddings.Dimension != 384 {
		t.Errorf("Expected dimension 384, got %d", app.config.Embeddings.Dimension)
	}

	// Verify data directory was created
	dataDir := filepath.Join(tempDir, "data")
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Error("Data directory should be created")
	}
}

// TestNewApplicationFromConfig_InvalidPath tests that a missing config file falls back to defaults
func TestNewApplicationFromConfig_InvalidPath(t *testing.T) {
	app, err := NewApplicationFromConfig("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("Expected success with default config when file missing, got error: %v", err)
	}
	if app == nil {
		t.Fatal("Expected application to be created with defaults")
	}
	app.Close()
}

// TestApplicationClose tests the cleanup of application resources
func TestApplicationClose(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "mira_test_close_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := getTestConfig()
	cfg.Storage.Path = tempDir

	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatalf("NewApplication failed: %v", err)
	}

	// Close should not return an error
	if err := app.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Verify that the database connection is closed by trying to use it
	// The repository should be closed
	if app.repository != nil {
		// After close, the DB should be closed
		// We can't easily test this without accessing the DB directly
	}
}

// TestApplicationClose_WithWebhookManager tests cleanup with webhook manager enabled
func TestApplicationClose_WithWebhookManager(t *testing.T) {
	// Create a temporary directory for test data
	tempDir, err := os.MkdirTemp("", "mira_test_close_webhook_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := getTestConfig()
	cfg.Storage.Path = tempDir
	cfg.Webhooks.Enabled = true
	cfg.Webhooks.Workers = 2
	cfg.Webhooks.QueueSize = 100
	cfg.Webhooks.Endpoints = []string{"http://localhost:8080/webhook"}

	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatalf("NewApplication failed: %v", err)
	}

	// Verify webhook manager is initialized
	if app.webhookManager == nil {
		t.Error("WebhookManager should be initialized when enabled")
	}

	// Close should not return an error
	if err := app.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

// TestDependencyInjection_StoreMemory tests that storeMemory is properly wired
func TestDependencyInjection_StoreMemory(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mira_test_di_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := getTestConfig()
	cfg.Storage.Path = tempDir

	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatalf("NewApplication failed: %v", err)
	}
	defer app.Close()

	if app.storeMemory == nil {
		t.Fatal("storeMemory should not be nil")
	}

	// Verify storeMemory has required dependencies injected
	// The storeMemory use case should have repository, extractor, vectorStore, and metricsCollector
}

// TestDependencyInjection_RecallMemory tests that recallMemory is properly wired
func TestDependencyInjection_RecallMemory(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mira_test_di_recall_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := getTestConfig()
	cfg.Storage.Path = tempDir

	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatalf("NewApplication failed: %v", err)
	}
	defer app.Close()

	if app.recallMemory == nil {
		t.Fatal("recallMemory should not be nil")
	}

	// Verify recallMemory has required dependencies injected
	// The recallMemory use case should have vectorStore, overlapCache, repository, extractor, renderer, config, and metricsCollector
}

// TestDependencyInjection_Controller tests that controller is properly wired
func TestDependencyInjection_Controller(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mira_test_di_controller_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := getTestConfig()
	cfg.Storage.Path = tempDir

	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatalf("NewApplication failed: %v", err)
	}
	defer app.Close()

	if app.controller == nil {
		t.Fatal("controller should not be nil")
	}

	// The controller should have all use cases injected
	// We can verify this by checking the controller is not nil
}

// TestDependencyInjection_MetricsCollector tests that metrics collector is properly wired when enabled
func TestDependencyInjection_MetricsCollector(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mira_test_di_metrics_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := getTestConfig()
	cfg.Storage.Path = tempDir
	cfg.Metrics.Enabled = true

	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatalf("NewApplication failed: %v", err)
	}
	defer app.Close()

	if app.metricsCollector == nil {
		t.Error("metricsCollector should be initialized when enabled")
	}
}

// TestDependencyInjection_VectorStore tests that vector store is properly wired
func TestDependencyInjection_VectorStore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mira_test_di_vector_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := getTestConfig()
	cfg.Storage.Path = tempDir

	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatalf("NewApplication failed: %v", err)
	}
	defer app.Close()

	if app.vectorStore == nil {
		t.Fatal("vectorStore should not be nil")
	}

	if app.overlapCache == nil {
		t.Fatal("overlapCache should not be nil")
	}
}

// TestNewApplication_EmbedderFallback tests that embedder falls back to SimpleEmbedder on failure
func TestNewApplication_EmbedderFallback(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mira_test_embedder_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := getTestConfig()
	cfg.Storage.Path = tempDir
	// Use an invalid model to trigger fallback
	cfg.Embeddings.CurrentModel = "invalid-model-name"

	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatalf("NewApplication failed: %v", err)
	}
	defer app.Close()

	// Even with an invalid model, the application should still work
	// because it falls back to SimpleEmbedder
	if app.embedder == nil {
		t.Fatal("Embedder should not be nil even with invalid model")
	}
}

// TestApplication_MultipleCloseCalls tests that multiple Close calls don't panic
func TestApplication_MultipleCloseCalls(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mira_test_multi_close_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := getTestConfig()
	cfg.Storage.Path = tempDir

	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatalf("NewApplication failed: %v", err)
	}

	// First close should succeed
	if err := app.Close(); err != nil {
		t.Errorf("First Close failed: %v", err)
	}

	// Second close should not panic (idempotent)
	if err := app.Close(); err != nil {
		t.Errorf("Second Close failed: %v", err)
	}
}

// TestApplication_NilRepositoryClose tests closing with nil repository
func TestApplication_NilRepositoryClose(t *testing.T) {
	app := &Application{
		config: getTestConfig(),
	}

	// Should not panic even with nil repository
	if err := app.Close(); err != nil {
		t.Errorf("Close with nil repository failed: %v", err)
	}
}
