// Main application - Composition Root with full feature integration
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/benoitpetit/mira/internal/adapters/extraction"
	"github.com/benoitpetit/mira/internal/adapters/logging"
	"github.com/benoitpetit/mira/internal/adapters/metrics"
	"github.com/benoitpetit/mira/internal/adapters/storage"
	"github.com/benoitpetit/mira/internal/adapters/vector"
	webhookadapter "github.com/benoitpetit/mira/internal/adapters/webhook"
	"github.com/benoitpetit/mira/internal/config"
	"github.com/benoitpetit/mira/internal/domain/entities"
	mcpserver "github.com/benoitpetit/mira/internal/interfaces/mcp"
	"github.com/benoitpetit/mira/internal/usecases/interactors"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	mcptypes "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	soul "github.com/benoitpetit/soul"
)

// Application holds all dependencies
type Application struct {
	config           *config.Config
	repository       *storage.SQLiteRepository
	embedder         ports.Embedder
	extractor        ports.Extractor
	vectorStore      ports.VectorStore
	overlapCache     *vector.SQLiteOverlapCache
	hnswIndex        *vector.HNSWStore
	storeMemory      *interactors.StoreMemory
	recallMemory     *interactors.RecallMemory
	loadMemory       *interactors.LoadMemory
	getTimeline      *interactors.GetTimeline
	getStatus        *interactors.GetStatus
	getCausalChain   *interactors.GetCausalChain
	archiveMemories  *interactors.ArchiveMemories
	clearMemory      *interactors.ClearMemory
	renderer         *interactors.DefaultFingerprintRenderer
	controller       *mcpserver.Controller
	webhookManager   ports.WebhookManager
	metricsCollector ports.MetricsCollector
	soulApp          *soul.Application
	soulCtrl         *soul.Controller
}

