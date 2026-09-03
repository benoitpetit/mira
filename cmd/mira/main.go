// MIRA - Memory with Information-theoretic Relevance Allocation
// Command-line interface using cobra
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
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

const miraVersion = "0.5.0"

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
  mira init                                      # Initialise a project-local store
  mira start                                     # Start MCP server
  mira server --with-api --api-addr :8080      # With REST API
  mira doctor                                   # Health check
  mira query -q "search term" --wing my-wing   # Recall memories

For detailed help on any command: mira <command> --help`

func main() {
	root := &cobra.Command{
		Use:     "mira",
		Short:   "MIRA – Memory with Information-theoretic Relevance Allocation",
		Long:    rootLong,
		Version: miraVersion,
	}

	// Persistent flags shared by all subcommands.
	root.PersistentFlags().StringVarP(&globalFlags.configPath, "config", "c",
		config.ResolveConfigPath(""), "path to configuration file")
	root.PersistentFlags().StringVar(&globalFlags.storagePath, "storage-path", "",
		"override storage data directory (also: MIRA_DATA_PATH env)")

	root.AddCommand(
		newInitCmd(),
		newSetupCmd(),
		newServerCmd(),
		newMigrateCmd(),
		newDoctorCmd(),
		newStatusCmd(),
		newQueryCmd(),
		newStoreCmd(),
		newDeleteCmd(),
		newExportCmd(),
		newImportCmd(),
		newIngestCmd(),
		newHookCmd(),
		newConfigCmd(),
		newOptimizeCmd(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// setup
// ---------------------------------------------------------------------------

// codexSetupArgs returns the official Codex CLI invocation for registering the
// local MIRA stdio server. Keeping this separate makes the command easy to
// inspect and test without modifying a user's Codex configuration.
func codexSetupArgs(binaryPath, configPath string) []string {
	return []string{"mcp", "add", "mira", "--", binaryPath, "--config", configPath, "server"}
}

// claudeCodeSetupArgs follows the official Claude Code MCP CLI syntax. Scope
// is local by default so a one-click setup stays private to the current project.
func claudeCodeSetupArgs(binaryPath, configPath, scope string) []string {
	return []string{"mcp", "add", "mira", "--scope", scope, "--", binaryPath, "--config", configPath, "server"}
}

type mcpStdioServerConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type mcpConfigFile struct {
	MCPServers map[string]mcpStdioServerConfig `json:"mcpServers"`
}

func cursorMCPConfigPath(miraConfigPath string) string {
	return filepath.Join(filepath.Dir(filepath.Dir(miraConfigPath)), ".cursor", "mcp.json")
}

func windsurfMCPConfigPath(homeDir string) string {
	return filepath.Join(homeDir, ".codeium", "windsurf", "mcp_config.json")
}

func windsurfHooksConfigPath(homeDir string) string {
	return filepath.Join(homeDir, ".codeium", "windsurf", "hooks.json")
}

// claudeDesktopMCPConfigPath returns the official Claude Desktop configuration
// location on platforms with a documented native location. An explicit
// --client-config can still be used on other platforms.
func claudeDesktopMCPConfigPath(goos, homeDir, appData string) (string, error) {
	switch goos {
	case "darwin":
		return filepath.Join(homeDir, "Library", "Application Support", "Claude", "claude_desktop_config.json"), nil
	case "windows":
		if appData == "" {
			return "", fmt.Errorf("setup: APPDATA is required to locate the Claude Desktop configuration")
		}
		return filepath.Join(appData, "Claude", "claude_desktop_config.json"), nil
	default:
		return "", fmt.Errorf("setup: Claude Desktop has no documented configuration location on %s; pass --client-config explicitly", goos)
	}
}

func readMCPConfig(path, client string) (mcpConfigFile, error) {
	config := mcpConfigFile{MCPServers: make(map[string]mcpStdioServerConfig)}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return config, nil
	}
	if err != nil {
		return config, fmt.Errorf("setup: read %s config %q: %w", client, path, err)
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return config, fmt.Errorf("setup: %s config %q is not valid JSON: %w", client, path, err)
	}
	if config.MCPServers == nil {
		config.MCPServers = make(map[string]mcpStdioServerConfig)
	}
	return config, nil
}

func configureMCPConfig(path, client, binaryPath, miraConfigPath string, force bool) ([]byte, error) {
	config, err := readMCPConfig(path, client)
	if err != nil {
		return nil, err
	}
	if existing, ok := config.MCPServers["mira"]; ok && !force {
		wanted := mcpStdioServerConfig{Command: binaryPath, Args: []string{"--config", miraConfigPath, "server"}}
		if existing.Command != wanted.Command || !slices.Equal(existing.Args, wanted.Args) {
			return nil, fmt.Errorf("setup: %s already has a different MIRA server in %q; use --force to replace it", client, path)
		}
	}
	config.MCPServers["mira"] = mcpStdioServerConfig{
		Command: binaryPath,
		Args:    []string{"--config", miraConfigPath, "server"},
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("setup: encode %s config: %w", client, err)
	}
	return append(data, '\n'), nil
}

func configureCursorMCP(path, binaryPath, miraConfigPath string, force bool) ([]byte, error) {
	return configureMCPConfig(path, "Cursor", binaryPath, miraConfigPath, force)
}

func configureWindsurfMCP(path, binaryPath, miraConfigPath string, force bool) ([]byte, error) {
	return configureMCPConfig(path, "Windsurf", binaryPath, miraConfigPath, force)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\\"'\\\"'") + "'"
}

func claudeCodeMemoryHookCommand(binaryPath, miraConfigPath, wing string) string {
	return strings.Join([]string{
		shellQuote(binaryPath), "--config", shellQuote(miraConfigPath),
		"hook", "claude-code", "--wing", shellQuote(wing),
	}, " ")
}

func codexMemoryHookCommand(binaryPath, miraConfigPath, wing string) string {
	return strings.Join([]string{
		shellQuote(binaryPath), "--config", shellQuote(miraConfigPath),
		"hook", "codex", "--wing", shellQuote(wing),
	}, " ")
}

func codexHooksConfigPath(homeDir string) string {
	return filepath.Join(homeDir, ".codex", "hooks.json")
}

type memoryHookSpec struct {
	event   string
	command string
}

// configureMemoryHooks merges non-blocking memory hooks while preserving all
// existing settings. An identical MIRA hook is never duplicated.
func configureMemoryHooks(path, client string, specs ...memoryHookSpec) ([]byte, error) {
	settings := make(map[string]any)
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("setup: read %s hook settings %q: %w", client, path, err)
	}
	if err == nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return nil, fmt.Errorf("setup: %s hook settings %q are not valid JSON: %w", client, path, err)
		}
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		if _, exists := settings["hooks"]; exists {
			return nil, fmt.Errorf("setup: %s hooks in %q must be a JSON object", client, path)
		}
		hooks = make(map[string]any)
		settings["hooks"] = hooks
	}
	for _, spec := range specs {
		events, ok := hooks[spec.event].([]any)
		if !ok {
			if _, exists := hooks[spec.event]; exists {
				return nil, fmt.Errorf("setup: %s %s hooks in %q must be a JSON array", client, spec.event, path)
			}
			events = make([]any, 0, 1)
		}
		duplicate := false
		for _, event := range events {
			group, ok := event.(map[string]any)
			if !ok {
				continue
			}
			entries, _ := group["hooks"].([]any)
			for _, entry := range entries {
				hook, ok := entry.(map[string]any)
				if ok && hook["type"] == "command" && hook["command"] == spec.command {
					duplicate = true
					break
				}
			}
			if duplicate {
				break
			}
		}
		if !duplicate {
			hooks[spec.event] = append(events, map[string]any{
				"matcher": "",
				"hooks": []any{map[string]any{
					"type":    "command",
					"command": spec.command,
				}},
			})
		}
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("setup: encode %s hook settings: %w", client, err)
	}
	return append(data, '\n'), nil
}

func configureClaudeCodeMemoryHook(path, binaryPath, miraConfigPath, wing string) ([]byte, error) {
	return configureClaudeCodeMemoryHooks(path, binaryPath, miraConfigPath, wing, false)
}

func configureCodexMemoryHook(path, binaryPath, miraConfigPath, wing string) ([]byte, error) {
	return configureCodexMemoryHooks(path, binaryPath, miraConfigPath, wing, false)
}

func configureClaudeCodeMemoryHooks(path, binaryPath, miraConfigPath, wing string, includeAssistant bool) ([]byte, error) {
	command := claudeCodeMemoryHookCommand(binaryPath, miraConfigPath, wing)
	specs := []memoryHookSpec{{event: "UserPromptSubmit", command: command}}
	if includeAssistant {
		specs = append(specs, memoryHookSpec{event: "Stop", command: command})
	}
	return configureMemoryHooks(path, "Claude Code", specs...)
}

func configureCodexMemoryHooks(path, binaryPath, miraConfigPath, wing string, includeAssistant bool) ([]byte, error) {
	command := codexMemoryHookCommand(binaryPath, miraConfigPath, wing)
	specs := []memoryHookSpec{{event: "UserPromptSubmit", command: command}}
	if includeAssistant {
		specs = append(specs, memoryHookSpec{event: "Stop", command: command})
	}
	return configureMemoryHooks(path, "Codex", specs...)
}

func automaticMemoryCaptureDescription(includeAssistant bool) string {
	if includeAssistant {
		return "Automatic user-prompt and completed assistant-response memory capture"
	}
	return "Automatic user-prompt memory capture"
}

func windsurfMemoryHookCommand(binaryPath, miraConfigPath, wing string) string {
	return strings.Join([]string{
		shellQuote(binaryPath), "--config", shellQuote(miraConfigPath),
		"hook", "windsurf", "--wing", shellQuote(wing),
	}, " ")
}

func configureWindsurfMemoryHooks(path, binaryPath, miraConfigPath, wing string, includeAssistant bool) ([]byte, error) {
	settings := make(map[string]any)
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("setup: read Windsurf hook settings %q: %w", path, err)
	}
	if err == nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return nil, fmt.Errorf("setup: Windsurf hook settings %q are not valid JSON: %w", path, err)
		}
	}
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		if _, exists := settings["hooks"]; exists {
			return nil, fmt.Errorf("setup: Windsurf hooks in %q must be a JSON object", path)
		}
		hooks = make(map[string]any)
		settings["hooks"] = hooks
	}
	command := windsurfMemoryHookCommand(binaryPath, miraConfigPath, wing)
	addHook := func(event string) error {
		entries, ok := hooks[event].([]any)
		if !ok {
			if _, exists := hooks[event]; exists {
				return fmt.Errorf("setup: Windsurf %s hooks in %q must be a JSON array", event, path)
			}
			entries = make([]any, 0, 1)
		}
		for _, entry := range entries {
			hook, ok := entry.(map[string]any)
			if ok && hook["command"] == command {
				return nil
			}
		}
		hooks[event] = append(entries, map[string]any{"command": command})
		return nil
	}
	if err := addHook("pre_user_prompt"); err != nil {
		return nil, err
	}
	if includeAssistant {
		if err := addHook("post_cascade_response"); err != nil {
			return nil, err
		}
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("setup: encode Windsurf hook settings: %w", err)
	}
	return append(data, '\n'), nil
}

func newSetupCmd() *cobra.Command {
	var (
		client           string
		miraConfig       string
		binaryPath       string
		clientConfig     string
		dryRun           bool
		scope            string
		force            bool
		automaticMemory  bool
		memoryWing       string
		includeAssistant bool
		hookConfig       string
	)

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure MIRA in a supported MCP client",
		Long: `Registers MIRA as a local stdio MCP server in a supported client.

