// MIRA - Memory with Information-theoretic Relevance Allocation
// Command-line interface using cobra
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/benoitpetit/mira/internal/app"
	"github.com/benoitpetit/mira/internal/config"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/interactors"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const miraVersion = "0.4.7"

// globalFlags holds flags shared by every subcommand.
var globalFlags struct {
	configPath  string
	storagePath string
}

const rootLong = `MIRA – Memory with Information-theoretic Relevance Allocation

MIRA is an MCP (Model Context Protocol) memory server that stores, retrieves,
and manages memories using HNSW vector search, Cybertron embeddings, and
optional SOUL identity features.

Usage: mira <command> [flags]

Examples:
  mira server                                    # Start MCP server
  mira server --with-api --api-addr :8080      # With REST API
  mira doctor                                   # Health check
  mira query -q "search term" --wing my-wing   # Recall memories

For detailed help on any command: mira <command> --help`

func main() {
	root := &cobra.Command{
		Use:   "mira",
		Short: "MIRA – Memory with Information-theoretic Relevance Allocation",
		Long:  rootLong,
		Version: miraVersion,
	}

	// Persistent flags shared by all subcommands.
	root.PersistentFlags().StringVarP(&globalFlags.configPath, "config", "c",
		config.ResolveConfigPath(""), "path to configuration file")
	root.PersistentFlags().StringVar(&globalFlags.storagePath, "storage-path", "",
		"override storage data directory (also: MIRA_DATA_PATH env)")

	root.AddCommand(
		newServerCmd(),
		newMigrateCmd(),
		newDoctorCmd(),
		newStatusCmd(),
		newQueryCmd(),
		newStoreCmd(),
		newDeleteCmd(),
		newExportCmd(),
		newImportCmd(),
		newConfigCmd(),
		newOptimizeCmd(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// server
// ---------------------------------------------------------------------------

func newServerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start the MCP memory server",
		Long: `Starts the MIRA MCP server.

Transport modes (--transport):
  stdio  – default, communication over stdin/stdout (Claude Desktop, etc.)
  sse    – HTTP Server-Sent Events at --mcp-addr (default localhost:3001)
  http   – plain HTTP at --mcp-addr (default localhost:3001)

Optional subsystems:
  --with-soul     enable SOUL identity subsystem (+8 tools, total 17)
  --with-api      expose a REST HTTP API (see --api-addr, --api-token)
  --with-llm      enable Ollama-backed extraction (see --llm-endpoint)
  --no-metrics    disable the Prometheus metrics endpoint

Examples:
  mira server
  mira server --transport sse --mcp-addr localhost:3001
  mira server --with-api --api-addr :8080 --api-token secret
  mira server --with-soul --with-api --api-token secret
  mira server --no-metrics
  mira server --with-llm --llm-endpoint http://localhost:11434`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			// Apply global storage-path override
			applyStoragePath(cfg)

			// MCP transport
			if t, _ := cmd.Flags().GetString("transport"); t != "" {
				cfg.MCP.Transport = t
			}
			if a, _ := cmd.Flags().GetString("mcp-addr"); a != "" {
				cfg.MCP.Address = a
			}

			// SOUL
			if on, _ := cmd.Flags().GetBool("with-soul"); on {
				cfg.Soul.Enabled = true
			}

			// REST API
			if on, _ := cmd.Flags().GetBool("with-api"); on {
				cfg.API.Enabled = true
			}
			if a, _ := cmd.Flags().GetString("api-addr"); a != "" {
				cfg.API.Address = a
			}
			if t, _ := cmd.Flags().GetString("api-token"); t != "" {
				cfg.API.AuthToken = t
			}

			// Metrics
			if off, _ := cmd.Flags().GetBool("no-metrics"); off {
				cfg.Metrics.Enabled = false
			}
			if a, _ := cmd.Flags().GetString("prometheus-addr"); a != "" {
				cfg.Metrics.PrometheusAddr = a
			}

			// LLM extractor
			if on, _ := cmd.Flags().GetBool("with-llm"); on {
				cfg.Extraction.LLM.Enabled = true
			}
			if ep, _ := cmd.Flags().GetString("llm-endpoint"); ep != "" {
				cfg.Extraction.LLM.Endpoint = ep
			}

			application, err := app.NewApplication(cfg)
			if err != nil {
				return fmt.Errorf("failed to start application: %w", err)
			}
			return application.Run()
		},
	}

	// Transport
	cmd.Flags().String("transport", "", "MCP transport: stdio (default), sse, or http")
	cmd.Flags().String("mcp-addr", "", "bind address for sse/http transport (default localhost:3001)")

	// SOUL
	cmd.Flags().Bool("with-soul", false, "enable SOUL identity subsystem (+8 tools, total 17)")

	// REST API
	cmd.Flags().Bool("with-api", false, "enable the REST HTTP API server")
	cmd.Flags().String("api-addr", "", "REST API listen address (default :8080)")
	cmd.Flags().String("api-token", "", "master bearer token for REST API auth")

	// Metrics
	cmd.Flags().Bool("no-metrics", false, "disable the Prometheus metrics endpoint")
	cmd.Flags().String("prometheus-addr", "", "Prometheus metrics listen address (default :9090)")

	// LLM extractor
	cmd.Flags().Bool("with-llm", false, "enable Ollama-backed extraction (requires running Ollama)")
	cmd.Flags().String("llm-endpoint", "", "Ollama HTTP endpoint (default http://localhost:11434)")

	return cmd
}

