package agentinstall

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultManifestUsesPrudentStandardPolicy(t *testing.T) {
	manifest := DefaultManifest("codex", "/repo")

	if manifest.Client != "codex" || manifest.Scope != ScopeProject {
		t.Fatalf("unexpected client scope: %+v", manifest)
	}
	if manifest.Policy != PolicyStandard || !manifest.Capture.UserPrompts {
		t.Fatalf("unexpected default policy: %+v", manifest)
	}
	if manifest.Capture.AssistantResponses || !manifest.Capture.RedactSecrets {
		t.Fatalf("prudent defaults leaked assistant capture or disabled redaction: %+v", manifest.Capture)
	}
	if !manifest.Recall.Enabled || manifest.Recall.BudgetTokens <= 0 {
		t.Fatalf("unexpected recall defaults: %+v", manifest.Recall)
	}
}

func TestManifestRoundTripsYAMLAndPreservesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".mira", "agent.yaml")
	manifest := DefaultManifest("claude-code", "/project")
	manifest.Wing = "my-project"
	manifest.Capture.MinChars = 32
	manifest.Recall.BudgetTokens = 720

	if err := SaveManifest(path, manifest); err != nil {
		t.Fatalf("SaveManifest failed: %v", err)
	}
	got, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}
	if got.Client != manifest.Client || got.Wing != manifest.Wing || got.Capture.MinChars != 32 || got.Recall.BudgetTokens != 720 {
		t.Fatalf("round trip changed manifest: got %+v want %+v", got, manifest)
	}
	if got.Capture.AssistantResponses || !got.Capture.RedactSecrets {
		t.Fatalf("round trip lost prudent defaults: %+v", got.Capture)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat manifest: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("manifest permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestLoadManifestRejectsInvalidPolicyAndWing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	if err := os.WriteFile(path, []byte("client: codex\nscope: project\npolicy: reckless\nwing: bad wing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(path); err == nil {
		t.Fatal("LoadManifest accepted invalid policy and wing")
	}
}

func TestResolveProjectWingIsStableForNormalizedAbsolutePath(t *testing.T) {
	root := t.TempDir()
	first, err := ResolveProjectWing(root)
	if err != nil {
		t.Fatalf("ResolveProjectWing failed: %v", err)
	}
	second, err := ResolveProjectWing(filepath.Join(root, "."))
	if err != nil {
		t.Fatalf("ResolveProjectWing normalized path failed: %v", err)
	}
	if first != second {
		t.Fatalf("wing is not stable: %q != %q", first, second)
	}
	if !wingPattern.MatchString(first) {
		t.Fatalf("wing %q contains unsupported characters", first)
	}
}