// NewApplication creates and wires all dependencies
func NewApplication(cfg *config.Config) (*Application, error) {
	app := &Application{config: cfg}

	// 1. Create data directory
	if err := os.MkdirAll(cfg.Storage.Path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}
	if err := ensureGitignore(cfg.Storage.Path); err != nil {
		slog.Info("could not ensure .gitignore", "error", err)
	}

	dbPath := cfg.Storage.Path + "/mira.db"
	modelsDir := cfg.Storage.Path + "/models"

	// 2. Initialize repository
	repo, err := storage.NewSQLiteRepository(dbPath, storage.SQLiteOptions{
		SessionNoteArchiveDays: int(cfg.ArchiveThresholds["session_note"]),
		DebugLogArchiveDays:    int(cfg.ArchiveThresholds["debug_log"]),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize repository: %w", err)
	}
	app.repository = repo

	// 2.5. Initialize SOUL identity sub-system (shares MIRA's SQLite connection)
	// SOUL is opt-in: must be explicitly enabled via config (soul.enabled: true) or --with-soul flag.
	if cfg.Soul.Enabled {
		if soulApp, err := soul.NewApplicationWithDB(repo.DB()); err != nil {
			slog.Warn("SOUL init failed, continuing without identity features", "error", err)
		} else {
			app.soulApp = soulApp
			app.soulCtrl = soul.NewController(soulApp)
			slog.Info("SOUL identity sub-system initialized", "tools", len(app.soulCtrl.ToolDefinitions()))
		}
	} else {
		slog.Info("SOUL is not enabled — running MIRA-only mode. Use --with-soul or set soul.enabled: true to activate identity features.")
	}

	// Log database stats
	stats, err := repo.GetStats(context.Background())
	if err == nil {
		slog.Info("database connected", "verbatims", stats.VerbatimCount, "fingerprints", stats.FingerprintCount, "embeddings", stats.EmbeddingCount)
	}

	// 3. Initialize metrics if enabled
	if cfg.Metrics.Enabled {
		if cfg.Metrics.PrometheusAddr != "" {
			// Use Prometheus collector with HTTP endpoint
			promCollector := metrics.NewPrometheusCollector()
			app.metricsCollector = promCollector
			slog.Info("prometheus collector enabled")

			// Start Prometheus HTTP server in background
			go func() {
				slog.Info("starting prometheus server", "addr", cfg.Metrics.PrometheusAddr+"/metrics")
				if err := promCollector.StartServer(cfg.Metrics.PrometheusAddr); err != nil {
					slog.Error("prometheus server error", "error", err)
				}
			}()
		} else {
			// Use simple collector (existing)
			app.metricsCollector = metrics.NewSimpleMetricsCollector()
			slog.Info("simple metrics collector enabled")
		}
	}

	// 4. Initialize embedder (Cybertron or Simple)
	if cfg.Embeddings.UseSimpleEmbedder {
		slog.Info("using simple embedder")
		app.embedder = extraction.NewSimpleEmbedder(cfg.Embeddings.Dimension)
	} else {
		cybertronEmbedder, err := extraction.NewCybertronEmbedder(extraction.CybertronEmbedderOptions{
			ModelName: cfg.Embeddings.CurrentModel,
			ModelsDir: modelsDir,
			Dimension: cfg.Embeddings.Dimension,
		})
		if err != nil {
			slog.Warn("failed to load cybertron model, falling back to simple embedder", "error", err)
			app.embedder = extraction.NewSimpleEmbedder(cfg.Embeddings.Dimension)
		} else {
			app.embedder = cybertronEmbedder
		}
	}

	// 5. Initialize extractor (NativeExtractor replaces archived prose library)
	app.extractor, err = extraction.NewNativeExtractor(app.embedder, extraction.NativeExtractorOptions{
		ModelName:       cfg.Embeddings.CurrentModel,
		MinEntityLength: cfg.Extraction.MinEntityLength,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize extractor: %w", err)
	}

	// 6. Register embedding model
	model := entities.NewEmbeddingModel(cfg.Embeddings.CurrentModel, cfg.Embeddings.Dimension)
	model.WithMetadata("batch_size", cfg.Embeddings.BatchSize)
	if err := repo.RegisterModel(context.Background(), model); err != nil {
		slog.Warn("failed to register embedding model", "error", err)
	}

	// Validate model hash consistency
	registeredModels, _ := repo.GetAllModels(context.Background())
	hasModelHash := false
	for _, mh := range registeredModels {
		if mh == cfg.Embeddings.ModelHash {
			hasModelHash = true
		}
	}
	if !hasModelHash && len(registeredModels) > 0 {
		slog.Warn("embedding model hash mismatch detected",
			"config_model_hash", cfg.Embeddings.ModelHash,
			"registered_models", registeredModels,
			"action", "run mira_reindex or clear memory to rebuild embeddings")
	}

	// 7. Initialize vector store (HNSW with SQLite fallback)
	app.overlapCache = vector.NewSQLiteOverlapCache(repo.DB())

	hnswOpts := vector.DefaultHNSWOptions()
	if cfg.HNSW.M > 0 {
		hnswOpts.M = cfg.HNSW.M
	}
	if cfg.HNSW.Ml > 0 {
		hnswOpts.Ml = cfg.HNSW.Ml
	}
	if cfg.HNSW.EfConstruction > 0 {
		hnswOpts.EfConstruction = cfg.HNSW.EfConstruction
	}
	if cfg.HNSW.EfSearch > 0 {
		hnswOpts.EfSearch = cfg.HNSW.EfSearch
	}

	indexPath := cfg.Storage.Path + "/vectors.bin"
	hnswIndex, err := vector.NewHNSWStore(repo, cfg.Embeddings.Dimension, indexPath, hnswOpts)
	if err != nil {
		slog.Warn("failed to initialize hnsw index, falling back to sqlite vector search", "error", err)
		app.vectorStore = vector.NewSQLiteVectorStore(repo.DB())
	} else {
		// Try to load existing index
		if err := hnswIndex.Load(); err != nil {
			slog.Warn("failed to load hnsw index, will build from scratch", "error", err)
		} else if hnswIndex.IsReady() {
			slog.Info("hnsw index loaded from disk", "vectors", hnswIndex.Stats())
		}

		// Build from DB if index is not ready (no file or error)
		if !hnswIndex.IsReady() {
			slog.Info("building hnsw index from sqlite")
			go func() {
				if err := hnswIndex.BuildFromStore(context.Background()); err != nil {
					slog.Warn("failed to build hnsw index", "error", err)
				}
			}()
		}

		app.hnswIndex = hnswIndex
		// Wrap HNSW with SQLite fallback so recall works while the index is building
		app.vectorStore = vector.NewFallbackVectorStore(hnswIndex, vector.NewSQLiteVectorStore(repo.DB()))
	}

	// 8. Initialize webhook manager if enabled
	if cfg.Webhooks.Enabled {
		slog.Info("initializing webhook manager")

		timeout := time.Duration(cfg.Webhooks.Timeout) * time.Second
		webhookMgr := webhookadapter.NewSimpleWebhookManagerWithDB(
			cfg.Webhooks.Workers,
			cfg.Webhooks.QueueSize,
			timeout,
			repo.DB(),
		)
		app.webhookManager = webhookMgr

		// Register default endpoints from config
		for _, endpoint := range cfg.Webhooks.Endpoints {
			if endpoint != "" {
				app.webhookManager.Register(context.Background(), endpoint, []string{"*"}, "")
				slog.Info("registered webhook endpoint", "url", endpoint)
			}
		}

		slog.Info("webhooks enabled", "workers", cfg.Webhooks.Workers, "endpoints", len(cfg.Webhooks.Endpoints))
	}

	// 10. Initialize renderer
	app.renderer = interactors.NewDefaultFingerprintRenderer()

	// 10.5 Initialize logger
	logger := logging.NewSimpleLoggerWithPrefix("[StoreMemory]", false)

	// 11. Initialize use cases (interactors)
	app.storeMemory = interactors.NewStoreMemory(
		repo, app.extractor, app.extractor, app.vectorStore, app.metricsCollector, logger,
	)

	app.recallMemory = interactors.NewRecallMemory(
		app.vectorStore,
		app.overlapCache,
		repo,
		app.extractor,
		app.renderer,
		interactors.RecallMemoryConfig{
			DefaultBudget:                 cfg.Allocator.DefaultBudget,
			MaxCandidates:                 cfg.Allocator.MaxCandidates,
			EarlyPruningThreshold:         cfg.Allocator.EarlyPruningThreshold,
			SessionWindowSeconds:          cfg.Allocator.SessionWindowSeconds,
			SessionBoostBeta:              cfg.Allocator.SessionBoostBeta,
			SessionBoostMax:               cfg.Allocator.SessionBoostMax,
			CausalPenaltyAlpha:            cfg.Allocator.CausalPenaltyAlpha,
			DensitySigmoidK:               cfg.Allocator.DensitySigmoid.K,
			DensitySigmoidMu:              cfg.Allocator.DensitySigmoid.Mu,
			EmbeddingCacheSize:            cfg.Embeddings.CacheSize,
			ThresholdMethod:               cfg.Recall.AdaptiveThresholdMethod,
			ThresholdFloor:                cfg.Recall.AdaptiveThresholdFloor,
			ThresholdCeiling:              cfg.Recall.AdaptiveThresholdCeiling,
			EnableFTS5:                    cfg.Recall.EnableFTS5,
			FTS5Limit:                     cfg.Recall.FTS5Limit,
			RRFK:                          cfg.Recall.RRFK,
			QueryExpansionEnabled:         cfg.Recall.QueryExpansion.Enabled,
			QueryExpansionNumVariants:     cfg.Recall.QueryExpansion.NumVariants,
			SearchTimeClusteringEnabled:   cfg.Recall.SearchTimeClustering.Enabled,
			SearchTimeClusteringThreshold: cfg.Recall.SearchTimeClustering.SimilarityThreshold,
			RerankerEnabled:               cfg.Recall.Reranker.Enabled,
			RerankerTopK:                  cfg.Recall.Reranker.TopK,
			TagRepo:                       repo,
			DecayRates:                    cfg.DecayRates,
		},
		app.metricsCollector,
	)

	app.loadMemory = interactors.NewLoadMemory(repo, repo)
	app.getTimeline = interactors.NewGetTimeline(repo)
	app.getStatus = interactors.NewGetStatus(repo, repo)
	app.getCausalChain = interactors.NewGetCausalChain(repo)
	app.archiveMemories = interactors.NewArchiveMemories(repo)
	app.clearMemory = interactors.NewClearMemory(repo, app.vectorStore)

	// 12. Initialize controller
	app.controller = mcpserver.NewController(
		app.storeMemory,
		app.recallMemory,
		app.loadMemory,
		app.getTimeline,
		app.getStatus,
		app.getCausalChain,
		app.archiveMemories,
		app.clearMemory,
		repo,
	)

	return app, nil
}

// Close cleans up resources
func (a *Application) Close() error {
	// Save HNSW index to disk
	if a.hnswIndex != nil {
		slog.Info("saving hnsw index to disk")
		if err := a.hnswIndex.Save(); err != nil {
			slog.Warn("failed to save hnsw index", "error", err)
		} else {
			slog.Info("hnsw index saved", "vectors", a.hnswIndex.Stats())
		}
	}

	// Stop webhook manager
	if a.webhookManager != nil {
		a.webhookManager.Stop()
	}

	// Close SOUL (does not close the shared DB connection)
	if a.soulApp != nil {
		a.soulApp.Close()
	}

	// Close repository
	if a.repository != nil {
		return a.repository.Close()
	}
	return nil
}

// Run starts the MCP server
func (a *Application) Run() error {
	defer a.Close()

	// Start webhook manager if enabled
	if a.webhookManager != nil {
		a.webhookManager.Start()
	}

	// Create MCP server
	s := server.NewDefaultServer(a.config.MCP.Name, a.config.MCP.Version)

	// Register combined MIRA + SOUL tools
	if a.soulCtrl != nil {
		// Combined mode: register all tools from both systems
		miraTools := a.controller.ToolDefinitions()
		soulTools := a.soulCtrl.ToolDefinitions()
		allTools := append(miraTools, soulTools...)
		slog.Info("MCP tools registered", "mira", len(miraTools), "soul", len(soulTools), "total", len(allTools))

		s.HandleListTools(func(ctx context.Context, cursor *string) (*mcptypes.ListToolsResult, error) {
			return &mcptypes.ListToolsResult{Tools: allTools}, nil
		})
		s.HandleCallTool(func(ctx context.Context, name string, arguments map[string]interface{}) (*mcptypes.CallToolResult, error) {
			if strings.HasPrefix(name, "soul_") {
				return a.soulCtrl.Call(ctx, name, arguments)
			}
			return a.controller.Call(ctx, name, arguments)
		})
	} else {
		// MIRA-only mode
		a.controller.RegisterTools(s)
		slog.Info("MCP tools registered", "mira", len(a.controller.ToolDefinitions()))
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		slog.Info("mcp server ready", "name", a.config.MCP.Name, "version", a.config.MCP.Version, "transport", a.config.MCP.Transport, "budget", a.config.Allocator.DefaultBudget)

		if a.config.MCP.Transport == "stdio" {
			errChan <- server.ServeStdio(s)
		} else {
			// Note: Only stdio transport is currently supported.
			// SSE and HTTP transports may be added in future versions.
			errChan <- fmt.Errorf("unsupported transport: %s (only stdio is supported)", a.config.MCP.Transport)
		}
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		slog.Info("received shutdown signal", "signal", sig)
		cancel()
		return nil
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return nil
	}
}

// NewApplicationFromConfig loads config and creates a new application
func NewApplicationFromConfig(configPath string) (*Application, error) {
	cfg, err := config.LoadOrDefault(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	return NewApplication(cfg)
}

// RunWithConfig loads config and runs the application
func RunWithConfig(configPath string) error {
	app, err := NewApplicationFromConfig(configPath)
	if err != nil {
		return err
	}
	return app.Run()
}

// ensureGitignore adds .mira/ to .gitignore if a .gitignore exists in the project root.
func ensureGitignore(dataPath string) error {
	absPath, err := filepath.Abs(dataPath)
	if err != nil {
		return err
	}
	projectDir := filepath.Dir(absPath)
	gitignorePath := filepath.Join(projectDir, ".gitignore")

	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		return nil // no gitignore, nothing to do
	}

	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		return err
	}

	s := string(content)
	if strings.Contains(s, ".mira") {
		return nil // already ignored
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if !strings.HasSuffix(s, "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}

	_, err = f.WriteString("# MIRA project data\n.mira/\n")
	return err
}
