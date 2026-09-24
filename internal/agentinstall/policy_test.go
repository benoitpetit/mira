package agentinstall

import "testing"

func TestPolicyCapabilities(t *testing.T) {
	tests := []struct {
		name      string
		policy    Policy
		user      bool
		assistant bool
		inject    bool
	}{
		{name: "minimal", policy: PolicyMinimal, inject: false},
		{name: "standard", policy: PolicyStandard, user: true, inject: true},
		{name: "complete", policy: PolicyComplete, user: true, assistant: true, inject: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, err := ParsePolicy(string(tt.policy))
			if err != nil {
				t.Fatalf("ParsePolicy failed: %v", err)
			}
			if policy.CaptureUserPrompts() != tt.user || policy.CaptureAssistantResponses() != tt.assistant || policy.InjectionEnabled() != tt.inject {
				t.Fatalf("unexpected capabilities for %s", tt.policy)
			}
		})
	}
}

func TestParsePolicyRejectsUnknownValues(t *testing.T) {
	if _, err := ParsePolicy("reckless"); err == nil {
		t.Fatal("ParsePolicy accepted unknown policy")
	}
}
