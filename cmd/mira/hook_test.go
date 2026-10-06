package main

import (
	"strings"
	"testing"

	"github.com/benoitpetit/mira/internal/config"
)

func TestPrepareHookConfigDisablesBackgroundServices(t *testing.T) {
	cfg := config.Default()
	cfg.Metrics.Enabled = true
	cfg.Webhooks.Enabled = true
	cfg.API.Enabled = true
	cfg.AgentMemory.Enabled = true

	prepareHookConfig(cfg)

	if cfg.Metrics.Enabled || cfg.Webhooks.Enabled || cfg.API.Enabled || !cfg.AgentMemory.Enabled {
		t.Fatal("hook configuration must disable all background services")
	}
}

func TestRedactHookCaptureContentRemovesSecretsBeforeStorage(t *testing.T) {
	content, ok := redactHookCaptureContent("Remember the deployment bearer sk-test-secret-1234567890 for later.", 20)
	if !ok {
		t.Fatal("substantive hook content was discarded")
	}
	if strings.Contains(content, "sk-test-secret") || !strings.Contains(content, "[REDACTED") {
		t.Fatalf("hook capture leaked a secret: %q", content)
	}
}

func TestPromptHookMessageSelectsAssistantResponseOnStop(t *testing.T) {
	role, content := promptHookMessage(&claudeCodeHookInput{
		HookEventName:        "Stop",
		LastAssistantMessage: "The migration is complete.",
		Prompt:               "This must not be selected.",
	})
	if role != "assistant" || content != "The migration is complete." {
		t.Fatalf("stop message = (%q, %q), want assistant response", role, content)
	}
}

func TestPromptHookMessageSelectsUserPrompt(t *testing.T) {
	role, content := promptHookMessage(&claudeCodeHookInput{UserInput: "Keep all data in the EU."})
	if role != "user" || content != "Keep all data in the EU." {
		t.Fatalf("prompt message = (%q, %q), want user input", role, content)
	}
}