// ---------------------------------------------------------------------------
// migrate
// ---------------------------------------------------------------------------

func newMigrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations and exit",
		Long:  "Initialises the SQLite schema to the latest version, then exits.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			applyStoragePath(cfg)
			application, err := app.NewApplication(cfg)
			if err != nil {
				return fmt.Errorf("migration failed: %w", err)
			}
			application.Close()
			fmt.Println("Database migrations completed successfully.")
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// doctor
// ---------------------------------------------------------------------------

func newDoctorCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check system health and print configuration summary",
		Long: `Loads the application, queries status, and prints a health report.

Use --json for machine-readable output (same data, JSON format).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			applyStoragePath(cfg)
			application, err := app.NewApplication(cfg)
			if err != nil {
				return fmt.Errorf("doctor: failed to initialise application: %w", err)
			}
			defer application.Close()

			ctx := context.Background()
			out, err := application.GetStatusUC().Execute(ctx)
			if err != nil {
				return fmt.Errorf("doctor: failed to get status: %w", err)
			}

			if asJSON {
				type doctorReport struct {
					Version     string `json:"version"`
					Uptime      string `json:"uptime"`
					ConfigFile  string `json:"config_file"`
					StoragePath string `json:"storage_path"`
					Model       string `json:"model"`
					MCPTransport string `json:"mcp_transport"`
					MCPAddress  string `json:"mcp_address,omitempty"`
					APIEnabled  bool   `json:"api_enabled"`
					APIAddress  string `json:"api_address,omitempty"`
					MetricsEnabled bool   `json:"metrics_enabled"`
					PrometheusAddr string `json:"prometheus_addr,omitempty"`
					SOULEnabled bool   `json:"soul_enabled"`
					WebhooksEnabled bool   `json:"webhooks_enabled"`
					WebhookCount int    `json:"webhook_count"`
					HNSWKeySet  bool   `json:"hnsw_key_set"`
					Stats       any    `json:"stats,omitempty"`
					Models      []string `json:"models,omitempty"`
				}
					r := doctorReport{
						Version:        miraVersion,
						Uptime:         out.Uptime,
					ConfigFile:     globalFlags.configPath,
					StoragePath:    cfg.Storage.Path,
					Model:          cfg.Embeddings.CurrentModel,
					MCPTransport:   cfg.MCP.Transport,
					SOULEnabled:    cfg.Soul.Enabled,
					WebhooksEnabled: cfg.Webhooks.Enabled,
					WebhookCount:   len(cfg.Webhooks.Endpoints),
					HNSWKeySet:     cfg.HNSW.EncryptionKey != "",
					APIEnabled:     cfg.API.Enabled,
					MetricsEnabled: cfg.Metrics.Enabled,
					Stats:          out.Stats,
					Models:         out.Models,
				}
				if cfg.MCP.Transport != "stdio" {
					r.MCPAddress = cfg.MCP.Address
				}
				if cfg.API.Enabled {
					r.APIAddress = cfg.API.Address
				}
				if cfg.Metrics.Enabled {
					r.PrometheusAddr = cfg.Metrics.PrometheusAddr
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(r)
			}

			// Human-readable output
			fmt.Printf("MIRA v%s  (uptime: %s)\n", miraVersion, out.Uptime)
			fmt.Println()
			fmt.Println("Configuration")
			fmt.Printf("  Config file     : %s\n", globalFlags.configPath)
			fmt.Printf("  Storage path    : %s\n", cfg.Storage.Path)
			fmt.Printf("  Model           : %s\n", cfg.Embeddings.CurrentModel)
			fmt.Printf("  HNSW key        : %s\n", boolPresent(cfg.HNSW.EncryptionKey != ""))
			fmt.Println()
			fmt.Println("MCP server")
			fmt.Printf("  Transport       : %s\n", cfg.MCP.Transport)
			if cfg.MCP.Transport != "stdio" {
				fmt.Printf("  Address         : %s\n", cfg.MCP.Address)
			}
			fmt.Println()
			fmt.Println("Subsystems")
			fmt.Printf("  SOUL            : %v\n", cfg.Soul.Enabled)
			fmt.Printf("  REST API        : enabled=%-5v", cfg.API.Enabled)
			if cfg.API.Enabled {
				fmt.Printf("  addr=%s", cfg.API.Address)
			}
			fmt.Println()
			fmt.Printf("  Prometheus      : enabled=%-5v", cfg.Metrics.Enabled)
			if cfg.Metrics.Enabled {
				fmt.Printf("  addr=%s", cfg.Metrics.PrometheusAddr)
			}
			fmt.Println()
			fmt.Printf("  LLM extractor   : %v\n", cfg.Extraction.LLM.Enabled)
			fmt.Printf("  Webhooks        : enabled=%-5v  endpoints=%d\n",
				cfg.Webhooks.Enabled, len(cfg.Webhooks.Endpoints))
			fmt.Println()
			if out.Stats != nil {
				s := out.Stats
				fmt.Println("Database")
				fmt.Printf("  Verbatims       : %d\n", s.VerbatimCount)
				fmt.Printf("  Fingerprints    : %d\n", s.FingerprintCount)
				fmt.Printf("  Embeddings      : %d\n", s.EmbeddingCount)
				fmt.Printf("  Causal nodes    : %d  edges: %d\n", s.CausalNodeCount, s.CausalEdgeCount)
				fmt.Printf("  Total tokens    : %d\n", s.TotalTokens)
				if len(s.ActiveWings) > 0 {
					fmt.Printf("  Wings           : %s\n", strings.Join(s.ActiveWings, ", "))
				}
			}
			fmt.Println()
			fmt.Println("Models registered")
			for _, m := range out.Models {
				fmt.Printf("  %s\n", m)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON (machine-readable)")
	return cmd
}

// ---------------------------------------------------------------------------
// status
// ---------------------------------------------------------------------------

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Print system status as JSON",
		Long: `Queries the application status and prints it as JSON to stdout.

