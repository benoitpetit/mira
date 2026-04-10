// Main application - Composition Root with full feature integration
package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/benoitpetit/mira/internal/adapters/extraction"
	"github.com/benoitpetit/mira/internal/adapters/metrics"
	"github.com/benoitpetit/mira/internal/adapters/storage"
	"github.com/benoitpetit/mira/internal/adapters/vector"
	webhookadapter "github.com/benoitpetit/mira/internal/adapters/webhook"
	"github.com/benoitpetit/mira/internal/config"
	"github.com/benoitpetit/mira/internal/domain/entities"
	mcpserver "github.com/benoitpetit/mira/internal/interfaces/mcp"
	"github.com/benoitpetit/mira/internal/usecases/interactors"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/mark3labs/mcp-go/server"
)

// Application holds all dependencies
type Application struct {
	config           *config.Config
	repository       *storage.SQLiteRepository
	embedder         ports.Embedder
	extractor        *extraction.ProseExtractor
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
	renderer         *interactors.DefaultFingerprintRenderer
	controller       *mcpserver.Controller
	webhookManager   ports.WebhookManager
	metricsCollector ports.MetricsCollector
}

// NewApplication creates and wires all dependencies
func NewApplication(cfg *config.Config) (*Application, error) {
	app := &Application{config: cfg}

	// 1. Create data directory
	if err := os.MkdirAll(cfg.Storage.Path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
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

	// Log database stats
	stats, err := repo.GetStats(context.Background())
	if err == nil {
		log.Printf("[DB] Connected: %d verbatims, %d fingerprints, %d embeddings",
			stats.VerbatimCount, stats.FingerprintCount, stats.EmbeddingCount)
	}

	// 3. Initialize metrics if enabled
	if cfg.Metrics.Enabled {
		if cfg.Metrics.PrometheusAddr != "" {
			// Use Prometheus collector with HTTP endpoint
			promCollector := metrics.NewPrometheusCollector()
			app.metricsCollector = promCollector
			log.Println("[Metrics] Prometheus collector enabled")

			// Start Prometheus HTTP server in background
			go func() {
				log.Printf("[Metrics] Starting Prometheus server on %s/metrics", cfg.Metrics.PrometheusAddr)
				if err := promCollector.StartServer(cfg.Metrics.PrometheusAddr); err != nil {
					log.Printf("[Metrics] Prometheus server error: %v", err)
				}
			}()
		} else {
			// Use simple collector (existing)
			app.metricsCollector = metrics.NewSimpleMetricsCollector()
			log.Println("[Metrics] Simple collector enabled")
		}
	}

	// 4. Initialize embedder (Cybertron or Simple)
	useSimpleEmbedder := false // TODO: Add UseSimpleEmbedder to config
	if useSimpleEmbedder {
		log.Println("[Embedder] Using SimpleEmbedder (deterministic)")
		app.embedder = extraction.NewSimpleEmbedder(cfg.Embeddings.Dimension)
	} else {
		log.Printf("[Embedder] Loading model: %s", cfg.Embeddings.CurrentModel)

		cybertronEmbedder, err := extraction.NewCybertronEmbedder(extraction.CybertronEmbedderOptions{
			ModelName: cfg.Embeddings.CurrentModel,
			ModelsDir: modelsDir,
			Dimension: cfg.Embeddings.Dimension,
		})
		if err != nil {
			log.Printf("[Embedder] Warning: Failed to load model: %v", err)
			log.Println("[Embedder] Falling back to SimpleEmbedder")
			app.embedder = extraction.NewSimpleEmbedder(cfg.Embeddings.Dimension)
		} else {
			app.embedder = cybertronEmbedder
			log.Println("[Embedder] Model loaded successfully")
		}
	}

	// 5. Initialize extractor
	app.extractor, err = extraction.NewProseExtractor(app.embedder, extraction.ProseExtractorOptions{
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
		log.Printf("Warning: failed to register embedding model: %v", err)
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
		log.Printf("Warning: Failed to initialize HNSW index: %v", err)
		log.Println("Falling back to SQLite vector search")
		app.vectorStore = vector.NewSQLiteVectorStore(repo.DB())
	} else {
		// Essayer de charger l'index existant
		if err := hnswIndex.Load(); err != nil {
			log.Printf("[Vector] Failed to load HNSW index: %v", err)
			log.Println("[Vector] Will build from scratch...")
		} else if hnswIndex.IsReady() {
			log.Printf("[Vector] HNSW index loaded from disk: %d vectors", hnswIndex.Stats())
		}

		// Construire depuis la DB si l'index n'est pas prêt (pas de fichier ou erreur)
		if !hnswIndex.IsReady() {
			log.Println("[Vector] Building HNSW index from SQLite...")
			go func() {
				if err := hnswIndex.BuildFromStore(context.Background()); err != nil {
					log.Printf("[Vector] Warning: Failed to build HNSW index: %v", err)
				}
			}()
		}

		app.hnswIndex = hnswIndex
		app.vectorStore = hnswIndex
	}

	// 8. Initialize webhook manager if enabled
	if cfg.Webhooks.Enabled {
		log.Println("[Webhook] Initializing webhook manager...")

		timeout := time.Duration(cfg.Webhooks.Timeout) * time.Second
		webhookMgr := webhookadapter.NewSimpleWebhookManager(
			cfg.Webhooks.Workers,
			cfg.Webhooks.QueueSize,
			timeout,
		)
		app.webhookManager = webhookMgr

		// Register default endpoints from config
		for _, endpoint := range cfg.Webhooks.Endpoints {
			if endpoint != "" {
				app.webhookManager.Register(context.Background(), endpoint, []string{"*"}, "")
				log.Printf("[Webhook] Registered endpoint: %s", endpoint)
			}
		}

		log.Printf("[Webhook] Enabled: %d workers, %d endpoints",
			cfg.Webhooks.Workers, len(cfg.Webhooks.Endpoints))
	}

	// 10. Initialize renderer
	app.renderer = interactors.NewDefaultFingerprintRenderer()

	// 11. Initialize use cases (interactors)
	app.storeMemory = interactors.NewStoreMemory(
		repo, app.extractor, app.extractor, app.vectorStore, app.metricsCollector,
	)

	app.recallMemory = interactors.NewRecallMemory(
		app.vectorStore,
		app.overlapCache,
		repo,
		app.extractor,
		app.renderer,
		interactors.RecallMemoryConfig{
			DefaultBudget:         cfg.Allocator.DefaultBudget,
			MaxCandidates:         cfg.Allocator.MaxCandidates,
			EarlyPruningThreshold: cfg.Allocator.EarlyPruningThreshold,
			SessionWindowSeconds:  cfg.Allocator.SessionWindowSeconds,
			SessionBoostBeta:      cfg.Allocator.SessionBoostBeta,
			CausalPenaltyAlpha:    cfg.Allocator.CausalPenaltyAlpha,
			DensitySigmoidK:       cfg.Allocator.DensitySigmoid.K,
			DensitySigmoidMu:      cfg.Allocator.DensitySigmoid.Mu,
			EmbeddingCacheSize:    cfg.Embeddings.CacheSize,
		},
		app.metricsCollector,
	)

	app.loadMemory = interactors.NewLoadMemory(repo)
	app.getTimeline = interactors.NewGetTimeline(repo)
	app.getStatus = interactors.NewGetStatus(repo, repo)
	app.getCausalChain = interactors.NewGetCausalChain(repo)
	app.archiveMemories = interactors.NewArchiveMemories(repo)

	// 12. Initialize controller
	app.controller = mcpserver.NewController(
		app.storeMemory,
		app.recallMemory,
		app.loadMemory,
		app.getTimeline,
		app.getStatus,
		app.getCausalChain,
		app.archiveMemories,
	)

	return app, nil
}

// Close cleans up resources
func (a *Application) Close() error {
	// Sauvegarder l'index HNSW
	if a.hnswIndex != nil {
		log.Println("[Vector] Saving HNSW index to disk...")
		if err := a.hnswIndex.Save(); err != nil {
			log.Printf("[Vector] Warning: Failed to save HNSW index: %v", err)
		} else {
			log.Printf("[Vector] HNSW index saved: %d vectors", a.hnswIndex.Stats())
		}
	}

	// Stop webhook manager
	if a.webhookManager != nil {
		a.webhookManager.Stop()
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

	// Register tools
	a.controller.RegisterTools(s)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		log.Printf("[MCP] Server '%s' v%s ready on %s (budget: %d tokens)", a.config.MCP.Name, a.config.MCP.Version, a.config.MCP.Transport, a.config.Allocator.DefaultBudget)

		if a.config.MCP.Transport == "stdio" {
			errChan <- server.ServeStdio(s)
		} else {
			// TODO: Support other transports
			errChan <- fmt.Errorf("unsupported transport: %s", a.config.MCP.Transport)
		}
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		log.Printf("[Server] Received signal: %v, shutting down...", sig)
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
	cfg, err := config.Load(configPath)
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
