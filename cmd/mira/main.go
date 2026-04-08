package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"mira/budget"
	"mira/causal"
	"mira/config"
	"mira/extract"
	mcpserver "mira/mcp"
	"mira/store"
	"mira/types"
	"mira/vector"
)

func main() {
	var (
		configPath = flag.String("config", "config.yaml", "Path to configuration file")
		migrate    = flag.Bool("migrate", false, "Run database migrations and exit")
		version    = flag.Bool("version", false, "Show version and exit")
	)
	flag.Parse()

	if *version {
		fmt.Println("MIRA v0.1.0 - Memory with Information-theoretic Relevance Allocation")
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

	// Initialize storage
	st, err := store.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize store: %v", err)
	}
	defer st.Close()

	if *migrate {
		fmt.Println("Database migrations completed successfully")
		os.Exit(0)
	}

	// Initialize extractor
	embedder := extract.NewSimpleEmbedder(cfg.Embeddings.Dimension)
	ext, err := extract.NewExtractor(cfg.Embeddings.CurrentModel, embedder)
	if err != nil {
		log.Fatalf("Failed to initialize extractor: %v", err)
	}

	// Register embedding model
	model := &types.EmbeddingModel{
		ModelHash: cfg.Embeddings.ModelHash,
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

	// Initialize causal graph
	cg := causal.New(st.DB())

	// Initialize budget allocator
	alloc := budget.NewAllocator(vecStore, nil, cg, ext)

	// Create and configure MCP server
	mcpSrv := mcpserver.NewServer(st, alloc, ext, cg)

	s := server.NewDefaultServer(cfg.MCP.Name, cfg.MCP.Version)

	mcpSrv.RegisterTools(s)

	// Signal handler for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nShutting down MIRA...")
		os.Exit(0)
	}()

	// Start server
	log.Printf("MIRA v%s starting...", cfg.System.Version)
	log.Printf("Database: %s", dbPath)
	log.Printf("MCP Server: %s (transport: %s)", cfg.MCP.Name, cfg.MCP.Transport)

	if cfg.MCP.Transport == "stdio" {
		if err := server.ServeStdio(s); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	} else {
		log.Fatal("Only stdio transport is supported in this version")
	}
}
