// Main application - Composition Root with full feature integration
package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
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
	restserver "github.com/benoitpetit/mira/internal/interfaces/rest"
	"github.com/benoitpetit/mira/internal/usecases/interactors"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	soul "github.com/benoitpetit/soul"
	mcptypes "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Application holds all dependencies
type Application struct {
	config              *config.Config
	repository          ports.Repository
	embedder            ports.Embedder
	extractor           ports.Extractor
	vectorStore         ports.VectorStore
	overlapCache        *vector.SQLiteOverlapCache
	hnswIndex           *vector.HNSWStore
	storeMemory         *interactors.StoreMemory
	recallMemory        *interactors.RecallMemory
	loadMemory          *interactors.LoadMemory
	getTimeline         *interactors.GetTimeline
	getStatus           *interactors.GetStatus
	getCausalChain      *interactors.GetCausalChain
	archiveMemories     *interactors.ArchiveMemories
	clearMemory         *interactors.ClearMemory
	deleteMemory        *interactors.DeleteMemory
	searchSemantic      *interactors.SearchSemantic
	updateMemory        *interactors.UpdateMemory
	consolidateMemories *interactors.ConsolidateMemories
	compressMemories   *interactors.CompressMemories
	renderer            *interactors.DefaultFingerprintRenderer
	controller          *mcpserver.Controller
	webhookManager      ports.WebhookManager
	metricsCollector    ports.MetricsCollector
	soulApp             *soul.Application
	soulCtrl            *soul.Controller
	restServer          *http.Server
	startTime           time.Time
	closeOnce           sync.Once
}

// NewApplication creates and wires all dependencies.
// Each sub-system is initialised by a dedicated private method so that this
// function reads as a clear, ordered sequence of concerns.
func NewApplication(cfg *config.Config) (*Application, error) {
	app := &Application{config: cfg, startTime: time.Now()}

	dbPath := cfg.Storage.Path + "/mira.db"
	modelsDir := cfg.Storage.Path + "/models"

	if err := app.initStorage(dbPath); err != nil {
		return nil, err
	}
	app.initMetrics()
	if err := app.initEmbedder(modelsDir); err != nil {
		return nil, err
	}
	if err := app.initExtractor(); err != nil {
		return nil, err
	}
	if err := app.initVectorStore(dbPath); err != nil {
		return nil, err
	}
	app.initWebhooks()
	app.initUseCases()
	// SOUL is initialized after use cases so storeMemory is available
	app.initSoul()
	app.initRestAPI()

	return app, nil
}

// ── Private init helpers ─────────────────────────────────────────────────────

