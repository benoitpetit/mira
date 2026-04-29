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
	mirasoul "github.com/benoitpetit/mira/internal/adapters/soul"
	"github.com/benoitpetit/mira/internal/adapters/storage"
	"github.com/benoitpetit/mira/internal/adapters/vector"
	webhookadapter "github.com/benoitpetit/mira/internal/adapters/webhook"
	"github.com/benoitpetit/mira/internal/config"
	"github.com/benoitpetit/mira/internal/domain/entities"
	mcpserver "github.com/benoitpetit/mira/internal/interfaces/mcp"
	"github.com/benoitpetit/mira/internal/usecases/interactors"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	soul "github.com/benoitpetit/soul"
	mcptypes "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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
	deleteMemory     *interactors.DeleteMemory
	searchSemantic   *interactors.SearchSemantic
	updateMemory     *interactors.UpdateMemory
	consolidateMemories *interactors.ConsolidateMemories
	renderer         *interactors.DefaultFingerprintRenderer
	controller       *mcpserver.Controller
	webhookManager   ports.WebhookManager
	metricsCollector ports.MetricsCollector
	soulApp          *soul.Application
	soulCtrl         *soul.Controller
	startTime        time.Time
}

// NewApplication creates and wires all dependencies
func NewApplication(cfg *config.Config) (*Application, error) {
	app := &Application{config: cfg, startTime: time.Now()}

	// 1. Create data directory
	if err := os.MkdirAll(cfg.Storage.Path, 0o755); err != nil {
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
	// When enabled, MIRA passes its own soul.* configuration block to SOUL so that
	// embedded mode supports the same tuning options as standalone mode.
	if cfg.Soul.Enabled {
		soulCfg := soul.DefaultConfig()
		soulCfg.MinTraitConfidence = cfg.Soul.Extraction.MinTraitConfidence
		soulCfg.MinObservationsForTrait = cfg.Soul.Extraction.MinObservationsForTrait
		soulCfg.MaxContextTokens = cfg.Soul.Recall.DefaultBudgetTokens
		soulCfg.DriftThreshold = cfg.Soul.DriftDetection.Threshold
		soulCfg.DriftWindowSize = cfg.Soul.DriftDetection.WindowSize
		soulCfg.AutoCheckAfterCapture = cfg.Soul.DriftDetection.AutoCheckAfterCapture
		soulCfg.AutoReinforce = cfg.Soul.ModelSwap.AutoReinforce
		soulCfg.EvolutionEnabled = cfg.Soul.Evolution.Enabled
		soulCfg.MaxHistoryVersions = cfg.Soul.Evolution.MaxHistoryVersions

		if soulApp, err := soul.NewApplicationWithDBAndConfig(repo.DB(), soulCfg); err != nil {
			slog.Warn("SOUL init failed, continuing without identity features", "error", err)
		} else {
			soulApp.SetMiraProvider(mirasoul.NewMiraProvider(repo.DB()))
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
		// Set model hash for validation
		hnswIndex.SetModelHash(cfg.Embeddings.ModelHash)

		// Try to load existing index
		if err := hnswIndex.Load(); err != nil {
			slog.Warn("failed to load hnsw index, will build from scratch", "error", err)
			// If mismatch (dimension or model hash), clear the stale index file
			if strings.Contains(err.Error(), "mismatch") {
				slog.Info("stale hnsw index detected, removing old index file")
				_ = os.Remove(indexPath)
				_ = os.Remove(indexPath + ".sha256")
			}
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

	recallLogger := logging.NewSimpleLoggerWithPrefix("[RecallMemory]", false)

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
		recallLogger,
	)

	app.loadMemory = interactors.NewLoadMemory(repo, repo)
	app.getTimeline = interactors.NewGetTimeline(repo)
	app.getStatus = interactors.NewGetStatus(repo, repo, app.startTime, "0.4.7")
	app.getCausalChain = interactors.NewGetCausalChain(repo)
	app.archiveMemories = interactors.NewArchiveMemories(repo)
	app.clearMemory = interactors.NewClearMemory(repo, app.vectorStore)
	app.deleteMemory = interactors.NewDeleteMemory(repo, app.vectorStore)
	app.searchSemantic = interactors.NewSearchSemantic(app.vectorStore, app.embedder)
	app.updateMemory = interactors.NewUpdateMemory(repo, app.extractor, app.vectorStore)
	app.consolidateMemories = interactors.NewConsolidateMemories(repo, app.vectorStore, app.embedder, app.extractor)

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

	// Advertise tools capability in the initialize handshake.
	// The default handler only advertises Resources, so Claude Code never
	// requests tools/list without this override.
	name, version := a.config.MCP.Name, a.config.MCP.Version
	s.HandleInitialize(func(ctx context.Context, _ mcptypes.ClientCapabilities, _ mcptypes.Implementation, _ string) (*mcptypes.InitializeResult, error) {
		return &mcptypes.InitializeResult{
			ProtocolVersion: "2024-11-05",
			ServerInfo:      mcptypes.Implementation{Name: name, Version: version},
			Capabilities: mcptypes.ServerCapabilities{
				Tools: &struct {
					ListChanged bool `json:"listChanged"`
				}{ListChanged: false},
			},
		}, nil
	})

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
	var sseServer *server.SSEServer
	go func() {
		slog.Info("mcp server ready", "name", a.config.MCP.Name, "version", a.config.MCP.Version, "transport", a.config.MCP.Transport, "budget", a.config.Allocator.DefaultBudget)

		switch a.config.MCP.Transport {
		case "stdio":
			errChan <- server.ServeStdio(s)
		case "sse":
			sseServer = server.NewSSEServer(s, "http://"+a.config.MCP.Address)
			errChan <- sseServer.Start(a.config.MCP.Address)
		default:
			errChan <- fmt.Errorf("unsupported transport: %s (stdio or sse supported)", a.config.MCP.Transport)
		}
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		slog.Info("received shutdown signal", "signal", sig)
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		// Shutdown SSE server first if running
		if sseServer != nil {
			if err := sseServer.Shutdown(shutdownCtx); err != nil {
				slog.Warn("sse server shutdown error", "error", err)
			}
		}
		done := make(chan error, 1)
		go func() {
			done <- a.Close()
		}()
		select {
		case err := <-done:
			if err != nil {
				slog.Warn("graceful shutdown completed with error", "error", err)
			} else {
				slog.Info("graceful shutdown completed")
			}
		case <-shutdownCtx.Done():
			slog.Warn("graceful shutdown timed out")
		}
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

// Library accessors for external modules (e.g., Miracloud SaaS)

func (a *Application) StoreMemoryUC() *interactors.StoreMemory     { return a.storeMemory }
func (a *Application) RecallMemoryUC() *interactors.RecallMemory   { return a.recallMemory }
func (a *Application) LoadMemoryUC() *interactors.LoadMemory       { return a.loadMemory }
func (a *Application) GetTimelineUC() *interactors.GetTimeline     { return a.getTimeline }
func (a *Application) GetStatusUC() *interactors.GetStatus         { return a.getStatus }
func (a *Application) GetCausalChainUC() *interactors.GetCausalChain { return a.getCausalChain }
func (a *Application) ArchiveMemoriesUC() *interactors.ArchiveMemories { return a.archiveMemories }
func (a *Application) ClearMemoryUC() *interactors.ClearMemory     { return a.clearMemory }
func (a *Application) DeleteMemoryUC() *interactors.DeleteMemory   { return a.deleteMemory }
func (a *Application) SearchSemanticUC() *interactors.SearchSemantic { return a.searchSemantic }
func (a *Application) UpdateMemoryUC() *interactors.UpdateMemory     { return a.updateMemory }
func (a *Application) ConsolidateMemoriesUC() *interactors.ConsolidateMemories { return a.consolidateMemories }
func (a *Application) SoulApplication() *soul.Application          { return a.soulApp }

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

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0o644)
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
