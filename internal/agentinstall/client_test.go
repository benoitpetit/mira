package agentinstall

import "testing"

func TestLookupClientDeclaresEverySupportedAgent(t *testing.T) {
	tests := []struct {
		id      string
		recall  IntegrationMode
		capture IntegrationMode
	}{
		{"codex", IntegrationHook, IntegrationHook},
		{"claude-code", IntegrationHook, IntegrationHook},
		{"windsurf", IntegrationSkillGuided, IntegrationHook},
		{"cursor", IntegrationSkillGuided, IntegrationNone},
		{"claude-desktop", IntegrationSkillGuided, IntegrationNone},
		{"hermes", IntegrationSkillGuided, IntegrationNone},
		{"opencode", IntegrationSkillGuided, IntegrationNone},
		{"pi", IntegrationSkillGuided, IntegrationNone},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			client, ok := LookupClient(tt.id)
			if !ok {
				t.Fatalf("LookupClient(%q) returned no client", tt.id)
			}
			if client.RecallMode != tt.recall || client.CaptureMode != tt.capture {
				t.Fatalf("modes = recall:%s capture:%s, want recall:%s capture:%s", client.RecallMode, client.CaptureMode, tt.recall, tt.capture)
			}
			if !client.SupportsScope(ScopeProject) || !client.SupportsScope(ScopeUser) {
				t.Fatalf("client %q must support project and user scopes", tt.id)
			}
		})
	}
}

func TestClientModesReflectPolicy(t *testing.T) {
	client, ok := LookupClient("codex")
	if !ok {
		t.Fatal("codex client missing")
	}

	minimalRecall, minimalCapture := client.Modes(PolicyMinimal)
	if minimalRecall != IntegrationSkillGuided || minimalCapture != IntegrationNone {
		t.Fatalf("minimal modes = %s/%s, want skill-guided/none", minimalRecall, minimalCapture)
	}
	standardRecall, standardCapture := client.Modes(PolicyStandard)
	if standardRecall != IntegrationHook || standardCapture != IntegrationHook {
		t.Fatalf("standard modes = %s/%s, want hook/hook", standardRecall, standardCapture)
	}
}