Codex and Claude Code are configured through their respective official CLI
commands. Cursor, Windsurf and Claude Desktop use a protected JSON merge that
preserves other MCP servers. MIRA project configuration must already exist;
create it first with mira init.

Examples:
  mira init
  mira setup --client codex
  mira setup --client codex --dry-run
  mira setup --client claude-code --scope local
	  mira setup --client claude-code --automatic-memory --memory-wing api
  mira setup --client cursor
	  mira setup --client windsurf
	  mira setup --client claude-desktop
  mira setup --client codex --mira-config /path/to/project/.mira/config.yaml`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if client != "codex" && client != "claude-code" && client != "cursor" && client != "windsurf" && client != "claude-desktop" {
				return fmt.Errorf("unsupported client %q; supported: codex, claude-code, cursor, windsurf, claude-desktop", client)
			}
			if client == "claude-code" && scope != "local" && scope != "project" && scope != "user" {
				return fmt.Errorf("invalid --scope %q; supported values: local, project, user", scope)
			}
			if automaticMemory && client != "claude-code" && client != "codex" && client != "windsurf" {
				return fmt.Errorf("--automatic-memory is currently supported only with --client codex, claude-code, or windsurf")
			}
			if includeAssistant && !(automaticMemory && (client == "codex" || client == "claude-code" || client == "windsurf")) {
				return fmt.Errorf("--include-assistant requires --automatic-memory with codex, claude-code, or windsurf")
			}
			if automaticMemory && (!interactors.WingRoomRe.MatchString(memoryWing) || len([]rune(memoryWing)) > 100) {
				return fmt.Errorf("--memory-wing must be 1-100 alphanumeric characters, hyphens or underscores")
			}

			configPath, err := filepath.Abs(miraConfig)
			if err != nil {
				return fmt.Errorf("setup: resolve MIRA config: %w", err)
			}
			if info, err := os.Stat(configPath); err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("setup: MIRA config not found at %q; run `mira init` first or pass --mira-config", configPath)
				}
				return fmt.Errorf("setup: inspect MIRA config: %w", err)
			} else if info.IsDir() {
				return fmt.Errorf("setup: MIRA config %q is a directory", configPath)
			}

			if binaryPath == "" {
				binaryPath, err = os.Executable()
				if err != nil {
					return fmt.Errorf("setup: resolve MIRA executable: %w", err)
				}
			}
			binaryPath, err = filepath.Abs(binaryPath)
			if err != nil {
				return fmt.Errorf("setup: resolve MIRA executable: %w", err)
			}
			if info, err := os.Stat(binaryPath); err != nil {
				return fmt.Errorf("setup: MIRA executable %q: %w", binaryPath, err)
			} else if info.IsDir() {
				return fmt.Errorf("setup: MIRA executable %q is a directory", binaryPath)
			}

			switch client {
			case "codex":
				setupArgs := codexSetupArgs(binaryPath, configPath)
				if dryRun {
					fmt.Printf("Would run: codex %s\n", strings.Join(setupArgs, " "))
					if automaticMemory {
						hookPath := clientConfig
						if hookPath == "" {
							homeDir, err := os.UserHomeDir()
							if err != nil {
								return fmt.Errorf("setup: resolve home directory for Codex hooks: %w", err)
							}
							hookPath = codexHooksConfigPath(homeDir)
						}
						data, err := configureCodexMemoryHooks(hookPath, binaryPath, configPath, memoryWing, includeAssistant)
						if err != nil {
							return err
						}
						fmt.Printf("Would write %s:\n%s", hookPath, data)
					}
					return nil
				}
				if _, err := exec.LookPath("codex"); err != nil {
					return fmt.Errorf("setup: Codex CLI is not available on PATH: %w", err)
				}
				command := exec.Command("codex", setupArgs...)
				command.Stdout = os.Stdout
				command.Stderr = os.Stderr
				if err := command.Run(); err != nil {
					return fmt.Errorf("setup: Codex MCP registration failed: %w", err)
				}
				if automaticMemory {
					hookPath := clientConfig
					if hookPath == "" {
						homeDir, err := os.UserHomeDir()
						if err != nil {
							return fmt.Errorf("setup: resolve home directory for Codex hooks: %w", err)
						}
						hookPath = codexHooksConfigPath(homeDir)
					}
					data, err := configureCodexMemoryHooks(hookPath, binaryPath, configPath, memoryWing, includeAssistant)
					if err != nil {
						return err
					}
					if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
						return fmt.Errorf("setup: create Codex hook directory: %w", err)
					}
					if err := os.WriteFile(hookPath, data, 0o600); err != nil {
						return fmt.Errorf("setup: write Codex hook settings: %w", err)
					}
					fmt.Printf("%s is configured in %s. Codex will ask you to trust the new hook.\n", automaticMemoryCaptureDescription(includeAssistant), hookPath)
				}
				fmt.Println("MIRA is configured for Codex. Restart Codex if it is already running.")
				return nil
			case "claude-code":
				setupArgs := claudeCodeSetupArgs(binaryPath, configPath, scope)
				if dryRun {
					fmt.Printf("Would run: claude %s\n", strings.Join(setupArgs, " "))
					if automaticMemory {
						hookPath := filepath.Join(filepath.Dir(filepath.Dir(configPath)), ".claude", "settings.local.json")
						data, err := configureClaudeCodeMemoryHooks(hookPath, binaryPath, configPath, memoryWing, includeAssistant)
						if err != nil {
							return err
						}
						fmt.Printf("Would write %s:\n%s", hookPath, data)
					}
					return nil
				}
				if _, err := exec.LookPath("claude"); err != nil {
					return fmt.Errorf("setup: Claude Code CLI is not available on PATH: %w", err)
				}
				command := exec.Command("claude", setupArgs...)
				command.Stdout = os.Stdout
				command.Stderr = os.Stderr
				if err := command.Run(); err != nil {
					return fmt.Errorf("setup: Claude Code MCP registration failed: %w", err)
				}
				if automaticMemory {
					hookPath := filepath.Join(filepath.Dir(filepath.Dir(configPath)), ".claude", "settings.local.json")
					data, err := configureClaudeCodeMemoryHooks(hookPath, binaryPath, configPath, memoryWing, includeAssistant)
					if err != nil {
						return err
					}
					if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
						return fmt.Errorf("setup: create Claude Code hook directory: %w", err)
					}
					if err := os.WriteFile(hookPath, data, 0o600); err != nil {
						return fmt.Errorf("setup: write Claude Code hook settings: %w", err)
					}
					fmt.Printf("%s is configured in %s.\n", automaticMemoryCaptureDescription(includeAssistant), hookPath)
				}
				fmt.Printf("MIRA is configured for Claude Code (%s scope). Restart Claude Code if it is already running.\n", scope)
				return nil
			case "cursor":
				cursorPath := clientConfig
				if cursorPath == "" {
					cursorPath = cursorMCPConfigPath(configPath)
				}
				data, err := configureCursorMCP(cursorPath, binaryPath, configPath, force)
				if err != nil {
					return err
				}
				if dryRun {
					fmt.Printf("Would write %s:\n%s", cursorPath, data)
					return nil
				}
				if err := os.MkdirAll(filepath.Dir(cursorPath), 0o755); err != nil {
					return fmt.Errorf("setup: create Cursor config directory: %w", err)
				}
				if err := os.WriteFile(cursorPath, data, 0o600); err != nil {
					return fmt.Errorf("setup: write Cursor config: %w", err)
				}
				fmt.Printf("MIRA is configured for Cursor in %s. Restart Cursor if it is already running.\n", cursorPath)
				return nil
			case "windsurf":
				windsurfPath := clientConfig
				if windsurfPath == "" {
					homeDir, err := os.UserHomeDir()
					if err != nil {
						return fmt.Errorf("setup: resolve home directory for Windsurf: %w", err)
					}
					windsurfPath = windsurfMCPConfigPath(homeDir)
				}
				data, err := configureWindsurfMCP(windsurfPath, binaryPath, configPath, force)
				if err != nil {
					return err
				}
				if dryRun {
					fmt.Printf("Would write %s:\n%s", windsurfPath, data)
					if automaticMemory {
						hookPath := hookConfig
						if hookPath == "" {
							homeDir, err := os.UserHomeDir()
							if err != nil {
								return fmt.Errorf("setup: resolve home directory for Windsurf hooks: %w", err)
							}
							hookPath = windsurfHooksConfigPath(homeDir)
						}
						hookData, err := configureWindsurfMemoryHooks(hookPath, binaryPath, configPath, memoryWing, includeAssistant)
						if err != nil {
							return err
						}
						fmt.Printf("Would write %s:\n%s", hookPath, hookData)
					}
					return nil
				}
				if err := os.MkdirAll(filepath.Dir(windsurfPath), 0o755); err != nil {
					return fmt.Errorf("setup: create Windsurf config directory: %w", err)
				}
				if err := os.WriteFile(windsurfPath, data, 0o600); err != nil {
					return fmt.Errorf("setup: write Windsurf config: %w", err)
				}
				if automaticMemory {
					hookPath := hookConfig
					if hookPath == "" {
						homeDir, err := os.UserHomeDir()
						if err != nil {
							return fmt.Errorf("setup: resolve home directory for Windsurf hooks: %w", err)
						}
						hookPath = windsurfHooksConfigPath(homeDir)
					}
					hookData, err := configureWindsurfMemoryHooks(hookPath, binaryPath, configPath, memoryWing, includeAssistant)
					if err != nil {
						return err
					}
					if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
						return fmt.Errorf("setup: create Windsurf hook directory: %w", err)
					}
					if err := os.WriteFile(hookPath, hookData, 0o600); err != nil {
						return fmt.Errorf("setup: write Windsurf hook settings: %w", err)
					}
					fmt.Printf("%s is configured in %s.\n", automaticMemoryCaptureDescription(includeAssistant), hookPath)
				}
				fmt.Printf("MIRA is configured for Windsurf in %s. Restart Windsurf if it is already running.\n", windsurfPath)
				return nil
			case "claude-desktop":
				claudePath := clientConfig
				if claudePath == "" {
					homeDir, err := os.UserHomeDir()
					if err != nil {
						return fmt.Errorf("setup: resolve home directory for Claude Desktop: %w", err)
					}
					claudePath, err = claudeDesktopMCPConfigPath(runtime.GOOS, homeDir, os.Getenv("APPDATA"))
					if err != nil {
						return err
					}
				}
				data, err := configureMCPConfig(claudePath, "Claude Desktop", binaryPath, configPath, force)
				if err != nil {
					return err
				}
				if dryRun {
					fmt.Printf("Would write %s:\n%s", claudePath, data)
					return nil
				}
				if err := os.MkdirAll(filepath.Dir(claudePath), 0o755); err != nil {
					return fmt.Errorf("setup: create Claude Desktop config directory: %w", err)
				}
				if err := os.WriteFile(claudePath, data, 0o600); err != nil {
					return fmt.Errorf("setup: write Claude Desktop config: %w", err)
				}
				fmt.Printf("MIRA is configured for Claude Desktop in %s. Fully quit and restart Claude Desktop to apply it.\n", claudePath)
				return nil
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&client, "client", "codex", "MCP client to configure: codex, claude-code, cursor, windsurf, claude-desktop")
	cmd.Flags().StringVar(&miraConfig, "mira-config", ".mira/config.yaml", "path to the MIRA project configuration")
	cmd.Flags().StringVar(&binaryPath, "mira-binary", "", "path to the MIRA executable (default: current executable)")
	cmd.Flags().StringVar(&clientConfig, "client-config", "", "override the target client JSON configuration path (or Codex hook path with --automatic-memory)")
	cmd.Flags().StringVar(&scope, "scope", "local", "Claude Code scope: local, project, or user")
	cmd.Flags().BoolVar(&force, "force", false, "replace an existing, different Cursor or Windsurf MIRA configuration")
	cmd.Flags().BoolVar(&automaticMemory, "automatic-memory", false, "with a supported client, store substantive user prompts through a local hook")
	cmd.Flags().StringVar(&memoryWing, "memory-wing", "project", "wing used by the automatic-memory hook")
	cmd.Flags().BoolVar(&includeAssistant, "include-assistant", false, "with automatic memory, also store completed assistant responses when the client exposes them")
	cmd.Flags().StringVar(&hookConfig, "hook-config", "", "override the automatic-memory hook configuration path")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the planned setup without changing configuration")
	return cmd
}

// ---------------------------------------------------------------------------
// init
// ---------------------------------------------------------------------------

func newInitCmd() *cobra.Command {
	var (
		dir   string
		force bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialise a project-local MIRA store",
		Long: `Creates .mira/config.yaml with safe local defaults and a .mira data directory.

