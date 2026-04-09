package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/benoitpetit/mira/budget"
	"github.com/benoitpetit/mira/causal"
	"github.com/benoitpetit/mira/config"
	"github.com/benoitpetit/mira/extract"
	mcpserver "github.com/benoitpetit/mira/mcp"
	"github.com/benoitpetit/mira/store"
	"github.com/benoitpetit/mira/types"
	"github.com/benoitpetit/mira/vector"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	var (
		configPath = flag.String("config", "config.yaml", "Path to configuration file")
		migrate    = flag.Bool("migrate", false, "Run database migrations and exit")
		version    = flag.Bool("version", false, "Show version and exit")
	)
	flag.Parse()

	if *version {
		fmt.Println("MIRA v0.1.2 - Memory with Information-theoretic Relevance Allocation")
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Printf("Warning: could not load config file %s: %v", *configPath, err)
		cfg = config.Default()
	}

	// Create data directory if needed
	if err := os.MkdirAll(cfg.Storage.Path, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	dbPath := cfg.Storage.Path + "/mira.db"

	// Initialize storage with config
	st, err := store.NewWithOptions(dbPath, store.StoreOptions{
		SessionNoteArchiveDays: int(cfg.ArchiveThresholds["session_note"]),
		DebugLogArchiveDays:    int(cfg.ArchiveThresholds["debug_log"]),
	})
	if err != nil {
		log.Fatalf("Failed to initialize store: %v", err)
	}
	defer st.Close()

	if *migrate {
		fmt.Println("Database migrations completed successfully")
		os.Exit(0)
	}

	// Initialize extractor with config
	embedder := extract.NewSimpleEmbedder(cfg.Embeddings.Dimension)
	ext, err := extract.NewExtractorWithOptions(cfg.Embeddings.CurrentModel, embedder, extract.ExtractorOptions{
		MinEntityLength: cfg.Extraction.MinEntityLength,
	})
	if err != nil {
		log.Fatalf("Failed to initialize extractor: %v", err)
	}

	// Register embedding model (use extractor's computed hash for consistency)
	model := &types.EmbeddingModel{
		ModelHash: ext.ModelHash(),
		ModelName: cfg.Embeddings.CurrentModel,
		Dimension: cfg.Embeddings.Dimension,
		CreatedAt: time.Now(),
		Metadata: map[string]any{
			"batch_size": cfg.Embeddings.BatchSize,
		},
	}
	if err := st.RegisterModel(model); err != nil {
		log.Printf("Warning: failed to register embedding model: %v", err)
	}

	// Initialize vector store (SQLite adapter for now)
	vecStore := vector.NewSQLiteAdapter(st)

	// Initialize overlap cache (persistent via SQLite)
	overlapCache := vector.NewSQLiteOverlapCache(st.DB())

	// Initialize causal graph
	cg := causal.New(st.DB())

	// Initialize budget allocator with config
	alloc := budget.NewAllocatorWithOptions(vecStore, overlapCache, cg, ext, budget.AllocatorOptions{
		DefaultBudget:         cfg.Allocator.DefaultBudget,
		MaxCandidates:         cfg.Allocator.MaxCandidates,
		EarlyPruningThreshold: cfg.Allocator.EarlyPruningThreshold,
		SessionWindowSeconds:  cfg.Allocator.SessionWindowSeconds,
		SessionBoostBeta:      cfg.Allocator.SessionBoostBeta,
		CausalPenaltyAlpha:    cfg.Allocator.CausalPenaltyAlpha,
		DensitySigmoidK:       cfg.Allocator.DensitySigmoid.K,
		DensitySigmoidMu:      cfg.Allocator.DensitySigmoid.Mu,
		EmbeddingCacheSize:    cfg.Embeddings.CacheSize,
	})

	// Create and configure MCP server
	mcpSrv := mcpserver.NewServer(st, alloc, ext, cg)

	s := server.NewDefaultServer(cfg.MCP.Name, cfg.MCP.Version)

	mcpSrv.RegisterTools(s)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signal handler for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nShutting down MIRA gracefully...")
		cancel()
		// Give some time for cleanup before force exit
		time.Sleep(100 * time.Millisecond)
	}()

	// Start server
	log.Printf("MIRA v%s starting...", cfg.System.Version)
	log.Printf("Database: %s", dbPath)
	log.Printf("MCP Server: %s (transport: %s)", cfg.MCP.Name, cfg.MCP.Transport)

	// Run server in a goroutine so we can handle shutdown
	serverErr := make(chan error, 1)
	go func() {
		if cfg.MCP.Transport == "stdio" {
			serverErr <- server.ServeStdio(s)
		} else {
			serverErr <- fmt.Errorf("only stdio transport is supported in this version")
		}
	}()

	// Wait for either server error or shutdown signal
	select {
	case err := <-serverErr:
		if err != nil {
			log.Printf("Server error: %v", err)
		}
	case <-ctx.Done():
		log.Println("Shutdown signal received, cleaning up...")
	}

	// Store cleanup is handled by defer st.Close() above
	log.Println("MIRA shutdown complete")
}