Designed for scripting and monitoring integrations.
For a human-readable report use: mira doctor`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			applyStoragePath(cfg)
			application, err := app.NewApplication(cfg)
			if err != nil {
				return fmt.Errorf("status: failed to initialise application: %w", err)
			}
			defer application.Close()

			ctx := context.Background()
			out, err := application.GetStatusUC().Execute(ctx)
			if err != nil {
				return fmt.Errorf("status: %w", err)
			}

			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		},
	}
}

// ---------------------------------------------------------------------------
// query
// ---------------------------------------------------------------------------

func newQueryCmd() *cobra.Command {
	var (
		query  string
		wing   string
		room   string
		budget int
		asJSON bool
	)
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Run a one-shot memory recall query",
		Long: `Encodes the query, searches the vector index, and prints selected memories.

Examples:
  mira query -q "Which database was chosen?"
  mira query -q "API design decisions" --wing backend-team --budget 4000
  mira query -q "latest session notes" --wing project --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if query == "" {
				return fmt.Errorf("--query / -q is required")
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			applyStoragePath(cfg)
			application, err := app.NewApplication(cfg)
			if err != nil {
				return fmt.Errorf("query: failed to initialise application: %w", err)
			}
			defer application.Close()

			ctx := context.Background()
			input := interactors.RecallMemoryInput{
				Query:  query,
				Budget: budget,
			}
			if wing != "" {
				input.Wing = &wing
			}
			if room != "" {
				input.Room = &room
			}

			out, err := application.RecallMemoryUC().Execute(ctx, input)
			if err != nil {
				return fmt.Errorf("query failed: %w", err)
			}

			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}

			if len(out.Memories) == 0 {
				fmt.Println("(no memories found)")
				return nil
			}

			fmt.Printf("Retrieved %d memories  (budget used: %.1f%%)\n\n",
				len(out.Memories), out.BudgetUsed*100)
			for i, m := range out.Memories {
				fmt.Printf("[%d] %s\n", i+1, m.Rendered)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&query, "query", "q", "", "query text (required)")
	cmd.Flags().StringVarP(&wing, "wing", "w", "", "restrict search to this wing")
	cmd.Flags().StringVarP(&room, "room", "r", "", "restrict search to this room")
	cmd.Flags().IntVarP(&budget, "budget", "b", 2000, "token budget for recall")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}

