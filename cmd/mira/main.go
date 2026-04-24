// MIRA - Memory with Information-theoretic Relevance Allocation
// Clean Architecture version with full feature support
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/benoitpetit/mira/internal/app"
	"github.com/benoitpetit/mira/internal/config"
)

func main() {
	var (
		configPath = flag.String("config", config.ResolveConfigPath(""), "Path to configuration file")
		migrate    = flag.Bool("migrate", false, "Run database migrations and exit")
		version    = flag.Bool("version", false, "Show version and exit")
		withSoul   = flag.Bool("with-soul", false, "Enable SOUL identity subsystem (MIRA+SOUL mode, 16 tools)")
	)
	flag.Parse()

	if *version {
		fmt.Println("MIRA v0.4.6")
		fmt.Println("Features: Dependency Inversion, Domain-Driven Design, Testable Architecture")
		fmt.Println("          HNSW Vector Index, Cybertron Embeddings")
		fmt.Println("          Webhook Notifications, Prometheus Metrics")
		fmt.Println("Architecture: Clean Architecture (Uncle Bob)")
		fmt.Println()
		fmt.Println("Layer Structure:")
		fmt.Println("  - Domain: Entities, Value Objects (pure business logic)")
		fmt.Println("  - Use Cases: Interactors, Repository Ports")
		fmt.Println("  - Interfaces: Controllers, Presenters")
		fmt.Println("  - Adapters: SQLite, HNSW, NLP Extraction, Webhooks")
		fmt.Println("  - Frameworks: MCP Server, SQLite3, Cybertron, Prometheus")
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.LoadOrDefault(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// CLI overrides
	if *withSoul {
		cfg.Soul.Enabled = true
	}

	if *migrate {
		// Just initialize the app to trigger migrations, then exit
		application, err := app.NewApplication(cfg)
		if err != nil {
			log.Fatalf("Migration failed: %v", err)
		}
		application.Close()
		fmt.Println("Database migrations completed successfully")
		os.Exit(0)
	}

	// Run the application
	application, err := app.NewApplication(cfg)
	if err != nil {
		log.Fatalf("Application error: %v", err)
	}
	if err := application.Run(); err != nil {
		log.Fatalf("Application error: %v", err)
	}
}
