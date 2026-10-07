package agentinstall

import "testing"

func TestLookupClientDeclaresEverySupportedAgent(t *testing.T) {
	tests := []struct {
		id      string
		recall  IntegrationMode
		capture IntegrationMode
		project bool
	}{
		{"codex", IntegrationHook, IntegrationHook, true},
		{"claude-code", IntegrationHook, IntegrationHook, true},
		{"windsurf", IntegrationSkillGuided, IntegrationHook, true},
		{"cursor", IntegrationSkillGuided, IntegrationNone, true},
		{"claude-desktop", IntegrationSkillGuided, IntegrationNone, false},
		{"hermes", IntegrationSkillGuided, IntegrationNone, true},
		{"opencode", IntegrationSkillGuided, IntegrationNone, true},
		{"pi", IntegrationSkillGuided, IntegrationNone, true},
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
			if client.SupportsScope(ScopeProject) != tt.project || !client.SupportsScope(ScopeUser) {
				t.Fatalf("client %q scope declaration is incorrect", tt.id)
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

func TestClientExecutableContract(t *testing.T) {
	tests := []struct {
		client string
		want   string
	}{
		{client: "codex", want: "codex"},
		{client: "claude-code", want: "claude"},
		{client: "cursor", want: ""},
		{client: "claude-desktop", want: ""},
		{client: "hermes", want: ""},
		{client: "opencode", want: ""},
		{client: "pi", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.client, func(t *testing.T) {
			client, ok := LookupClient(tt.client)
			if !ok {
				t.Fatalf("client %q is not registered", tt.client)
			}
			if client.Executable != tt.want {
				t.Errorf("Executable = %q, want %q", client.Executable, tt.want)
			}
		})
	}
}