// ---------------------------------------------------------------------------
// store
// ---------------------------------------------------------------------------

func newStoreCmd() *cobra.Command {
	var (
		content  string
		wing     string
		room     string
		memType  string
		asJSON   bool
	)
	cmd := &cobra.Command{
		Use:   "store",
		Short: "Store a single memory from the command line",
		Long: `Encodes and stores a memory without launching the MCP server.

Examples:
  mira store --content "PostgreSQL chosen for primary DB" --wing backend-team
  mira store --content "Prefer REST over gRPC for public APIs" -w arch --type decision
  mira store --content "Fixed N+1 query in user loader" -w backend-team --type fact --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if content == "" {
				return fmt.Errorf("--content / -c is required")
			}
			if wing == "" {
				return fmt.Errorf("--wing / -w is required")
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			applyStoragePath(cfg)
			application, err := app.NewApplication(cfg)
			if err != nil {
				return fmt.Errorf("store: failed to initialise application: %w", err)
			}
			defer application.Close()

			input := interactors.StoreMemoryInput{
				Content: content,
				Wing:    wing,
			}
			if room != "" {
				input.Room = &room
			}
			if memType != "" {
				mt := valueobjects.MemoryType(memType)
				if mt.IsValid() {
					input.Type = &mt
				} else {
					return fmt.Errorf("invalid --type %q; valid values: fact, decision, preference, session_note, debug_log", memType)
				}
			}

			ctx := context.Background()
			out, err := application.StoreMemoryUC().Execute(ctx, input)
			if err != nil {
				return fmt.Errorf("store failed: %w", err)
			}

			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}

			fmt.Printf("Stored  fingerprint=%s  type=%s  facts=%d  tokens=%d\n",
				out.FingerprintID, out.Type, out.FactCount, out.TokenCount)
			return nil
		},
	}
	cmd.Flags().StringVar(&content, "content", "", "memory content text (required)")
	cmd.Flags().StringVarP(&wing, "wing", "w", "", "wing (project/context namespace) (required)")
	cmd.Flags().StringVarP(&room, "room", "r", "", "optional sub-namespace within the wing")
	cmd.Flags().StringVarP(&memType, "type", "t", "", "memory type: fact, decision, preference, session_note, debug_log")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output stored fingerprint as JSON")
	return cmd
}

// ---------------------------------------------------------------------------
// delete
// ---------------------------------------------------------------------------

func newDeleteCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "delete <uuid>",
		Short: "Delete a memory by UUID",
		Long: `Permanently removes a verbatim and its vector embedding from the index.

The UUID can be found via: mira query, mira export, or GET /api/v1/timeline.

Examples:
  mira delete 5a159ddf-bc11-46a6-8a0d-f39f25853cb4
  mira delete 5a159ddf-bc11-46a6-8a0d-f39f25853cb4 --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid UUID %q: %w", args[0], err)
			}

			if !force {
				fmt.Printf("Delete memory %s? [y/N] ", id)
				var answer string
				fmt.Scanln(&answer)
				if strings.ToLower(strings.TrimSpace(answer)) != "y" {
					fmt.Println("Aborted.")
					return nil
				}
			}

			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			applyStoragePath(cfg)
			application, err := app.NewApplication(cfg)
			if err != nil {
				return fmt.Errorf("delete: failed to initialise application: %w", err)
			}
			defer application.Close()

			ctx := context.Background()
			if err := application.DeleteMemoryUC().Execute(ctx, interactors.DeleteMemoryInput{ID: id}); err != nil {
				return fmt.Errorf("delete failed: %w", err)
			}
			fmt.Printf("Deleted %s\n", id)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "skip confirmation prompt")
	return cmd
}

