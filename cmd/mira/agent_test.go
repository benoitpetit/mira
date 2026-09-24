package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