The generated configuration uses an absolute storage path so it continues to
refer to this project even when the server is launched from an MCP client with
a different working directory. Existing configuration is never overwritten
unless --force is supplied.

Examples:
  mira init
  mira init --dir ../my-project
  mira --config .mira/config.yaml doctor
  mira --config .mira/config.yaml start`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir, err := filepath.Abs(dir)
			if err != nil {
				return fmt.Errorf("init: resolve project directory: %w", err)
			}
			info, err := os.Stat(projectDir)
			if err != nil {
				return fmt.Errorf("init: project directory %q: %w", projectDir, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("init: %q is not a directory", projectDir)
			}

			dataDir := filepath.Join(projectDir, ".mira")
			configPath := filepath.Join(dataDir, "config.yaml")
			if _, err := os.Stat(configPath); err == nil && !force {
				return fmt.Errorf("init: configuration already exists at %q (use --force to replace it)", configPath)
			} else if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("init: inspect configuration: %w", err)
			}

			if err := os.MkdirAll(dataDir, 0o755); err != nil {
				return fmt.Errorf("init: create data directory: %w", err)
			}

			cfg := config.Default()
			cfg.Storage.Path = dataDir
			if err := cfg.Save(configPath); err != nil {
				return fmt.Errorf("init: write configuration: %w", err)
			}

			fmt.Printf("MIRA initialised for %s\n", projectDir)
			fmt.Printf("  Configuration : %s\n", configPath)
			fmt.Printf("  Data directory: %s\n", dataDir)
			fmt.Printf("\nNext steps:\n  mira --config %s doctor\n  mira --config %s start\n", configPath, configPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "project directory to initialise")
	cmd.Flags().BoolVar(&force, "force", false, "replace an existing .mira/config.yaml")
	return cmd
}

// ---------------------------------------------------------------------------
// server
// ---------------------------------------------------------------------------

func newServerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "server",
		Aliases: []string{"start"},
		Short:   "Start the MCP memory server",
		Long: `Starts the MIRA MCP server.

