package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/config"
	"github.com/benoitpetit/mira/internal/usecases/interactors"
)

func TestInitCreatesProjectLocalConfiguration(t *testing.T) {
	projectDir := t.TempDir()
	cmd := newInitCmd()
	cmd.SetArgs([]string{"--dir", projectDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	configPath := filepath.Join(projectDir, ".mira", "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("configuration was not created: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("generated configuration is invalid: %v", err)
	}
	if got, want := cfg.Storage.Path, filepath.Join(projectDir, ".mira"); got != want {
		t.Errorf("storage path = %q, want %q", got, want)
	}

	second := newInitCmd()
	second.SetArgs([]string{"--dir", projectDir})
	if err := second.Execute(); err == nil {
		t.Fatal("expected init to reject an existing configuration")
	}
}

func TestServerProvidesStartAlias(t *testing.T) {
	for _, alias := range newServerCmd().Aliases {
		if alias == "start" {
			return
		}
	}
	t.Error("server command does not provide the start alias")
}

func TestCodexSetupArgs(t *testing.T) {
	got := codexSetupArgs("/usr/local/bin/mira", "/work/api/.mira/config.yaml")
	want := []string{
		"mcp", "add", "mira", "--", "/usr/local/bin/mira",
		"--config", "/work/api/.mira/config.yaml", "server",
	}
	if len(got) != len(want) {
		t.Fatalf("argument count = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("argument %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestClaudeCodeSetupArgs(t *testing.T) {
	got := claudeCodeSetupArgs("/usr/local/bin/mira", "/work/api/.mira/config.yaml", "project")
	want := []string{
		"mcp", "add", "mira", "--scope", "project", "--",
		"/usr/local/bin/mira", "--config", "/work/api/.mira/config.yaml", "server",
	}
	if !slices.Equal(got, want) {
		t.Errorf("arguments = %#v, want %#v", got, want)
	}
}

func TestConfigureCursorMCP(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".cursor", "mcp.json")
	if err := os.Mkdir(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"other":{"command":"other","args":[]},"mira":{"command":"old","args":["server"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := configureCursorMCP(path, "/bin/mira", "/project/.mira/config.yaml", false); err == nil {
		t.Fatal("expected a conflicting MIRA Cursor configuration to be rejected")
	}

	data, err := configureCursorMCP(path, "/bin/mira", "/project/.mira/config.yaml", true)
	if err != nil {
		t.Fatalf("configureCursorMCP failed: %v", err)
	}
	var config mcpConfigFile
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	if config.MCPServers["other"].Command != "other" {
		t.Error("existing non-MIRA Cursor servers should be preserved")
	}
	if got := config.MCPServers["mira"]; got.Command != "/bin/mira" || !slices.Equal(got.Args, []string{"--config", "/project/.mira/config.yaml", "server"}) {
		t.Errorf("MIRA Cursor config = %#v", got)
	}
}

func TestCursorMCPConfigPath(t *testing.T) {
	got := cursorMCPConfigPath("/work/api/.mira/config.yaml")
	want := filepath.Join("/work/api", ".cursor", "mcp.json")
	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}

func TestConfigureWindsurfMCP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp_config.json")
	data, err := configureWindsurfMCP(path, "/bin/mira", "/project/.mira/config.yaml", false)
	if err != nil {
		t.Fatalf("configureWindsurfMCP failed: %v", err)
	}
	var config mcpConfigFile
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	if got := config.MCPServers["mira"]; got.Command != "/bin/mira" || !slices.Equal(got.Args, []string{"--config", "/project/.mira/config.yaml", "server"}) {
		t.Errorf("MIRA Windsurf config = %#v", got)
	}
}

func TestWindsurfMCPConfigPath(t *testing.T) {
	got := windsurfMCPConfigPath("/home/alice")
	want := filepath.Join("/home/alice", ".codeium", "windsurf", "mcp_config.json")
	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}

func TestClaudeDesktopMCPConfigPath(t *testing.T) {
	path, err := claudeDesktopMCPConfigPath("darwin", "/Users/alice", "")
	if err != nil {
		t.Fatalf("macOS path: %v", err)
	}
	if want := filepath.Join("/Users/alice", "Library", "Application Support", "Claude", "claude_desktop_config.json"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	path, err = claudeDesktopMCPConfigPath("windows", "", `C:\\Users\\alice\\AppData\\Roaming`)
	if err != nil {
		t.Fatalf("Windows path: %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join("Claude", "claude_desktop_config.json")) {
		t.Errorf("Windows path = %q", path)
	}
	if _, err := claudeDesktopMCPConfigPath("linux", "/home/alice", ""); err == nil {
		t.Error("expected Linux to require --client-config")
	}
}

func TestConfigureClaudeCodeMemoryHookPreservesSettingsAndAvoidsDuplicates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.local.json")
	if err := os.WriteFile(path, []byte(`{"permissions":{"allow":["Read"]},"hooks":{"PostToolUse":[{"matcher":"Write","hooks":[{"type":"command","command":"fmt"}]}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := configureClaudeCodeMemoryHook(path, "/bin/mira", "/project/.mira/config.yaml", "api")
	if err != nil {
		t.Fatalf("configureClaudeCodeMemoryHook failed: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	if _, ok := settings["permissions"]; !ok {
		t.Error("existing settings should be preserved")
	}
	hooks := settings["hooks"].(map[string]any)
	if len(hooks["UserPromptSubmit"].([]any)) != 1 {
		t.Fatalf("UserPromptSubmit hooks = %#v", hooks["UserPromptSubmit"])
	}
	if !strings.Contains(string(data), "hook claude-code --wing 'api'") {
		t.Errorf("hook command missing from %s", data)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	again, err := configureClaudeCodeMemoryHook(path, "/bin/mira", "/project/.mira/config.yaml", "api")
	if err != nil {
		t.Fatalf("second configure failed: %v", err)
	}
	if strings.Count(string(again), "hook claude-code") != 1 {
		t.Errorf("duplicate hook in %s", again)
	}
}

func TestConfigureCodexMemoryHookUsesCodexCommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	data, err := configureCodexMemoryHook(path, "/bin/mira", "/project/.mira/config.yaml", "api")
	if err != nil {
		t.Fatalf("configureCodexMemoryHook failed: %v", err)
	}
	if !strings.Contains(string(data), "hook codex --wing 'api'") {
		t.Errorf("Codex hook command missing from %s", data)
	}
	if got := codexHooksConfigPath("/home/alice"); got != filepath.Join("/home/alice", ".codex", "hooks.json") {
		t.Errorf("Codex hooks path = %q", got)
	}
}

func TestConfigureClientMemoryHooksAddsAssistantStopOnOptIn(t *testing.T) {
	for _, client := range []struct {
		name      string
		configure func(string, string, string, string, bool) ([]byte, error)
	}{
		{name: "Claude Code", configure: configureClaudeCodeMemoryHooks},
		{name: "Codex", configure: configureCodexMemoryHooks},
	} {
		t.Run(client.name, func(t *testing.T) {
			data, err := client.configure(filepath.Join(t.TempDir(), "hooks.json"), "/bin/mira", "/project/.mira/config.yaml", "api", true)
			if err != nil {
				t.Fatalf("configure hooks: %v", err)
			}
			var settings map[string]any
			if err := json.Unmarshal(data, &settings); err != nil {
				t.Fatal(err)
			}
			hooks := settings["hooks"].(map[string]any)
			if _, ok := hooks["UserPromptSubmit"]; !ok {
				t.Error("missing user-prompt hook")
			}
			if _, ok := hooks["Stop"]; !ok {
				t.Error("missing assistant-response Stop hook")
			}
		})
	}
}

func TestConfigureWindsurfMemoryHooksHonorsAssistantOptIn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	data, err := configureWindsurfMemoryHooks(path, "/bin/mira", "/project/.mira/config.yaml", "api", false)
	if err != nil {
		t.Fatalf("configureWindsurfMemoryHooks failed: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	hooks := settings["hooks"].(map[string]any)
	if _, ok := hooks["pre_user_prompt"]; !ok {
		t.Error("missing user prompt hook")
	}
	if _, ok := hooks["post_cascade_response"]; ok {
		t.Error("assistant response hook should require opt-in")
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	data, err = configureWindsurfMemoryHooks(path, "/bin/mira", "/project/.mira/config.yaml", "api", true)
	if err != nil {
		t.Fatalf("assistant opt-in failed: %v", err)
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	if _, ok := settings["hooks"].(map[string]any)["post_cascade_response"]; !ok {
		t.Error("missing opted-in assistant response hook")
	}
}

func TestReadOptimizeAssertions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "assertions.json")
	if err := os.WriteFile(path, []byte(`["Database is PostgreSQL", "Use migrations"]`), 0o600); err != nil {
		t.Fatal(err)
	}
	assertions, err := readOptimizeAssertions(path)
	if err != nil {
		t.Fatalf("readOptimizeAssertions failed: %v", err)
	}
	if len(assertions) != 2 || assertions[0] != "Database is PostgreSQL" {
		t.Errorf("assertions = %#v", assertions)
	}

	invalid := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(invalid, []byte(`["ok", " "]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readOptimizeAssertions(invalid); err == nil {
		t.Error("expected an empty assertion to be rejected")
	}
}

func TestConversationMemoryInputs(t *testing.T) {
	messages, err := interactors.ParseConversationMessages([]byte(`{"messages":[{"role":"user","content":"Use PostgreSQL as the primary database."},{"role":"assistant","content":"I will record the decision."}]}`))
	if err != nil {
		t.Fatalf("ParseConversationMessages failed: %v", err)
	}
	if len(messages) != 2 || messages[0].Role != "user" {
		t.Fatalf("messages = %#v", messages)
	}

	inputs, err := interactors.ConversationMemoryInputs(messages, "test", nil, false, 20)
	if err != nil {
		t.Fatalf("ConversationMemoryInputs failed: %v", err)
	}
	if len(inputs) != 1 || inputs[0].Metrics["role"] != "user" {
		t.Errorf("default inputs = %#v, want only the user message", inputs)
	}
	inputs, err = interactors.ConversationMemoryInputs(messages, "test", nil, true, 20)
	if err != nil {
		t.Fatalf("ConversationMemoryInputs including assistant failed: %v", err)
	}
	if len(inputs) != 2 {
		t.Errorf("inputs including assistant = %#v, want two messages", inputs)
	}
}

func TestParseConversationMessagesRejectsEmptyMessages(t *testing.T) {
	if _, err := interactors.ParseConversationMessages([]byte(`[]`)); err == nil {
		t.Error("expected an empty conversation to be rejected")
	}
}

func TestIngestRequiresExactlyOneInputMode(t *testing.T) {
	for _, args := range [][]string{
		{"--wing", "test"},
		{"--wing", "test", "--file", "conversation.json", "--stream"},
	} {
		cmd := newIngestCmd()
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Errorf("args %v: expected input mode validation error", args)
		}
	}
}

func TestRenderMarkdownExport(t *testing.T) {
	room := "sprint-42"
	got := renderMarkdownExport([]exportRecord{{
		ID:        "a-memory-id",
		Content:   "Use PostgreSQL for JSONB support.",
		Wing:      "backend",
		Room:      &room,
		Type:      "decision",
		Kind:      "project",
		CreatedAt: "2026-09-02T10:00:00Z",
	}})

	for _, want := range []string{
		"# MIRA memory export",
		"1 memories exported.",
		"## `a-memory-id`",
		"- Wing: `backend`",
		"- Room: `sprint-42`",
		"- Type: `decision`",
		"- Kind: `project`",
		"Use PostgreSQL for JSONB support.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Markdown export missing %q:\n%s", want, got)
		}
	}
}

func TestParseMarkdownImportRoundTrip(t *testing.T) {
	room := "decisions"
	from := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	until := from.Add(24 * time.Hour)
	export := renderMarkdownExport([]exportRecord{{
		ID:         "memory-id",
		Content:    "Use PostgreSQL for JSONB support.\nKeep the migration reversible.",
		Wing:       "backend",
		Room:       &room,
		Type:       "decision",
		Kind:       "project",
		CreatedAt:  "2026-09-02T10:00:00Z",
		ValidFrom:  &from,
		ValidUntil: &until,
	}})

	records, err := parseImportRecords([]byte(export), "markdown")
	if err != nil {
		t.Fatalf("parseImportRecords failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	got := records[0]
	if got.Content != "Use PostgreSQL for JSONB support.\nKeep the migration reversible." || got.Wing != "backend" || got.Room == nil || *got.Room != room || got.Type != "decision" || got.Kind != "project" {
		t.Errorf("record = %#v, want round-tripped values", got)
	}
	if got.ValidFrom == nil || !got.ValidFrom.Equal(from) || got.ValidUntil == nil || !got.ValidUntil.Equal(until) {
		t.Errorf("validity = %v..%v, want %v..%v", got.ValidFrom, got.ValidUntil, from, until)
	}
}

func TestParseMem0ImportRoundTrip(t *testing.T) {
	room := "decisions"
	from := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	until := from.Add(24 * time.Hour)
	data, err := json.Marshal(renderMem0Export([]exportRecord{{
		ID:         "memory-id",
		Content:    "Use PostgreSQL for JSONB support.",
		Wing:       "backend",
		Room:       &room,
		Type:       "decision",
		Kind:       "project",
		CreatedAt:  "2026-09-02T10:00:00Z",
		ValidFrom:  &from,
		ValidUntil: &until,
	}}))
	if err != nil {
		t.Fatalf("marshal Mem0 export: %v", err)
	}

	records, err := parseImportRecords(data, "mem0")
	if err != nil {
		t.Fatalf("parseImportRecords failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	got := records[0]
	if got.Content != "Use PostgreSQL for JSONB support." || got.Wing != "backend" || got.Room == nil || *got.Room != room || got.Type != "decision" || got.Kind != "project" {
		t.Errorf("record = %#v, want round-tripped values", got)
	}
	if got.ValidFrom == nil || !got.ValidFrom.Equal(from) || got.ValidUntil == nil || !got.ValidUntil.Equal(until) {
		t.Errorf("validity = %v..%v, want %v..%v", got.ValidFrom, got.ValidUntil, from, until)
	}
}
