package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/benoitpetit/mira/internal/agentinstall"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func TestClientMCPAdaptersPreserveExistingConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name      string
		initial   string
		configure func(string, string, string, bool) ([]byte, error)
		json      bool
	}{
		{name: "OpenCode", initial: `{"theme":"dark","mcp":{"other":{"type":"local","command":"other"}}}`, configure: configureOpenCodeMCP, json: true},
		{name: "Pi", initial: `{"mcpServers":{"other":{"command":"other"}}}`, configure: configurePiMCP, json: true},
		{name: "Hermes", initial: "theme: dark\nmcp_servers:\n  other:\n    command: other\n", configure: configureHermesMCP},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			if err := os.WriteFile(path, []byte(tt.initial), 0o600); err != nil {
				t.Fatal(err)
			}
			data, err := tt.configure(path, "/bin/mira", "/project/.mira/config.yaml", false)
			if err != nil {
				t.Fatal(err)
			}
			settings := map[string]any{}
			if tt.json {
				err = json.Unmarshal(data, &settings)
			} else {
				err = yaml.Unmarshal(data, &settings)
			}
			if err != nil {
				t.Fatal(err)
			}
			key := "mcp"
			if tt.name == "Pi" {
				key = "mcpServers"
			} else if tt.name == "Hermes" {
				key = "mcp_servers"
			}
			servers := settings[key].(map[string]any)
			if servers["other"] == nil || servers["mira"] == nil {
				t.Fatalf("MCP entries were not preserved and merged: %#v", servers)
			}
		})
	}
}

func TestCursorRuleHasFrontMatter(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".cursor", "rules", "mira.mdc")
	if err := installManagedInstructions(path, "MIRA guidance", false, io.Discard); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "---\n") || !strings.Contains(string(data), "alwaysApply: true") {
		t.Fatalf("Cursor rule is missing front matter: %s", data)
	}
}

func TestAgentStatusReportsActualModes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	manifest := agentinstall.DefaultManifest("opencode", t.TempDir())
	manifest.RecallMode = agentinstall.IntegrationSkillGuided
	manifest.CaptureMode = agentinstall.IntegrationNone
	if err := agentinstall.SaveManifest(path, manifest); err != nil {
		t.Fatal(err)
	}
	cmd := newAgentStatusCmd()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"--manifest", path})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "recall=skill-guided capture=none") {
		t.Fatalf("status lacks actual modes: %s", output.String())
	}
}

func TestAgentDocumentationContract(t *testing.T) {
	clients := agentinstall.SupportedClients()
	if len(clients) != 8 {
		t.Fatalf("public clients = %d, want 8", len(clients))
	}
	for _, client := range clients {
		if client.ID == "" || !client.RecallMode.IsValid() || !client.CaptureMode.IsValid() {
			t.Fatalf("invalid public client contract: %#v", client)
		}
	}
}

func TestAgentHooksInstallSessionStartForCodexAndClaude(t *testing.T) {
	projectRoot := t.TempDir()
	configPath := filepath.Join(projectRoot, ".mira", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("storage: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := agentInstallOptions{
		BinaryPath:   "/bin/mira",
		MiraConfig:   configPath,
		ManifestPath: filepath.Join(projectRoot, ".mira", "agent.yaml"),
		Scope:        agentinstall.ScopeProject,
		Policy:       string(agentinstall.PolicyStandard),
	}
	for _, client := range []string{clientCodex, clientClaudeCode} {
		t.Run(client, func(t *testing.T) {
			cmd := &cobra.Command{}
			if err := installAgentHooks(cmd, client, projectRoot, t.TempDir(), options, ""); err != nil {
				t.Fatalf("install hooks: %v", err)
			}
			path := resolvedAgentHookPath(client, options, t.TempDir())
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read hooks: %v", err)
			}
			var settings map[string]any
			if err := json.Unmarshal(data, &settings); err != nil {
				t.Fatal(err)
			}
			hooks := settings["hooks"].(map[string]any)
			for _, event := range []string{"SessionStart", "UserPromptSubmit"} {
				if _, ok := hooks[event]; !ok {
					t.Errorf("missing %s hook", event)
				}
			}
		})
	}
}

func TestNormalizeAgentEventAppliesClientEventAndManifest(t *testing.T) {
	event, err := normalizeAgentEvent(strings.NewReader(`{"role":"user","content":"Keep project memory local.","session_id":"s1"}`), "codex", "prompt-submit", "/project/.mira/agent.yaml")
	if err != nil {
		t.Fatalf("normalizeAgentEvent failed: %v", err)
	}
	if event.Client != "codex" || event.EventName != "prompt-submit" || event.ManifestPath != "/project/.mira/agent.yaml" || event.Role != "user" || event.SessionID != "s1" {
		t.Fatalf("unexpected normalized event: %+v", event)
	}
}