Transport modes (--transport):
  stdio  – default, communication over stdin/stdout (Claude Desktop, etc.)
  sse    – HTTP Server-Sent Events at --mcp-addr (default localhost:3001)
  http   – stateless JSON-RPC over HTTP at --mcp-addr/mcp (default localhost:3001)

Optional subsystems:
  --with-soul     enable SOUL identity subsystem (+8 tools, total 22)
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
					Version         string   `json:"version"`
					Uptime          string   `json:"uptime"`
					ConfigFile      string   `json:"config_file"`
					StoragePath     string   `json:"storage_path"`
					Model           string   `json:"model"`
					MCPTransport    string   `json:"mcp_transport"`
					MCPAddress      string   `json:"mcp_address,omitempty"`
					APIEnabled      bool     `json:"api_enabled"`
					APIAddress      string   `json:"api_address,omitempty"`
					MetricsEnabled  bool     `json:"metrics_enabled"`
					PrometheusAddr  string   `json:"prometheus_addr,omitempty"`
					SOULEnabled     bool     `json:"soul_enabled"`
					WebhooksEnabled bool     `json:"webhooks_enabled"`
					WebhookCount    int      `json:"webhook_count"`
					HNSWKeySet      bool     `json:"hnsw_key_set"`
					Stats           any      `json:"stats,omitempty"`
					Models          []string `json:"models,omitempty"`
				}
				r := doctorReport{
					Version:         miraVersion,
					Uptime:          out.Uptime,
					ConfigFile:      globalFlags.configPath,
					StoragePath:     cfg.Storage.Path,
					Model:           cfg.Embeddings.CurrentModel,
					MCPTransport:    cfg.MCP.Transport,
					SOULEnabled:     cfg.Soul.Enabled,
					WebhooksEnabled: cfg.Webhooks.Enabled,
					WebhookCount:    len(cfg.Webhooks.Endpoints),
					HNSWKeySet:      cfg.HNSW.EncryptionKey != "",
					APIEnabled:      cfg.API.Enabled,
					MetricsEnabled:  cfg.Metrics.Enabled,
					Stats:           out.Stats,
					Models:          out.Models,
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
		query         string
		wing          string
		room          string
		memoryKind    string
		budget        int
		includeGlobal bool
		asJSON        bool
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
			if memoryKind != "" {
				kind := valueobjects.MemoryKind(memoryKind)
				if !kind.IsValid() {
					return fmt.Errorf("invalid --kind %q; valid values: identity, user, project, task, knowledge, history", memoryKind)
				}
				input.Kind = &kind
			}
			if includeGlobal && wing != "general" {
				input.FallbackWings = []string{"general"}
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
				len(out.Memories), out.BudgetUsed)
			for i, m := range out.Memories {
				fmt.Printf("[%d] %s\n", i+1, m.Rendered)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&query, "query", "q", "", "query text (required)")
	cmd.Flags().StringVarP(&wing, "wing", "w", "", "restrict search to this wing")
	cmd.Flags().StringVarP(&room, "room", "r", "", "restrict search to this room")
	cmd.Flags().StringVar(&memoryKind, "kind", "", "restrict search to a memory kind: identity, user, project, task, knowledge, history")
	cmd.Flags().IntVarP(&budget, "budget", "b", 2000, "token budget for recall")
	cmd.Flags().BoolVar(&includeGlobal, "include-global", false, "fall back to the shared general wing when the project has no results")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}