// ---------------------------------------------------------------------------
// export
// ---------------------------------------------------------------------------

// exportRecord is the JSON shape for one exported memory.
type exportRecord struct {
	ID        string  `json:"id"`
	Content   string  `json:"content"`
	Wing      string  `json:"wing"`
	Room      *string `json:"room,omitempty"`
	Type      string  `json:"type"`
	CreatedAt string  `json:"created_at"`
}

func newExportCmd() *cobra.Command {
	var (
		wing   string
		output string
		limit  int
	)
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export memories to a JSON file",
		Long: `Fetches verbatims via the timeline and writes them as a JSON array.

Output goes to stdout by default; use --output to write to a file.

Examples:
  mira export                                  # all wings, stdout
  mira export --wing backend-team              # filter by wing
  mira export --output memories.json           # write to file
  mira export --wing backend-team -o out.json  # combined
  mira export --limit 500                      # cap at 500 records`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			applyStoragePath(cfg)
			application, err := app.NewApplication(cfg)
			if err != nil {
				return fmt.Errorf("export: failed to initialise application: %w", err)
			}
			defer application.Close()

			ctx := context.Background()

			tlOut, err := application.GetTimelineUC().Execute(ctx, interactors.GetTimelineInput{
				Wing:  wing,
				Limit: limit,
			})
			if err != nil {
				return fmt.Errorf("export: timeline error: %w", err)
			}

			records := make([]exportRecord, 0, len(tlOut.Items))
			loadUC := application.LoadMemoryUC()
			for _, item := range tlOut.Items {
				id, parseErr := uuid.Parse(item.ID)
				if parseErr != nil {
					log.Printf("export: skip invalid ID %q: %v", item.ID, parseErr)
					continue
				}
				loaded, loadErr := loadUC.Execute(ctx, interactors.LoadMemoryInput{ID: id})
				if loadErr != nil || loaded.Verbatim == nil {
					log.Printf("export: skip %s: %v", item.ID, loadErr)
					continue
				}
				v := loaded.Verbatim
				records = append(records, exportRecord{
					ID:        v.ID.String(),
					Content:   v.Content,
					Wing:      v.Wing,
					Room:      v.Room,
					Type:      string(item.Type),
					CreatedAt: v.CreatedAt.Format(time.RFC3339),
				})
			}

			data, marshalErr := json.MarshalIndent(records, "", "  ")
			if marshalErr != nil {
				return fmt.Errorf("export: JSON marshal error: %w", marshalErr)
			}

			if output == "" || output == "-" {
				fmt.Println(string(data))
				return nil
			}
			if writeErr := os.WriteFile(output, data, 0o644); writeErr != nil {
				return fmt.Errorf("export: write error: %w", writeErr)
			}
			fmt.Printf("Exported %d records to %s\n", len(records), output)
			return nil
		},
	}
	cmd.Flags().StringVarP(&wing, "wing", "w", "", "wing to export (empty = all wings)")
	cmd.Flags().StringVarP(&output, "output", "o", "-", "output file path (- for stdout)")
	cmd.Flags().IntVarP(&limit, "limit", "n", 10000, "maximum number of records to export")
	return cmd
}

// ---------------------------------------------------------------------------
// import
// ---------------------------------------------------------------------------

// importRecord mirrors exportRecord for round-trippable JSON import.
type importRecord struct {
	Content string  `json:"content"`
	Wing    string  `json:"wing"`
	Room    *string `json:"room,omitempty"`
	Type    string  `json:"type,omitempty"`
}

