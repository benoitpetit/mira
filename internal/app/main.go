// Main application - Composition Root with full feature integration
package app

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
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
	"github.com/benoitpetit/mira/internal/adapters/storage"
	"github.com/benoitpetit/mira/internal/adapters/vector"
	webhookadapter "github.com/benoitpetit/mira/internal/adapters/webhook"
	"github.com/benoitpetit/mira/internal/agentmemory"
	"github.com/benoitpetit/mira/internal/config"
	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	mcpserver "github.com/benoitpetit/mira/internal/interfaces/mcp"
	restserver "github.com/benoitpetit/mira/internal/interfaces/rest"
	"github.com/benoitpetit/mira/internal/usecases/interactors"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
	mcptypes "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Application holds all dependencies
type Application struct {
	config              *config.Config
	repository          ports.Repository
	embedder            ports.Embedder
	extractor           ports.Extractor //nolint:staticcheck // consolidation still requires the composite legacy interface
	vectorStore         ports.VectorStore
	overlapCache        ports.OverlapCache
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
	compressMemories    *interactors.CompressMemories
	renderer            *interactors.DefaultFingerprintRenderer
	controller          *mcpserver.Controller
	webhookManager      ports.WebhookManager
	metricsCollector    ports.MetricsCollector
	agentMemoryApp      *agentmemory.Runtime
	agentMemoryCtrl     *agentmemory.Controller
	restServer          *http.Server
	metricsServer       *http.Server
	startTime           time.Time
	closeOnce           sync.Once
	buildCancel         context.CancelFunc // cancels HNSW build goroutine
}

// NewApplication creates and wires all dependencies.
// Each sub-system is initialized by a dedicated private method so that this
// function reads as a clear, ordered sequence of concerns.
func NewApplication(cfg *config.Config) (app *Application, err error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration must not be nil")
	}
	app = &Application{config: cfg, startTime: time.Now()}
	defer func() {
		if err != nil {
			_ = app.Close()
		}
	}()

	dbPath := cfg.Storage.Path + "/mira.db"
	modelsDir := cfg.Storage.Path + "/models"

	if err := app.initStorage(dbPath); err != nil {
		return nil, err
	}
	app.initMetrics()
	app.initEmbedder(modelsDir)
	if err := app.initExtractor(); err != nil {
		return nil, err
	}
	app.initVectorStore()
	app.initWebhooks()
	app.initUseCases()
	// Agent memory is initialized after use cases so the shared MIRA memory
	// provider is available to the identity continuity engine.
	if err := app.initAgentMemory(); err != nil {
		return nil, err
	}
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
		URL:                    a.config.Storage.Postgres.URL,
		MaxConns:               a.config.Storage.Postgres.MaxConns,
		MinConns:               a.config.Storage.Postgres.MinConns,
		MaxIdleTime:            time.Duration(a.config.Storage.Postgres.MaxIdleTime) * time.Second,
		MaxConnTime:            time.Duration(a.config.Storage.Postgres.MaxConnTime) * time.Second,
		SessionNoteArchiveDays: int(a.config.ArchiveThresholds["session_note"]),
		DebugLogArchiveDays:    int(a.config.ArchiveThresholds["debug_log"]),
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