// ---------------------------------------------------------------------------
// store
// ---------------------------------------------------------------------------

func newStoreCmd() *cobra.Command {
	var (
		content    string
		wing       string
		room       string
		memType    string
		memoryKind string
		global     bool
		asJSON     bool
		validFrom  string
		validUntil string
	)
	cmd := &cobra.Command{
		Use:   "store",
		Short: "Store a single memory from the command line",
		Long: `Encodes and stores a memory without launching the MCP server.

Examples:
  mira store --content "PostgreSQL chosen for primary DB" --wing backend-team
  mira store --content "Prefer REST over gRPC for public APIs" -w arch --type decision
  mira store --content "User prefers French" --global --type preference
	  mira store --content "PostgreSQL is the primary DB" -w backend --valid-from 2026-04-15T00:00:00Z
  mira store --content "Fixed N+1 query in user loader" -w backend-team --type fact --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if content == "" {
				return fmt.Errorf("--content / -c is required")
			}
			if global {
				if wing != "" && wing != "general" {
					return fmt.Errorf("--global cannot be combined with --wing other than general")
				}
				wing = "general"
			}
			if wing == "" {
				return fmt.Errorf("--wing / -w is required (or use --global for shared memory)")
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
			var parseErr error
			if input.ValidFrom, parseErr = parseOptionalRFC3339(validFrom); parseErr != nil {
				return fmt.Errorf("invalid --valid-from: %w", parseErr)
			}
			if input.ValidUntil, parseErr = parseOptionalRFC3339(validUntil); parseErr != nil {
				return fmt.Errorf("invalid --valid-until: %w", parseErr)
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
			if memoryKind != "" {
				kind := valueobjects.MemoryKind(memoryKind)
				if !kind.IsValid() {
					return fmt.Errorf("invalid --kind %q; valid values: identity, user, project, task, knowledge, history", memoryKind)
				}
				input.Kind = &kind
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

			fmt.Printf("Stored  fingerprint=%s  type=%s  kind=%s  facts=%d  tokens=%d\n",
				out.FingerprintID, out.Type, out.Kind, out.FactCount, out.TokenCount)
			return nil
		},
	}
	cmd.Flags().StringVar(&content, "content", "", "memory content text (required)")
	cmd.Flags().StringVarP(&wing, "wing", "w", "", "wing (project/context namespace) (required)")
	cmd.Flags().StringVarP(&room, "room", "r", "", "optional sub-namespace within the wing")
	cmd.Flags().StringVarP(&memType, "type", "t", "", "memory type: fact, decision, preference, session_note, debug_log")
	cmd.Flags().StringVar(&memoryKind, "kind", "", "business role: identity, user, project, task, knowledge, history")
	cmd.Flags().BoolVar(&global, "global", false, "store in the shared general wing")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output stored fingerprint as JSON")
	cmd.Flags().StringVar(&validFrom, "valid-from", "", "RFC3339 time from which this memory is valid")
	cmd.Flags().StringVar(&validUntil, "valid-until", "", "RFC3339 time after which this memory is no longer valid")
	return cmd
}

func parseOptionalRFC3339(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
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
				_, _ = fmt.Scanln(&answer)
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
	ID         string     `json:"id"`
	Content    string     `json:"content"`
	Wing       string     `json:"wing"`
	Room       *string    `json:"room,omitempty"`
	Type       string     `json:"type"`
	Kind       string     `json:"kind"`
	CreatedAt  string     `json:"created_at"`
	ValidFrom  *time.Time `json:"valid_from,omitempty"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`
}

// mem0Export mirrors the documented Mem0 list response envelope. MIRA-specific
// fields live under metadata so that the text remains usable by Mem0 clients.
type mem0Export struct {
	Results []mem0ExportRecord `json:"results"`
}

type mem0ExportRecord struct {
	ID         string         `json:"id"`
	Memory     string         `json:"memory"`
	Metadata   map[string]any `json:"metadata"`
	Categories []string       `json:"categories,omitempty"`
	CreatedAt  string         `json:"created_at,omitempty"`
	UpdatedAt  string         `json:"updated_at,omitempty"`
}

func newExportCmd() *cobra.Command {
	var (
		wing   string
		output string
		limit  int
		format string
	)
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export memories as JSON, Markdown, or Mem0 JSON",
		Long: `Fetches verbatims via the timeline and writes them as JSON, Markdown, or Mem0 JSON.

Output goes to stdout by default; use --output to write to a file.
Both JSON and MIRA Markdown exports are accepted by mira import. Markdown is
intended for readable snapshots as well as round-trip portability.

Examples:
  mira export                                  # all wings, stdout
  mira export --wing backend-team              # filter by wing
  mira export --output memories.json           # write to file
	  mira export --format markdown -o memories.md # readable snapshot
  mira export --wing backend-team -o out.json  # combined
  mira export --limit 500                      # cap at 500 records`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if format != "json" && format != "markdown" && format != "mem0" {
				return fmt.Errorf("export: unsupported format %q; supported formats: json, markdown, mem0", format)
			}

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
					ID:         v.ID.String(),
					Content:    v.Content,
					Wing:       v.Wing,
					Room:       v.Room,
					Type:       string(item.Type),
					Kind:       string(v.Kind),
					CreatedAt:  v.CreatedAt.Format(time.RFC3339),
					ValidFrom:  v.ValidFrom,
					ValidUntil: v.ValidUntil,
				})
			}

			var data []byte
			if format == "markdown" {
				data = []byte(renderMarkdownExport(records))
			} else if format == "mem0" {
				var marshalErr error
				data, marshalErr = json.MarshalIndent(renderMem0Export(records), "", "  ")
				if marshalErr != nil {
					return fmt.Errorf("export: Mem0 JSON marshal error: %w", marshalErr)
				}
			} else {
				var marshalErr error
				data, marshalErr = json.MarshalIndent(records, "", "  ")
				if marshalErr != nil {
					return fmt.Errorf("export: JSON marshal error: %w", marshalErr)
				}
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
	cmd.Flags().StringVar(&format, "format", "json", "export format: json, markdown, or mem0")
	return cmd
}