// initStorage creates the data directory and opens the repository based on config type.
func (a *Application) initStorage(dbPath string) error {
	if a.config.Storage.Type == "postgres" {
		return a.initPostgresStorage()
	}

	if err := os.MkdirAll(a.config.Storage.Path, 0o755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}
	if err := ensureGitignore(a.config.Storage.Path); err != nil {
		slog.Info("could not ensure .gitignore", "error", err)
	}

	repo, err := storage.NewSQLiteRepository(dbPath, storage.SQLiteOptions{
		SessionNoteArchiveDays: int(a.config.ArchiveThresholds["session_note"]),
		DebugLogArchiveDays:    int(a.config.ArchiveThresholds["debug_log"]),
		EncryptionKey:          a.config.Storage.SQLite.EncryptionKey,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize repository: %w", err)
	}
	a.repository = repo

	stats, err := repo.GetStats(context.Background())
	if err == nil {
		slog.Info("database connected",
			"type", "sqlite",
			"verbatims", stats.VerbatimCount,
			"fingerprints", stats.FingerprintCount,
			"embeddings", stats.EmbeddingCount)
	}
	return nil
}

// initPostgresStorage initializes a PostgreSQL repository.
func (a *Application) initPostgresStorage() error {
	opts := storage.PostgreSQLOptions{
		URL:         a.config.Storage.Postgres.URL,
		MaxConns:    a.config.Storage.Postgres.MaxConns,
		MinConns:    a.config.Storage.Postgres.MinConns,
		MaxIdleTime: time.Duration(a.config.Storage.Postgres.MaxIdleTime) * time.Second,
		MaxConnTime: time.Duration(a.config.Storage.Postgres.MaxConnTime) * time.Second,
	}

	repo, err := storage.NewPostgreSQLRepository(opts)
	if err != nil {
		return fmt.Errorf("failed to initialize postgres repository: %w", err)
	}
	a.repository = repo

	stats, err := repo.GetStats(context.Background())
	if err == nil {
		slog.Info("database connected",
			"type", "postgres",
			"verbatims", stats.VerbatimCount,
			"fingerprints", stats.FingerprintCount,
			"embeddings", stats.EmbeddingCount)
	}
	return nil
}

// initSoul wires the SOUL identity sub-system when enabled.
// Failure is non-fatal: MIRA continues without identity features.
func (a *Application) initSoul() {
	cfg := a.config
	if !cfg.Soul.Enabled {
		slog.Info("SOUL is not enabled — running MIRA-only mode. Use --with-soul or set soul.enabled: true to activate identity features.")
		return
	}

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
	if cfg.Soul.Memory.EnrichWithMiraMemories {
		soulCfg.EnrichWithMiraMemories = true
	}
	if cfg.Soul.Memory.MaxMiraMemories > 0 {
		soulCfg.MaxMiraMemories = cfg.Soul.Memory.MaxMiraMemories
	}

	soulApp, err := soul.NewApplicationWithDBAndConfig(a.repository.DB(), soulCfg)
	if err != nil {
		slog.Warn("SOUL init failed, continuing without identity features", "error", err)
		return
	}
	soulApp.SetMiraProvider(mirasoul.NewMiraProvider(a.repository.DB(), a.storeMemory))
	a.soulApp = soulApp
	a.soulCtrl = soul.NewController(soulApp)
	slog.Info("SOUL identity sub-system initialized", "tools", len(a.soulCtrl.ToolDefinitions()))
}

// initMetrics starts the configured metrics back-end (Prometheus or simple).
func (a *Application) initMetrics() {
	if !a.config.Metrics.Enabled {
		return
	}
	if a.config.Metrics.PrometheusAddr != "" {
		promCollector := metrics.NewPrometheusCollector()
		a.metricsCollector = promCollector
		slog.Info("prometheus collector enabled")
		go func() {
			slog.Info("starting prometheus server", "addr", a.config.Metrics.PrometheusAddr+"/metrics")
			if err := promCollector.StartServer(a.config.Metrics.PrometheusAddr); err != nil {
				slog.Error("prometheus server error", "error", err)
			}
		}()
	} else {
		a.metricsCollector = metrics.NewSimpleMetricsCollector()
		slog.Info("simple metrics collector enabled")
	}
}

// initEmbedder loads the Cybertron model or falls back to the simple embedder.
func (a *Application) initEmbedder(modelsDir string) error {
	cfg := a.config
	if cfg.Embeddings.UseSimpleEmbedder {
		slog.Info("using simple embedder")
		a.embedder = extraction.NewSimpleEmbedder(cfg.Embeddings.Dimension)
		return nil
	}

	cybertronEmbedder, err := extraction.NewCybertronEmbedder(extraction.CybertronEmbedderOptions{
		ModelName: cfg.Embeddings.CurrentModel,
		ModelsDir: modelsDir,
		Dimension: cfg.Embeddings.Dimension,
	})
	if err != nil {
		slog.Warn("failed to load cybertron model, falling back to simple embedder", "error", err)
		a.embedder = extraction.NewSimpleEmbedder(cfg.Embeddings.Dimension)
	} else {
		a.embedder = cybertronEmbedder
	}
	return nil
}

// initExtractor creates the fingerprint extractor.
// When cfg.Extraction.LLM.Enabled is true an OllamaExtractor is used (with NativeExtractor
// as fallback). Otherwise the NativeExtractor is used directly.
func (a *Application) initExtractor() error {
	cfg := a.config
	nativeOpts := extraction.NativeExtractorOptions{
		ModelName:       cfg.Embeddings.CurrentModel,
		MinEntityLength: cfg.Extraction.MinEntityLength,
	}

	if cfg.Extraction.LLM.Enabled {
		ollamaOpts := extraction.OllamaExtractorOptions{
			Endpoint:        cfg.Extraction.LLM.Endpoint,
			Model:           cfg.Extraction.LLM.Model,
			Timeout:         time.Duration(cfg.Extraction.LLM.TimeoutSeconds) * time.Second,
			FallbackOnError: cfg.Extraction.LLM.FallbackOnError,
			NativeOptions:   nativeOpts,
		}
		ext, err := extraction.NewOllamaExtractor(a.embedder, ollamaOpts)
		if err != nil {
			return fmt.Errorf("failed to initialize ollama extractor: %w", err)
		}
		a.extractor = ext
		slog.Info("extractor initialized", "backend", "ollama", "model", cfg.Extraction.LLM.Model, "endpoint", cfg.Extraction.LLM.Endpoint)
		return nil
	}

	ext, err := extraction.NewNativeExtractor(a.embedder, nativeOpts)
	if err != nil {
		return fmt.Errorf("failed to initialize extractor: %w", err)
	}
	a.extractor = ext
	slog.Info("extractor initialized", "backend", "native")
	return nil
}

// initVectorStore registers the embedding model, then builds the HNSW index
// (or falls back to the SQLite vector store when HNSW init fails).
func (a *Application) initVectorStore(dbPath string) error {
	cfg := a.config
	repo := a.repository
	ctx := context.Background()

	// Register model
	model := entities.NewEmbeddingModel(cfg.Embeddings.CurrentModel, cfg.Embeddings.Dimension)
	model.WithMetadata("batch_size", cfg.Embeddings.BatchSize)
	if err := repo.RegisterModel(ctx, model); err != nil {
		slog.Warn("failed to register embedding model", "error", err)
	}

	// Validate model hash consistency
	registeredModels, _ := repo.GetAllModels(ctx)
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

	// Overlap cache (shared between HNSW and RecallMemory)
	a.overlapCache = vector.NewSQLiteOverlapCache(repo.DB())

	// HNSW options
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
		a.vectorStore = vector.NewSQLiteVectorStore(repo.DB())
		return nil
	}

	hnswIndex.SetModelHash(cfg.Embeddings.ModelHash)

	// AES-256-GCM encryption for vectors.bin (optional, opt-in via HNSW.EncryptionKey / MIRA_HNSW_KEY)
	if key := cfg.HNSW.EncryptionKey; key != "" {
		h := sha256.Sum256([]byte(key))
		hnswIndex.SetEncryptionKey(h[:])
	}

	if err := hnswIndex.Load(); err != nil {
		slog.Warn("failed to load hnsw index, will build from scratch", "error", err)
		if strings.Contains(err.Error(), "mismatch") {
			slog.Info("stale hnsw index detected, removing old index file")
			_ = os.Remove(indexPath)
			_ = os.Remove(indexPath + ".sha256")
		}
	} else if hnswIndex.IsReady() {
		slog.Info("hnsw index loaded from disk", "vectors", hnswIndex.Stats())
	}

	if !hnswIndex.IsReady() {
		slog.Info("building hnsw index from sqlite")
		go func() {
			if err := hnswIndex.BuildFromStore(ctx); err != nil {
				slog.Warn("failed to build hnsw index", "error", err)
			}
		}()
	}

	a.hnswIndex = hnswIndex
	// Wrap with SQLite fallback so recall works while the index is building
	a.vectorStore = vector.NewFallbackVectorStore(hnswIndex, vector.NewSQLiteVectorStore(repo.DB()))
	return nil
}