// initAgentMemory wires the built-in agent memory continuity subsystem on the
// same database and dialect as the rest of MIRA.
func (a *Application) initAgentMemory() error {
	cfg := a.config

	memoryCfg := agentmemory.Config{
		MinTraitConfidence:      cfg.AgentMemory.Extraction.MinTraitConfidence,
		MinObservationsForTrait: cfg.AgentMemory.Extraction.MinObservationsForTrait,
		DefaultBudgetTokens:     cfg.AgentMemory.Recall.DefaultBudgetTokens,
		MaxBudgetTokens:         cfg.Allocator.DefaultBudget,
		DriftThreshold:          cfg.AgentMemory.DriftDetection.Threshold,
		DriftWindowSize:         cfg.AgentMemory.DriftDetection.WindowSize,
		AutoCheckAfterCapture:   cfg.AgentMemory.DriftDetection.AutoCheckAfterCapture,
		AutoReinforce:           cfg.AgentMemory.ModelSwap.AutoReinforce,
		EvolutionEnabled:        cfg.AgentMemory.Evolution.Enabled,
		MaxHistoryVersions:      cfg.AgentMemory.Evolution.MaxHistoryVersions,
		EnrichWithMiraMemories:  cfg.AgentMemory.Memory.EnrichWithMiraMemories,
		MaxMiraMemories:         cfg.AgentMemory.Memory.MaxMiraMemories,
	}
	recall := func(ctx context.Context, query string, budget int) ([]agentmemory.MemoryReference, error) {
		out, err := a.recallMemory.Execute(ctx, interactors.RecallMemoryInput{Query: query, Budget: budget})
		if err != nil {
			return nil, err
		}
		memories := make([]agentmemory.MemoryReference, 0, len(out.Memories))
		for _, selected := range out.Memories {
			memories = append(memories, agentmemory.MemoryReference{MemoryID: selected.VerbatimID, Content: selected.Rendered, Relevance: selected.Confidence, Timestamp: selected.SelectedAt})
		}
		return memories, nil
	}
	store := func(ctx context.Context, content, wing string, room *string, memoryType *valueobjects.MemoryType) (uuid.UUID, error) {
		out, err := a.storeMemory.Execute(ctx, interactors.StoreMemoryInput{Content: content, Wing: wing, Room: room, Type: memoryType})
		if err != nil {
			return uuid.Nil, err
		}
		return uuid.Parse(out.FingerprintID)
	}
	provider := agentmemory.NewMiraProviderWithDialect(a.repository.DB(), recall, store, cfg.Storage.Type)
	memoryApp, err := agentmemory.NewRuntimeWithDialect(a.repository.DB(), memoryCfg, provider, cfg.Storage.Type)
	if err != nil {
		return fmt.Errorf("failed to initialize built-in agent memory: %w", err)
	}
	a.agentMemoryApp = memoryApp
	a.agentMemoryCtrl = agentmemory.NewController(memoryApp)
	slog.Info("agent memory continuity initialized", "tools", len(a.agentMemoryCtrl.ToolDefinitions()))
	return nil
}

// initMetrics starts the configured metrics back-end (Prometheus or simple).
func (a *Application) initMetrics() {
	if !a.config.Metrics.Enabled {
		return
	}
	if a.config.Metrics.PrometheusAddr != "" {
		promCollector := metrics.NewPrometheusCollector()
		a.metricsCollector = promCollector
		healthChecker := NewHealthChecker(a, a.config.MCP.Version)
		mux := http.NewServeMux()
		mux.Handle("/metrics", promCollector.Handler())
		mux.Handle("/health", healthChecker.Handler())
		mux.Handle("/health/live", healthChecker.LivenessHandler())
		mux.Handle("/health/ready", healthChecker.ReadinessHandler())
		a.metricsServer = &http.Server{
			Addr:         a.config.Metrics.PrometheusAddr,
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
		slog.Info("prometheus collector enabled")
		go func() {
			slog.Info("starting observability server", "addr", a.config.Metrics.PrometheusAddr)
			if err := a.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("prometheus server error", "error", err)
			}
		}()
	} else {
		a.metricsCollector = metrics.NewSimpleMetricsCollector()
		slog.Info("simple metrics collector enabled")
	}
}