func renderMem0Export(records []exportRecord) mem0Export {
	result := mem0Export{Results: make([]mem0ExportRecord, 0, len(records))}
	for _, record := range records {
		metadata := map[string]any{
			"mira_wing": record.Wing,
			"mira_type": record.Type,
			"mira_kind": record.Kind,
		}
		if record.Room != nil {
			metadata["mira_room"] = *record.Room
		}
		if record.ValidFrom != nil {
			metadata["mira_valid_from"] = record.ValidFrom.Format(time.RFC3339)
		}
		if record.ValidUntil != nil {
			metadata["mira_valid_until"] = record.ValidUntil.Format(time.RFC3339)
		}
		result.Results = append(result.Results, mem0ExportRecord{
			ID:         record.ID,
			Memory:     record.Content,
			Metadata:   metadata,
			Categories: []string{record.Type},
			CreatedAt:  record.CreatedAt,
			UpdatedAt:  record.CreatedAt,
		})
	}
	return result
}

func renderMarkdownExport(records []exportRecord) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# MIRA memory export\n\n%d memories exported.\n", len(records))

	for _, record := range records {
		fmt.Fprintf(&out, "\n## %s\n\n", markdownCode(record.ID))
		fmt.Fprintf(&out, "- Wing: %s\n", markdownCode(record.Wing))
		if record.Room != nil {
			fmt.Fprintf(&out, "- Room: %s\n", markdownCode(*record.Room))
		}
		fmt.Fprintf(&out, "- Type: %s\n- Kind: %s\n- Created: %s\n", markdownCode(record.Type), markdownCode(record.Kind), markdownCode(record.CreatedAt))
		if record.ValidFrom != nil {
			fmt.Fprintf(&out, "- Valid from: %s\n", markdownCode(record.ValidFrom.Format(time.RFC3339)))
		}
		if record.ValidUntil != nil {
			fmt.Fprintf(&out, "- Valid until: %s\n", markdownCode(record.ValidUntil.Format(time.RFC3339)))
		}
		out.WriteString("\n")
		out.WriteString("### Memory\n\n")
		out.WriteString(record.Content)
		out.WriteString("\n")
	}

	return out.String()
}

func markdownCode(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "\\`") + "`"
}

// ---------------------------------------------------------------------------
// import
// ---------------------------------------------------------------------------

// importRecord mirrors exportRecord for round-trippable JSON import.
type importRecord struct {
	Content    string     `json:"content"`
	Wing       string     `json:"wing"`
	Room       *string    `json:"room,omitempty"`
	Type       string     `json:"type,omitempty"`
	Kind       string     `json:"kind,omitempty"`
	ValidFrom  *time.Time `json:"valid_from,omitempty"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`
}