// initWebhooks starts the webhook manager and registers configured endpoints.
func (a *Application) initWebhooks() {
	cfg := a.config
	if !cfg.Webhooks.Enabled {
		return
	}
	slog.Info("initializing webhook manager")

	timeout := time.Duration(cfg.Webhooks.Timeout) * time.Second
	webhookMgr := webhookadapter.NewSimpleWebhookManagerWithDB(
		cfg.Webhooks.Workers,
		cfg.Webhooks.QueueSize,
		timeout,
		a.repository.DB(),
	)
	a.webhookManager = webhookMgr

	ctx := context.Background()
	for _, endpoint := range cfg.Webhooks.Endpoints {
		if endpoint != "" {
			a.webhookManager.Register(ctx, endpoint, []string{"*"}, "")
			slog.Info("registered webhook endpoint", "url", endpoint)
		}
	}
	slog.Info("webhooks enabled",
		"workers", cfg.Webhooks.Workers,
		"endpoints", len(cfg.Webhooks.Endpoints))
}

// initUseCases wires all interactors and the MCP controller.
func (a *Application) initUseCases() {
	cfg := a.config
	repo := a.repository

	a.renderer = interactors.NewDefaultFingerprintRenderer()

	logger := logging.NewSimpleLoggerWithPrefix("[StoreMemory]", false)
	a.storeMemory = interactors.NewStoreMemory(
		repo, a.extractor, a.extractor, a.vectorStore, a.metricsCollector, logger,
	)
	a.storeMemory.WithCompression(cfg.Compression.AutoCompress, cfg.Compression.MinTokens)

	recallLogger := logging.NewSimpleLoggerWithPrefix("[RecallMemory]", false)
	a.recallMemory = interactors.NewRecallMemory(
		a.vectorStore,
		a.overlapCache,
		repo,
		a.extractor,
		a.renderer,
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
		a.metricsCollector,
		recallLogger,
	)

	a.loadMemory = interactors.NewLoadMemory(repo, repo)
	a.getTimeline = interactors.NewGetTimeline(repo)
	a.getStatus = interactors.NewGetStatus(repo, repo, a.startTime, "0.4.7")
	a.getCausalChain = interactors.NewGetCausalChain(repo)
	a.archiveMemories = interactors.NewArchiveMemories(repo)
	a.clearMemory = interactors.NewClearMemory(repo, a.vectorStore)
	a.deleteMemory = interactors.NewDeleteMemory(repo, a.vectorStore)
	a.searchSemantic = interactors.NewSearchSemantic(a.vectorStore, a.embedder)
	a.updateMemory = interactors.NewUpdateMemory(repo, a.extractor, a.vectorStore)
	a.consolidateMemories = interactors.NewConsolidateMemories(repo, a.vectorStore, a.embedder, a.extractor)
	a.compressMemories = interactors.NewCompressMemories(repo, repo)

	a.controller = mcpserver.NewController(
		a.storeMemory,
		a.recallMemory,
		a.loadMemory,
		a.getTimeline,
		a.getStatus,
		a.getCausalChain,
		a.archiveMemories,
		a.clearMemory,
		repo,
		a.compressMemories,
	)
}