func TestNormalizeAgentEventRejectsMalformedInput(t *testing.T) {
	if _, err := normalizeAgentEvent(strings.NewReader("not json"), "codex", "prompt-submit", "manifest"); err == nil {
		t.Fatal("normalizeAgentEvent accepted malformed JSON")
	}
}

func TestNormalizeAgentEventAdaptsClientHookPayloads(t *testing.T) {
	claude, err := normalizeAgentEvent(strings.NewReader(`{"prompt":"Use a local memory store for this project.","session_id":"s2"}`), "claude-code", "prompt-submit", "manifest")
	if err != nil || claude.Role != "user" || claude.Content == "" {
		t.Fatalf("Claude hook payload was not normalized: %+v err=%v", claude, err)
	}
	windsurf, err := normalizeAgentEvent(strings.NewReader(`{"agent_action_name":"post_cascade_response","tool_info":{"response":"The response should stay concise."}}`), "windsurf", "response-complete", "manifest")
	if err != nil || windsurf.Role != "assistant" || windsurf.Content == "" {
		t.Fatalf("Windsurf hook payload was not normalized: %+v err=%v", windsurf, err)
	}
}

func TestAgentInstallCursorIsIdempotentAndWritesManagedFiles(t *testing.T) {
	projectDir := t.TempDir()
	initCmd := newInitCmd()
	initCmd.SetArgs([]string{"--dir", projectDir})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	configPath := filepath.Join(projectDir, ".mira", "config.yaml")
	runInstall := func() string {
		cmd := newAgentCmd()
		var output bytes.Buffer
		cmd.SetOut(&output)
		cmd.SetArgs([]string{"install", "--client", "cursor", "--mira-config", configPath, "--policy", "standard"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("agent install failed: %v", err)
		}
		return output.String()
	}
	runInstall()
	runInstall()

	manifestPath := filepath.Join(projectDir, ".mira", "agent.yaml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("manifest missing: %v", err)
	}
	if !strings.Contains(string(manifest), "policy: standard") {
		t.Fatalf("manifest missing policy: %s", manifest)
	}
	rules, err := os.ReadFile(filepath.Join(projectDir, ".cursor", "rules", "mira.mdc"))
	if err != nil {
		t.Fatalf("managed Cursor rules missing: %v", err)
	}
	if strings.Count(string(rules), "MIRA:BEGIN managed instructions") != 1 {
		t.Fatalf("managed rules were duplicated: %s", rules)
	}
	mcp, err := os.ReadFile(filepath.Join(projectDir, ".cursor", "mcp.json"))
	if err != nil || strings.Count(string(mcp), `"mira"`) != 1 {
		t.Fatalf("Cursor MCP config is invalid: %s err=%v", mcp, err)
	}
}

func TestAgentInstallDoctorMatrixUsesProjectOwnedArtifacts(t *testing.T) {
	binDir := t.TempDir()
	for _, executable := range []string{"codex", "claude"} {
		path := filepath.Join(binDir, executable)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", t.TempDir())

	for _, client := range []string{"codex", "claude-code", "windsurf", "cursor", "hermes", "opencode", "pi"} {
		t.Run(client, func(t *testing.T) {
			projectDir := t.TempDir()
			initCmd := newInitCmd()
			initCmd.SetArgs([]string{"--dir", projectDir})
			if err := initCmd.Execute(); err != nil {
				t.Fatalf("init failed: %v", err)
			}
			configPath := filepath.Join(projectDir, ".mira", "config.yaml")
			manifestPath := filepath.Join(projectDir, ".mira", "agent.yaml")
			clientConfig := filepath.Join(projectDir, ".agent-config", client+".json")
			clientConfigArgs := []string{"--client-config", clientConfig}
			if client == clientClaudeCode {
				clientConfigArgs = nil
			}
			install := newAgentCmd()
			install.SetArgs(append([]string{"install", "--client", client, "--mira-config", configPath, "--mira-binary", "/bin/sh"}, clientConfigArgs...))
			if err := install.Execute(); err != nil {
				t.Fatalf("install failed: %v", err)
			}
			reinstall := newAgentCmd()
			reinstall.SetArgs(append([]string{"install", "--client", client, "--mira-config", configPath, "--mira-binary", "/bin/sh"}, clientConfigArgs...))
			if err := reinstall.Execute(); err != nil {
				t.Fatalf("reinstall failed: %v", err)
			}
			manifest, err := agentinstall.LoadManifest(manifestPath)
			if err != nil {
				t.Fatalf("load manifest: %v", err)
			}
			if client != clientClaudeCode && manifest.ClientConfigPath != clientConfig {
				t.Fatalf("client config path = %q, want %q", manifest.ClientConfigPath, clientConfig)
			}
			if manifest.HookConfigPath != "" && !strings.HasPrefix(manifest.HookConfigPath, projectDir+string(filepath.Separator)) {
				t.Fatalf("hook config escaped project scope: %q", manifest.HookConfigPath)
			}
			doctor := newAgentCmd()
			doctor.SetArgs([]string{"doctor", "--manifest", manifestPath})
			if err := doctor.Execute(); err != nil {
				t.Fatalf("doctor failed: %v", err)
			}
		})
	}
}

func TestAgentInstallDryRunDoesNotWrite(t *testing.T) {
	projectDir := t.TempDir()
	initCmd := newInitCmd()
	initCmd.SetArgs([]string{"--dir", projectDir})
	if err := initCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(projectDir, ".mira", "config.yaml")
	cmd := newAgentCmd()
	cmd.SetArgs([]string{"install", "--client", "cursor", "--mira-config", configPath, "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry-run install failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".mira", "agent.yaml")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created manifest: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".cursor")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created Cursor files: %v", err)
	}
}

func TestAgentUninstallPreservesUserInstructionContent(t *testing.T) {
	projectDir := t.TempDir()
	initCmd := newInitCmd()
	initCmd.SetArgs([]string{"--dir", projectDir})
	if err := initCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(projectDir, ".mira", "config.yaml")
	install := newAgentCmd()
	install.SetArgs([]string{"install", "--client", "cursor", "--mira-config", configPath})
	if err := install.Execute(); err != nil {
		t.Fatal(err)
	}
	rulesPath := filepath.Join(projectDir, ".cursor", "rules", "mira.mdc")
	rules, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatal(err)
	}
	rules = append([]byte("# User rules\nKeep this.\n"), rules...)
	if err := os.WriteFile(rulesPath, rules, 0o644); err != nil {
		t.Fatal(err)
	}
	uninstall := newAgentCmd()
	uninstall.SetArgs([]string{"uninstall", "--manifest", filepath.Join(projectDir, ".mira", "agent.yaml")})
	if err := uninstall.Execute(); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}
	remaining, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(remaining) != "# User rules\nKeep this.\n" {
		t.Fatalf("uninstall changed user instructions: %q", remaining)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".mira", "agent.yaml")); !os.IsNotExist(err) {
		t.Fatalf("manifest was not removed: %v", err)
	}
}