func newImportCmd() *cobra.Command {
	var (
		file   string
		wing   string
		format string
		dryRun bool
	)
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import memories from JSON or MIRA Markdown",
		Long: `Reads a JSON export or a MIRA Markdown export and stores each memory.

The file format matches the output of: mira export. Use --format to force a
parser; the default automatically detects JSON or Markdown.

Examples:
  mira import --file memories.json
	  mira import --file memories.md --format markdown
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
			records, err := parseImportRecords(data, format)
			if err != nil {
				return fmt.Errorf("import: %w", err)
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
					Content:    rec.Content,
					Wing:       w,
					Room:       rec.Room,
					ValidFrom:  rec.ValidFrom,
					ValidUntil: rec.ValidUntil,
				}
				if rec.Type != "" {
					mt := valueobjects.MemoryType(rec.Type)
					if mt.IsValid() {
						input.Type = &mt
					}
				}
				if rec.Kind != "" {
					kind := valueobjects.MemoryKind(rec.Kind)
					if !kind.IsValid() {
						log.Printf("import: invalid memory kind %q", rec.Kind)
						failed++
						continue
					}
					input.Kind = &kind
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
	cmd.Flags().StringVarP(&file, "file", "f", "", "path to JSON or MIRA Markdown file to import (required)")
	cmd.Flags().StringVarP(&wing, "wing", "w", "", "override wing for all imported records")
	cmd.Flags().StringVar(&format, "format", "auto", "input format: auto, json, or markdown")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate and preview without writing")
	return cmd
}

func parseImportRecords(data []byte, format string) ([]importRecord, error) {
	switch format {
	case "auto":
		if json.Valid(data) {
			format = "json"
		} else {
			format = "markdown"
		}
	case "json", "markdown", "mem0":
	default:
		return nil, fmt.Errorf("unsupported format %q; supported formats: auto, json, markdown, mem0", format)
	}

	if format == "json" {
		var records []importRecord
		if err := json.Unmarshal(data, &records); err != nil {
			return nil, fmt.Errorf("JSON parse error: %w", err)
		}
		return records, nil
	}
	if format == "mem0" {
		return parseMem0Import(data)
	}
	return parseMarkdownImport(data)
}

type mem0ImportRecord struct {
	Memory     string         `json:"memory"`
	Metadata   map[string]any `json:"metadata"`
	Categories []string       `json:"categories"`
}

func parseMem0Import(data []byte) ([]importRecord, error) {
	var envelope struct {
		Results []mem0ImportRecord `json:"results"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("Mem0 JSON parse error: %w", err)
	}
	if len(envelope.Results) == 0 {
		if err := json.Unmarshal(data, &envelope.Results); err != nil {
			return nil, fmt.Errorf("Mem0 JSON must contain a results array or be an array: %w", err)
		}
	}

	records := make([]importRecord, 0, len(envelope.Results))
	for index, record := range envelope.Results {
		if strings.TrimSpace(record.Memory) == "" {
			return nil, fmt.Errorf("Mem0 record %d has no memory field", index+1)
		}
		wing := mem0MetadataString(record.Metadata, "mira_wing")
		if wing == "" {
			wing = "mem0"
		}
		imported := importRecord{
			Content: record.Memory,
			Wing:    wing,
			Room:    mem0MetadataStringPointer(record.Metadata, "mira_room"),
			Type:    mem0MetadataString(record.Metadata, "mira_type"),
			Kind:    mem0MetadataString(record.Metadata, "mira_kind"),
		}
		if imported.Type == "" && len(record.Categories) > 0 {
			imported.Type = record.Categories[0]
		}
		var err error
		if imported.ValidFrom, err = parseOptionalRFC3339(mem0MetadataString(record.Metadata, "mira_valid_from")); err != nil {
			return nil, fmt.Errorf("Mem0 record %d has invalid mira_valid_from: %w", index+1, err)
		}
		if imported.ValidUntil, err = parseOptionalRFC3339(mem0MetadataString(record.Metadata, "mira_valid_until")); err != nil {
			return nil, fmt.Errorf("Mem0 record %d has invalid mira_valid_until: %w", index+1, err)
		}
		records = append(records, imported)
	}
	return records, nil
}

func mem0MetadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key].(string)
	if !ok {
		return ""
	}
	return value
}

func mem0MetadataStringPointer(metadata map[string]any, key string) *string {
	value := mem0MetadataString(metadata, key)
	if value == "" {
		return nil
	}
	return &value
}

func parseMarkdownImport(data []byte) ([]importRecord, error) {
	var records []importRecord
	var current *importRecord
	var content strings.Builder
	inMemory := false

	flush := func() error {
		if current == nil {
			return nil
		}
		// One newline belongs to the record terminator and one may come from
		// strings.Split after the export's final newline.
		current.Content = strings.TrimSuffix(strings.TrimSuffix(content.String(), "\n"), "\n")
		if current.Content == "" {
			return fmt.Errorf("Markdown record has no memory content")
		}
		if current.Wing == "" {
			return fmt.Errorf("Markdown record has no wing")
		}
		records = append(records, *current)
		return nil
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "## `") && strings.HasSuffix(line, "`") {
			if err := flush(); err != nil {
				return nil, err
			}
			current = &importRecord{}
			content.Reset()
			inMemory = false
			continue
		}
		if current == nil {
			continue
		}
		if line == "### Memory" {
			inMemory = true
			continue
		}
		if inMemory {
			// The canonical MIRA export has one blank separator after the
			// "### Memory" marker; it is formatting, not memory content.
			if content.Len() == 0 && line == "" {
				continue
			}
			content.WriteString(line)
			content.WriteByte('\n')
			continue
		}

		if value, ok := markdownMetadata(line, "Wing"); ok {
			current.Wing = value
		} else if value, ok := markdownMetadata(line, "Room"); ok {
			current.Room = &value
		} else if value, ok := markdownMetadata(line, "Type"); ok {
			current.Type = value
		} else if value, ok := markdownMetadata(line, "Kind"); ok {
			current.Kind = value
		} else if value, ok := markdownMetadata(line, "Valid from"); ok {
			parsed, err := parseOptionalRFC3339(value)
			if err != nil {
				return nil, fmt.Errorf("invalid Markdown valid from: %w", err)
			}
			current.ValidFrom = parsed
		} else if value, ok := markdownMetadata(line, "Valid until"); ok {
			parsed, err := parseOptionalRFC3339(value)
			if err != nil {
				return nil, fmt.Errorf("invalid Markdown valid until: %w", err)
			}
			current.ValidUntil = parsed
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no MIRA Markdown records found")
	}
	return records, nil
}

func markdownMetadata(line, key string) (string, bool) {
	prefix := "- " + key + ": "
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	value := strings.TrimPrefix(line, prefix)
	if len(value) < 2 || value[0] != '`' || value[len(value)-1] != '`' {
		return "", false
	}
	return strings.ReplaceAll(value[1:len(value)-1], "\\`", "`"), true
}

// ---------------------------------------------------------------------------
// ingest
// ---------------------------------------------------------------------------