// initRestAPI builds the optional REST HTTP server if enabled in config.
// The server is stored on the Application; it is started in Run() and shut
// down gracefully in Close().
func (a *Application) initRestAPI() {
	if !a.config.API.Enabled {
		return
	}
	// Only enable DB-based policy auth when static auth tokens are configured.
	// Passing a non-nil PolicyRepository unconditionally would activate the auth
	// middleware even with no token configured, causing 401 on all requests.
	var policyRepo ports.PolicyRepository
	if a.config.API.AuthToken != "" || len(a.config.API.WingTokens) > 0 {
		policyRepo = a.repository
	}
	h := restserver.NewHandler(
		a.storeMemory,
		a.recallMemory,
		a.loadMemory,
		a.updateMemory,
		a.deleteMemory,
		a.searchSemantic,
		a.consolidateMemories,
		a.clearMemory,
		a.getTimeline,
		a.archiveMemories,
		a.getCausalChain,
		a.getStatus,
		a.repository,
		policyRepo,
	)
	readTimeout := time.Duration(a.config.API.ReadTimeout) * time.Second
	writeTimeout := time.Duration(a.config.API.WriteTimeout) * time.Second
	a.restServer = restserver.NewServer(h, a.config.API.Address, a.config.API.AuthToken, a.config.API.WingTokens, readTimeout, writeTimeout)
	if a.soulApp != nil {
		h.SetSoulQuerier(mirasoul.NewStatusQuerier(a.soulApp))
	}
	slog.Info("rest api configured", "addr", a.config.API.Address)
}

// ── Lifecycle ────────────────────────────────────────────────────────────────

// Close cleans up resources. It is safe to call multiple times; only the first
// call performs actual cleanup (subsequent calls are no-ops).
func (a *Application) Close() error {
	var closeErr error
	a.closeOnce.Do(func() {
		if a.restServer != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := a.restServer.Shutdown(shutdownCtx); err != nil {
				slog.Warn("rest server shutdown error", "error", err)
			}
		}

		if a.hnswIndex != nil {
			slog.Info("saving hnsw index to disk")
			if err := a.hnswIndex.Save(); err != nil {
				slog.Warn("failed to save hnsw index", "error", err)
			} else {
				slog.Info("hnsw index saved", "vectors", a.hnswIndex.Stats())
			}
		}

		if a.webhookManager != nil {
			a.webhookManager.Stop()
		}

		if a.soulApp != nil {
			a.soulApp.Close()
		}

		if a.repository != nil {
			closeErr = a.repository.Close()
		}
	})
	return closeErr
}