func TestAgentUninstallUsesExplicitClientConfigPath(t *testing.T) {
	projectDir := t.TempDir()
	initCmd := newInitCmd()
	initCmd.SetArgs([]string{"--dir", projectDir})
	if err := initCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(projectDir, ".mira", "config.yaml")
	clientPath := filepath.Join(projectDir, "settings", "cursor.json")
	install := newAgentCmd()
	install.SetArgs([]string{"install", "--client", "cursor", "--mira-config", configPath, "--client-config", clientPath})
	if err := install.Execute(); err != nil {
		t.Fatal(err)
	}
	uninstall := newAgentCmd()
	uninstall.SetArgs([]string{"uninstall", "--manifest", filepath.Join(projectDir, ".mira", "agent.yaml")})
	if err := uninstall.Execute(); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}
	data, err := os.ReadFile(clientPath)
	if err != nil || strings.Contains(string(data), `"mira"`) {
		t.Fatalf("explicit client config was not cleaned: %s err=%v", data, err)
	}
}

func TestAgentDryRunReportsHookChanges(t *testing.T) {
	projectDir := t.TempDir()
	initCmd := newInitCmd()
	initCmd.SetArgs([]string{"--dir", projectDir})
	if err := initCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	cmd := newAgentCmd()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"install", "--client", "windsurf", "--mira-config", filepath.Join(projectDir, ".mira", "config.yaml"), "--client-config", filepath.Join(projectDir, "windsurf.json"), "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry-run install failed: %v", err)
	}
	if !strings.Contains(output.String(), "hooks.json") {
		t.Fatalf("dry-run did not report hook changes: %s", output.String())
	}
}