func newIngestCmd() *cobra.Command {
	var file, wing, room string
	var includeAssistant, dryRun, stream bool
	var minChars int
	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Extract memories automatically from a JSON conversation",
		Long: `Reads a portable JSON conversation and sends selected messages through
MIRA's normal T0/T1/T2 extraction pipeline. By default it captures substantive
user messages; --include-assistant also captures substantive assistant replies.
Captured messages are marked kind=history while their technical type remains
auto-detected by the extractor. Exact duplicate protection applies normally.

Accepted file input: a JSON array of {"role","content"} messages, or an
object containing a "messages" array. With --stream, MIRA reads JSON Lines
from standard input and stores selected messages as they arrive; this is suited
to local conversation hooks, exporters, and Cursor CLI stream-json output.

Examples:
  mira ingest --file conversation.json --wing api
  mira ingest --file conversation.json --wing api --include-assistant
	  mira ingest --file conversation.json --wing api --dry-run
	  conversation-hook --jsonl | mira ingest --stream --wing api
	  cursor-agent --output-format stream-json "..." | mira ingest --stream --wing api --include-assistant`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if (file == "" && !stream) || (file != "" && stream) {
				return fmt.Errorf("provide exactly one of --file / -f or --stream")
			}
			if wing == "" {
				return fmt.Errorf("--wing / -w is required")
			}
			var roomRef *string
			if room != "" {
				roomRef = &room
			}
			if stream {
				return ingestConversationStream(wing, roomRef, includeAssistant, minChars, dryRun)
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("ingest: cannot read file %q: %w", file, err)
			}
			messages, err := interactors.ParseConversationMessages(raw)
			if err != nil {
				return fmt.Errorf("ingest: %w", err)
			}
			inputs, err := interactors.ConversationMemoryInputs(messages, wing, roomRef, includeAssistant, minChars)
			if err != nil {
				return fmt.Errorf("ingest: %w", err)
			}
			if len(inputs) == 0 {
				return fmt.Errorf("ingest: no messages matched the selected roles and --min-chars=%d", minChars)
			}
			if dryRun {
				fmt.Printf("Dry-run: %d of %d conversation messages would be extracted as history memories.\n", len(inputs), len(messages))
				for index, input := range inputs {
					fmt.Printf("  [%d] role=%-9s len=%d %q\n", index+1, input.Metrics["role"], len([]rune(input.Content)), truncateRunes(input.Content, 80))
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
				return fmt.Errorf("ingest: failed to initialise application: %w", err)
			}
			defer application.Close()

			stored, failed := 0, 0
			storeUC := application.StoreMemoryUC()
			for index, input := range inputs {
				_, storeErr := storeUC.Execute(context.Background(), input)
				if storeErr != nil {
					log.Printf("ingest: failed to store message %d: %v", index+1, storeErr)
					failed++
					continue
				}
				stored++
			}
			fmt.Printf("Conversation ingest complete: %d stored, %d failed (selected: %d)\n", stored, failed, len(inputs))
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "JSON conversation file (required)")
	cmd.Flags().StringVarP(&wing, "wing", "w", "", "wing to store extracted memories in (required)")
	cmd.Flags().StringVarP(&room, "room", "r", "", "optional room for extracted memories")
	cmd.Flags().BoolVar(&includeAssistant, "include-assistant", false, "also extract substantive assistant messages")
	cmd.Flags().IntVar(&minChars, "min-chars", 20, "minimum Unicode character count for a captured message")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview selected conversation messages without storing them")
	cmd.Flags().BoolVar(&stream, "stream", false, "read portable JSONL or Cursor CLI stream-json events from standard input")
	return cmd
}

func ingestConversationStream(wing string, room *string, includeAssistant bool, minChars int, dryRun bool) error {
	var store func(context.Context, interactors.StoreMemoryInput) error
	if !dryRun {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		applyStoragePath(cfg)
		application, err := app.NewApplication(cfg)
		if err != nil {
			return fmt.Errorf("ingest: failed to initialise application: %w", err)
		}
		defer application.Close()
		storeUC := application.StoreMemoryUC()
		store = func(ctx context.Context, input interactors.StoreMemoryInput) error {
			_, err := storeUC.Execute(ctx, input)
			return err
		}
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	line, selected, stored, failed := 0, 0, 0, 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		message, recognized, err := interactors.ParseConversationStreamMessage([]byte(raw))
		if err != nil {
			return fmt.Errorf("ingest: invalid JSONL message on line %d: %w", line, err)
		}
		if !recognized {
			continue
		}
		inputs, err := interactors.ConversationMemoryInputs([]interactors.ConversationMessage{message}, wing, room, includeAssistant, minChars)
		if err != nil {
			return fmt.Errorf("ingest: %w", err)
		}
		if len(inputs) == 0 {
			continue
		}
		selected++
		input := inputs[0]
		input.Metrics["source"] = "conversation_stream"
		input.Metrics["message_index"] = line
		if dryRun {
			fmt.Printf("Dry-run: line %d role=%-9s len=%d %q\n", line, input.Metrics["role"], len([]rune(input.Content)), truncateRunes(input.Content, 80))
			continue
		}
		if err := store(context.Background(), input); err != nil {
			log.Printf("ingest: failed to store streamed message on line %d: %v", line, err)
			failed++
			continue
		}
		stored++
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("ingest: read JSONL stream: %w", err)
	}
	if selected == 0 {
		return fmt.Errorf("ingest: no streamed messages matched the selected roles and --min-chars=%d", minChars)
	}
	if dryRun {
		fmt.Printf("Stream dry-run complete: %d message(s) would be extracted as history memories.\n", selected)
		return nil
	}
	fmt.Printf("Conversation stream complete: %d stored, %d failed (selected: %d)\n", stored, failed, selected)
	return nil
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
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
		file       string
		budget     int
		keepLastN  int
		output     string
		statsOnly  bool
		assertions string
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
	  mira optimize --file history.json --assertions expected-evidence.json --stats-only
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
			if assertions != "" {
				required, err := readOptimizeAssertions(assertions)
				if err != nil {
					return err
				}
				metric := interactors.MeasureContextEfficiency(out.Messages, required, out.OptimizedTokens)
				fmt.Fprintf(os.Stderr, "evidence retained: %d / %d (%.1f%%)\n", metric.RetainedAssertions, metric.RequiredAssertions, metric.CoveragePercent)
				fmt.Fprintf(os.Stderr, "context efficiency: %.3f coverage-points / 1K tokens\n", metric.CoveragePer1KTokens)
			}

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
	cmd.Flags().StringVar(&assertions, "assertions", "", "JSON file containing required evidence strings to measure retained context coverage")
	return cmd
}

func readOptimizeAssertions(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("optimize: failed to read assertions %q: %w", path, err)
	}
	var assertions []string
	if err := json.Unmarshal(raw, &assertions); err != nil {
		return nil, fmt.Errorf("optimize: assertions must be a JSON array of strings: %w", err)
	}
	for index, assertion := range assertions {
		if strings.TrimSpace(assertion) == "" {
			return nil, fmt.Errorf("optimize: assertion %d must not be empty", index+1)
		}
	}
	return assertions, nil
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