// Run starts the MCP server
func (a *Application) Run() error {
	defer a.Close()

	if a.webhookManager != nil {
		a.webhookManager.Start()
	}

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

	if a.soulCtrl != nil {
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
		a.controller.RegisterTools(s)
		slog.Info("MCP tools registered", "mira", len(a.controller.ToolDefinitions()))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the optional REST API server in a background goroutine.
	if a.restServer != nil {
		go func() {
			slog.Info("rest api listening", "addr", a.restServer.Addr)
			if err := a.restServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("rest server error", "error", err)
			}
		}()
	}

	errChan := make(chan error, 1)
	var sseServer *server.SSEServer
	var httpHandler *mcpserver.MCPServerHandler
	go func() {
		slog.Info("mcp server ready",
			"name", a.config.MCP.Name,
			"version", a.config.MCP.Version,
			"transport", a.config.MCP.Transport,
			"budget", a.config.Allocator.DefaultBudget)

		switch a.config.MCP.Transport {
		case "stdio":
			errChan <- server.ServeStdio(s)
		case "sse":
			sseServer = server.NewSSEServer(s, "http://"+a.config.MCP.Address)
			errChan <- sseServer.Start(a.config.MCP.Address)
		case "http":
			httpHandler := mcpserver.NewMCPServerHandler(s, a.config.MCP.Address)
			errChan <- httpHandler.Start(a.config.MCP.Address)
		default:
			errChan <- fmt.Errorf("unsupported transport: %s (stdio, sse, or http supported)", a.config.MCP.Transport)
		}
	}()

	select {
	case sig := <-sigChan:
		slog.Info("received shutdown signal", "signal", sig)
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if sseServer != nil {
			if err := sseServer.Shutdown(shutdownCtx); err != nil {
				slog.Warn("sse server shutdown error", "error", err)
			}
		}
		if httpHandler != nil {
			if err := httpHandler.Shutdown(shutdownCtx); err != nil {
				slog.Warn("http server shutdown error", "error", err)
			}
		}
		done := make(chan error, 1)
		go func() { done <- a.Close() }()
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

// ── Constructor helpers ──────────────────────────────────────────────────────

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

// ── Library accessors (for external modules e.g. Miracloud SaaS) ─────────────

func (a *Application) StoreMemoryUC() *interactors.StoreMemory         { return a.storeMemory }
func (a *Application) RecallMemoryUC() *interactors.RecallMemory       { return a.recallMemory }
func (a *Application) LoadMemoryUC() *interactors.LoadMemory           { return a.loadMemory }
func (a *Application) GetTimelineUC() *interactors.GetTimeline         { return a.getTimeline }
func (a *Application) GetStatusUC() *interactors.GetStatus             { return a.getStatus }
func (a *Application) GetCausalChainUC() *interactors.GetCausalChain   { return a.getCausalChain }
func (a *Application) ArchiveMemoriesUC() *interactors.ArchiveMemories { return a.archiveMemories }
func (a *Application) ClearMemoryUC() *interactors.ClearMemory         { return a.clearMemory }
func (a *Application) DeleteMemoryUC() *interactors.DeleteMemory       { return a.deleteMemory }
func (a *Application) SearchSemanticUC() *interactors.SearchSemantic   { return a.searchSemantic }
func (a *Application) UpdateMemoryUC() *interactors.UpdateMemory       { return a.updateMemory }
func (a *Application) ConsolidateMemoriesUC() *interactors.ConsolidateMemories {
	return a.consolidateMemories
}
func (a *Application) CompressMemoriesUC() *interactors.CompressMemories {
	return a.compressMemories
}
func (a *Application) SoulApplication() *soul.Application { return a.soulApp }

// ── Internal helpers ─────────────────────────────────────────────────────────

// ensureGitignore adds .mira/ to .gitignore if a .gitignore exists in the project root.
func ensureGitignore(dataPath string) error {
	absPath, err := filepath.Abs(dataPath)
	if err != nil {
		return err
	}
	projectDir := filepath.Dir(absPath)
	gitignorePath := filepath.Join(projectDir, ".gitignore")

	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		return nil
	}

	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		return err
	}

	s := string(content)
	if strings.Contains(s, ".mira") {
		return nil
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
