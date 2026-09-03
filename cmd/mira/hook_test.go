package main

import (
	"testing"

	"github.com/benoitpetit/mira/internal/config"
)

func TestPrepareHookConfigDisablesBackgroundServices(t *testing.T) {
	cfg := config.Default()
	cfg.Metrics.Enabled = true
	cfg.Webhooks.Enabled = true
	cfg.API.Enabled = true
	cfg.Soul.Enabled = true

	prepareHookConfig(cfg)

	if cfg.Metrics.Enabled || cfg.Webhooks.Enabled || cfg.API.Enabled || cfg.Soul.Enabled {
		t.Fatal("hook configuration must disable all background services")
	}
}

func TestPromptHookMessageSelectsAssistantResponseOnStop(t *testing.T) {
	role, content := promptHookMessage(claudeCodeHookInput{
		HookEventName:        "Stop",
		LastAssistantMessage: "The migration is complete.",
		Prompt:               "This must not be selected.",
	})
	if role != "assistant" || content != "The migration is complete." {
		t.Fatalf("stop message = (%q, %q), want assistant response", role, content)
	}
}

func TestPromptHookMessageSelectsUserPrompt(t *testing.T) {
	role, content := promptHookMessage(claudeCodeHookInput{UserInput: "Keep all data in the EU."})
	if role != "user" || content != "Keep all data in the EU." {
		t.Fatalf("prompt message = (%q, %q), want user input", role, content)
	}
}