func newImportCmd() *cobra.Command {
	var (
		file    string
		wing    string
		dryRun  bool
	)
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import memories from a JSON file",
		Long: `Reads a JSON array of {content, wing, room, type} records and stores each.

The file format matches the output of: mira export

Examples:
  mira import --file memories.json
  mira import -f memories.json --wing project-x       # override all wings
  mira import -f memories.json --dry-run              # validate only`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("--file / -f is required")
			}

			data, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("import: cannot read file %q: %w", file, err)
			}
			var records []importRecord
			if unmarshalErr := json.Unmarshal(data, &records); unmarshalErr != nil {
				return fmt.Errorf("import: JSON parse error: %w", unmarshalErr)
			}

			if dryRun {
				fmt.Printf("Dry-run: %d records in %s — no data written.\n", len(records), file)
				for i, rec := range records {
					w := rec.Wing
					if wing != "" {
						w = wing
					}
					if w == "" {
						w = "imported"
					}
					fmt.Printf("  [%d] wing=%-20s type=%-12s len=%d\n",
						i+1, w, rec.Type, len(rec.Content))
				}
				return nil
			}

			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			applyStoragePath(cfg)
			application, err := app.NewApplication(cfg)
			if err != nil {
				return fmt.Errorf("import: failed to initialise application: %w", err)
			}
			defer application.Close()

			ctx := context.Background()
			storeUC := application.StoreMemoryUC()
			var imported, failed int

			for _, rec := range records {
				w := rec.Wing
				if wing != "" {
					w = wing
				}
				if w == "" {
					w = "imported"
				}

				input := interactors.StoreMemoryInput{
					Content: rec.Content,
					Wing:    w,
					Room:    rec.Room,
				}
				if rec.Type != "" {
					mt := valueobjects.MemoryType(rec.Type)
					if mt.IsValid() {
						input.Type = &mt
					}
				}

				if _, storeErr := storeUC.Execute(ctx, input); storeErr != nil {
					log.Printf("import: failed to store record: %v", storeErr)
					failed++
					continue
				}
				imported++
			}

			fmt.Printf("Import complete: %d stored, %d failed (total: %d)\n",
				imported, failed, len(records))
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "path to JSON file to import (required)")
	cmd.Flags().StringVarP(&wing, "wing", "w", "", "override wing for all imported records")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate and preview without writing")
	return cmd
}

// ---------------------------------------------------------------------------
// config
// ---------------------------------------------------------------------------

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Configuration utilities",
		Long: `Utilities for inspecting and validating MIRA configuration.

Sub-commands:
  validate   Load and validate config, report errors
  show       Print the effective resolved configuration`,
	}
	cmd.AddCommand(newConfigValidateCmd(), newConfigShowCmd())
	return cmd
}

func newConfigValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate the configuration file",
		Long: `Loads and validates the configuration file without starting the application.

Exits 0 if valid, 1 if there are errors.

Examples:
  mira config validate
  mira --config /etc/mira/config.yaml config validate`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := globalFlags.configPath
			if _, err := os.Stat(path); os.IsNotExist(err) {
				fmt.Printf("Config file not found at %q — would use built-in defaults.\n", path)
				return nil
			}
			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("config validation failed: %w", err)
			}
			fmt.Printf("Config OK: %s\n", path)
			fmt.Printf("  storage path : %s\n", cfg.Storage.Path)
			fmt.Printf("  model        : %s\n", cfg.Embeddings.CurrentModel)
			fmt.Printf("  mcp transport: %s\n", cfg.MCP.Transport)
			return nil
		},
	}
}

func newConfigShowCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Print the effective resolved configuration",
		Long: `Loads the configuration (file + env overrides) and prints it.

Sensitive fields (auth_token, wing_tokens, encryption_key) are redacted.

Examples:
  mira config show
  mira config show --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			applyStoragePath(cfg)

			// Redact sensitive fields
			if cfg.API.AuthToken != "" {
				cfg.API.AuthToken = "[redacted]"
			}
			if cfg.HNSW.EncryptionKey != "" {
				cfg.HNSW.EncryptionKey = "[redacted]"
			}
			for k := range cfg.API.WingTokens {
				cfg.API.WingTokens[k] = []string{"[redacted]"}
			}

			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(cfg)
			}

			data, err := yaml.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("config show: marshal error: %w", err)
			}
			fmt.Print(string(data))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON instead of YAML")
	return cmd
}

// ---------------------------------------------------------------------------
// optimize
// ---------------------------------------------------------------------------

// optimizeMessage mirrors the wire shape of a single chat message in the
// --file input/output (OpenAI/Anthropic/Mistral-style {"role","content"}).
type optimizeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func newOptimizeCmd() *cobra.Command {
	var (
		file    string
		budget  int
		keepLastN int
		output  string
		statsOnly bool
	)
	cmd := &cobra.Command{
		Use:   "optimize",
		Short: "Prune a chat history file to fit a token budget (no LLM calls)",
		Long: `Runs MIRA's deterministic O(n log n) CBA-lite optimizer on a JSON chat