// initEmbedder loads the Cybertron model or falls back to the simple embedder.
func (a *Application) initEmbedder(modelsDir string) {
	cfg := a.config
	if cfg.Embeddings.UseSimpleEmbedder {
		slog.Info("using simple embedder")
		a.embedder = extraction.NewSimpleEmbedder(cfg.Embeddings.Dimension)
		return
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
// (or falls back to a portable brute-force vector store when HNSW is unavailable).
func (a *Application) initVectorStore() {
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
	a.overlapCache = vector.NewSQLOverlapCache(repo.DB(), cfg.Storage.Type)

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
		slog.Warn("hnsw.EfConfiguration is set but ignored: the underlying coder/hnsw library v0.4.0 does not support this field. Set to 0 in config to suppress this warning.")
	}
	if cfg.HNSW.EfSearch > 0 {
		hnswOpts.EfSearch = cfg.HNSW.EfSearch
	}

	// Create a cancellable context for the HNSW build goroutine
	ctx, buildCancel := context.WithCancel(ctx)
	a.buildCancel = buildCancel

	indexPath := cfg.Storage.Path + "/vectors.bin"
	hnswIndex, err := vector.NewHNSWStore(repo, cfg.Embeddings.Dimension, indexPath, hnswOpts)
	if err != nil {
		slog.Warn("failed to initialize hnsw index, falling back to brute-force vector search", "error", err)
		a.vectorStore = vector.NewBruteForceVectorStore(repo)
		return
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
		slog.Info("building hnsw index from authoritative repository", "storage", cfg.Storage.Type)
		go func() {
			if err := hnswIndex.BuildFromStore(ctx); err != nil {
				slog.Warn("failed to build hnsw index", "error", err)
			}
		}()
	}

	a.hnswIndex = hnswIndex
	// Wrap with a portable fallback so recall works while the index is building.
	a.vectorStore = vector.NewFallbackVectorStore(hnswIndex, vector.NewBruteForceVectorStore(repo))
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
			SessionMemoryBoost:            cfg.Allocator.SessionMemoryBoost,
			SessionCacheTTLSeconds:        cfg.Allocator.SessionCacheTTLSeconds,
			SessionCacheStore:             vector.NewSQLSessionCache(repo.DB(), cfg.Storage.Type),
			CausalPenaltyAlpha:            cfg.Allocator.CausalPenaltyAlpha,
			DiversityBoostAlpha:           cfg.Allocator.DiversityBoostAlpha,
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
	a.getStatus = interactors.NewGetStatus(repo, repo, a.startTime, config.CurrentVersion)
	a.getCausalChain = interactors.NewGetCausalChain(repo)
	a.archiveMemories = interactors.NewArchiveMemories(repo)
	a.clearMemory = interactors.NewClearMemory(repo, a.vectorStore)
	a.deleteMemory = interactors.NewDeleteMemory(repo, a.vectorStore)
	a.searchSemantic = interactors.NewSearchSemantic(a.vectorStore, a.embedder)
	a.updateMemory = interactors.NewUpdateMemory(repo, a.extractor, a.vectorStore).WithCausalDetector(a.extractor)
	a.consolidateMemories = interactors.NewConsolidateMemories(repo, a.vectorStore, a.embedder, a.extractor)
	a.compressMemories = interactors.NewCompressMemories(repo, repo)

	a.controller = mcpserver.NewControllerWithLimits(
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
		a.updateMemory,
		a.searchSemantic,
		a.consolidateMemories,
		mcpserver.ValidationLimits{
			MaxContentLength: cfg.MCP.MaxContentLength,
			MaxWingLength:    cfg.MCP.MaxWingLength,
			MaxRoomLength:    cfg.MCP.MaxRoomLength,
			MaxQueryLength:   cfg.MCP.MaxQueryLength,
		},
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
	if a.agentMemoryApp != nil {
		h.SetAgentMemoryQuerier(a.agentMemoryApp)
	}
	slog.Info("rest api configured", "addr", a.config.API.Address)
}

// ── Lifecycle ────────────────────────────────────────────────────────────────

// Close cleans up resources. It is safe to call multiple times; only the first
// call performs actual cleanup (subsequent calls are no-ops).
func (a *Application) Close() error {
	var closeErr error
	a.closeOnce.Do(func() {
		if a.metricsServer != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := a.metricsServer.Shutdown(shutdownCtx); err != nil {
				slog.Warn("observability server shutdown error", "error", err)
			}
		}
		if a.restServer != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := a.restServer.Shutdown(shutdownCtx); err != nil {
				slog.Warn("rest server shutdown error", "error", err)
			}
		}

		// Cancel the HNSW build goroutine if it's running
		if a.buildCancel != nil {
			a.buildCancel()
			a.buildCancel = nil
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

		// Agent memory shares the repository connection and has no independent
		// close operation. The repository remains the single owner of storage.

		if a.repository != nil {
			closeErr = a.repository.Close()
		}
	})
	return closeErr
}

// RebuildVectorIndex rebuilds the derived HNSW index from the authoritative
// embedding rows in the repository. It is safe to call after a failed write,
// a checksum mismatch, or an interrupted background build.
func (a *Application) RebuildVectorIndex(ctx context.Context) error {
	if a.hnswIndex == nil {
		return fmt.Errorf("hnsw index is not enabled")
	}
	if a.buildCancel != nil {
		a.buildCancel()
		a.buildCancel = nil
	}
	return a.hnswIndex.Rebuild(ctx)
}

// ReembedAll regenerates T2 embeddings from stored T0 content and updates the
// model hash in T1/T2. The command is intentionally explicit because it can
// be expensive on large memory stores.
func (a *Application) ReembedAll(ctx context.Context) (int, error) {
	if a.config.Storage.Type == "postgres" {
		return a.reembedAllPostgres(ctx)
	}
	if a.buildCancel != nil {
		a.buildCancel()
		a.buildCancel = nil
	}
	rows, err := a.repository.DB().QueryContext(ctx, `SELECT id, content FROM verbatim ORDER BY created_at, id`)
	if err != nil {
		return 0, fmt.Errorf("list memories for reembedding: %w", err)
	}
	type source struct {
		id      []byte
		content string
	}
	var sources []source
	for rows.Next() {
		var item source
		if err := rows.Scan(&item.id, &item.content); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("read memory for reembedding: %w", err)
		}
		sources = append(sources, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, fmt.Errorf("iterate memories for reembedding: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close memory scan for reembedding: %w", err)
	}

	modelHash := a.config.Embeddings.ModelHash
	updated := 0
	for _, item := range sources {
		id, err := uuid.FromBytes(item.id)
		if err != nil {
			return updated, fmt.Errorf("invalid memory ID during reembedding: %w", err)
		}
		vec, err := a.embedder.Encode(ctx, item.content)
		if err != nil {
			return updated, fmt.Errorf("embed memory %s: %w", id, err)
		}
		if len(vec) != a.config.Embeddings.Dimension {
			return updated, fmt.Errorf("embedding dimension mismatch for %s: got %d, expected %d", id, len(vec), a.config.Embeddings.Dimension)
		}
		emb := entities.NewEmbedding(id, modelHash, vec).WithNormalization()
		vectorBytes := make([]byte, len(emb.Vector)*4)
		for i, value := range emb.Vector {
			binary.LittleEndian.PutUint32(vectorBytes[i*4:], math.Float32bits(value))
		}

		tx, err := a.repository.Begin()
		if err != nil {
			return updated, fmt.Errorf("begin reembedding transaction for %s: %w", id, err)
		}
		result, err := tx.ExecContext(ctx,
			`UPDATE embeddings SET model_hash = ?, dim = ?, vector = ?, normalized = ?, created_at = ? WHERE id = ?`,
			modelHash, emb.Dim, vectorBytes, emb.Normalized, float64(emb.CreatedAt.Unix()), item.id,
		)
		if err != nil {
			_ = tx.Rollback()
			return updated, fmt.Errorf("update embedding %s: %w", id, err)
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			_ = tx.Rollback()
			return updated, fmt.Errorf("update embedding %s affected %d rows", id, affected)
		}
		result, err = tx.ExecContext(ctx,
			`UPDATE fingerprints SET model_hash = ? WHERE verbatim_id = ?`, modelHash, item.id,
		)
		if err != nil {
			_ = tx.Rollback()
			return updated, fmt.Errorf("update fingerprint model %s: %w", id, err)
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			_ = tx.Rollback()
			return updated, fmt.Errorf("update fingerprint model %s affected %d rows", id, affected)
		}
		if err := tx.Commit(); err != nil {
			return updated, fmt.Errorf("commit reembedding %s: %w", id, err)
		}
		updated++
	}

	if a.hnswIndex != nil {
		if err := a.hnswIndex.Rebuild(ctx); err != nil {
			return updated, fmt.Errorf("rebuild HNSW after reembedding: %w", err)
		}
	}
	return updated, nil
}

func (a *Application) reembedAllPostgres(ctx context.Context) (int, error) {
	rows, err := a.repository.DB().QueryContext(ctx, `SELECT id, content FROM verbatim ORDER BY created_at, id`)
	if err != nil {
		return 0, fmt.Errorf("list memories for PostgreSQL reembedding: %w", err)
	}
	defer rows.Close()
	type source struct {
		id      uuid.UUID
		content string
	}
	var sources []source
	for rows.Next() {
		var item source
		if err := rows.Scan(&item.id, &item.content); err != nil {
			return 0, fmt.Errorf("read memory for PostgreSQL reembedding: %w", err)
		}
		sources = append(sources, item)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate memories for PostgreSQL reembedding: %w", err)
	}

	modelHash := a.config.Embeddings.ModelHash
	updated := 0
	for _, item := range sources {
		vec, err := a.embedder.Encode(ctx, item.content)
		if err != nil {
			return updated, fmt.Errorf("embed memory %s: %w", item.id, err)
		}
		if len(vec) != a.config.Embeddings.Dimension {
			return updated, fmt.Errorf("embedding dimension mismatch for %s: got %d, expected %d", item.id, len(vec), a.config.Embeddings.Dimension)
		}
		emb := entities.NewEmbedding(item.id, modelHash, vec).WithNormalization()
		vectorValues := make([]string, len(emb.Vector))
		for i, value := range emb.Vector {
			vectorValues[i] = fmt.Sprintf("%f", value)
		}
		vectorLiteral := "[" + strings.Join(vectorValues, ",") + "]"

		tx, err := a.repository.Begin()
		if err != nil {
			return updated, fmt.Errorf("begin PostgreSQL reembedding transaction for %s: %w", item.id, err)
		}
		_, err = tx.ExecContext(ctx,
			`UPDATE embeddings SET model_hash = $1, dim = $2, vector = $3::vector, normalized = $4, created_at = $5 WHERE id = $6`,
			modelHash, emb.Dim, vectorLiteral, boolToInt(emb.Normalized), float64(emb.CreatedAt.Unix()), item.id)
		if err != nil {
			_ = tx.Rollback()
			return updated, fmt.Errorf("update PostgreSQL embedding %s: %w", item.id, err)
		}
		if _, err = tx.ExecContext(ctx, `UPDATE fingerprints SET model_hash = $1 WHERE verbatim_id = $2`, modelHash, item.id); err != nil {
			_ = tx.Rollback()
			return updated, fmt.Errorf("update PostgreSQL fingerprint model %s: %w", item.id, err)
		}
		if err := tx.Commit(); err != nil {
			return updated, fmt.Errorf("commit PostgreSQL reembedding %s: %w", item.id, err)
		}
		updated++
	}

	if a.hnswIndex != nil {
		if err := a.hnswIndex.Rebuild(ctx); err != nil {
			return updated, fmt.Errorf("rebuild HNSW after PostgreSQL reembedding: %w", err)
		}
	}
	return updated, nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
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

	if a.agentMemoryCtrl != nil {
		miraTools := a.controller.ToolDefinitions()
		agentMemoryTools := a.agentMemoryCtrl.ToolDefinitions()
		allTools := make([]mcptypes.Tool, 0, len(miraTools)+len(agentMemoryTools))
		allTools = append(allTools, miraTools...)
		allTools = append(allTools, agentMemoryTools...)
		slog.Info("MCP tools registered", "mira", len(miraTools), "soul", len(agentMemoryTools), "total", len(allTools))

		s.HandleListTools(func(ctx context.Context, cursor *string) (*mcptypes.ListToolsResult, error) {
			return &mcptypes.ListToolsResult{Tools: allTools}, nil
		})
		s.HandleCallTool(func(ctx context.Context, name string, arguments map[string]interface{}) (*mcptypes.CallToolResult, error) {
			if strings.HasPrefix(name, agentmemory.ToolPrefix) {
				return a.agentMemoryCtrl.Call(ctx, name, arguments)
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
			httpHandler = mcpserver.NewMCPServerHandlerWithAuth(s, a.config.MCP.AuthToken)
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

// AgentMemoryApplication returns the embedded agent-memory engine.
func (a *Application) AgentMemoryApplication() *agentmemory.Runtime { return a.agentMemoryApp }

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