history and prints the pruned message array plus token-savings stats.

This does not require a running MIRA server, storage, or an LLM call: it
works purely on the "messages" array supplied in --file, the same mechanism
used by MIRA Proxy. Input is a JSON array of {"role","content"} objects.

Examples:
  mira optimize --file history.json
  mira optimize --file history.json --budget 2000 --keep-last 6
  mira optimize --file history.json --stats-only
  mira optimize --file history.json --output pruned.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("--file is required")
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("optimize: failed to read %q: %w", file, err)
			}

			var messages []optimizeMessage
			if err := json.Unmarshal(raw, &messages); err != nil {
				return fmt.Errorf("optimize: invalid JSON array in %q: %w", file, err)
			}

			input := interactors.OptimizeContextInput{
				BudgetTokens: budget,
				KeepLastN:    keepLastN,
			}
			for _, m := range messages {
				input.Messages = append(input.Messages, interactors.ContextMessage{Role: m.Role, Content: m.Content})
			}

			out := interactors.NewOptimizeContext().Execute(input)

			fmt.Fprintf(os.Stderr, "original tokens:  %d\n", out.OriginalTokens)
			fmt.Fprintf(os.Stderr, "optimized tokens: %d\n", out.OptimizedTokens)
			fmt.Fprintf(os.Stderr, "tokens saved:     %d (%.1f%%)\n", out.TokensSaved, savingsPercent(out.OriginalTokens, out.TokensSaved))
			fmt.Fprintf(os.Stderr, "messages dropped: %d / %d\n", out.Dropped, len(messages))

			if statsOnly {
				return nil
			}

			pruned := make([]optimizeMessage, 0, len(out.Messages))
			for _, m := range out.Messages {
				pruned = append(pruned, optimizeMessage{Role: m.Role, Content: m.Content})
			}
			data, err := json.MarshalIndent(pruned, "", "  ")
			if err != nil {
				return fmt.Errorf("optimize: JSON marshal error: %w", err)
			}

			if output == "" || output == "-" {
				fmt.Println(string(data))
				return nil
			}
			if err := os.WriteFile(output, data, 0o644); err != nil {
				return fmt.Errorf("optimize: write error: %w", err)
			}
			fmt.Fprintf(os.Stderr, "wrote pruned conversation to %s\n", output)
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "path to a JSON array of {role,content} messages (required)")
	cmd.Flags().IntVarP(&budget, "budget", "b", interactors.DefaultOptimizeBudgetTokens, "token budget for the pruned conversation")
	cmd.Flags().IntVar(&keepLastN, "keep-last", interactors.DefaultOptimizeKeepLastN, "number of most recent messages always kept verbatim")
	cmd.Flags().StringVarP(&output, "output", "o", "-", "output file path for the pruned JSON (- for stdout)")
	cmd.Flags().BoolVar(&statsOnly, "stats-only", false, "print only the token-savings stats, not the pruned messages")
	return cmd
}

func savingsPercent(original, saved int) float64 {
	if original <= 0 {
		return 0
	}
	return float64(saved) / float64(original) * 100
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func loadConfig() (*config.Config, error) {
	cfg, err := config.LoadOrDefault(globalFlags.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from %q: %w", globalFlags.configPath, err)
	}
	return cfg, nil
}

// applyStoragePath applies the --storage-path global flag to the config if set.
func applyStoragePath(cfg *config.Config) {
	if globalFlags.storagePath != "" {
		cfg.Storage.Path = globalFlags.storagePath
	}
}

// boolPresent returns "set" or "unset" for display purposes.
func boolPresent(v bool) string {
	if v {
		return "set"
	}
	return "unset"
}
