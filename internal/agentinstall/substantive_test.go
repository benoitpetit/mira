package agentinstall

import "testing"

func TestIsSubstantivePromptFiltersTransientChatter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "decision", input: "Use PostgreSQL for the production deployment and document the migration plan.", want: true},
		{name: "question", input: "Can you explain how the authentication boundary should work?", want: true},
		{name: "short", input: "ok", want: false},
		{name: "transient command", input: "/clear", want: false},
		{name: "listing", input: "ls -la", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSubstantivePrompt(tt.input, 20); got != tt.want {
				t.Fatalf("IsSubstantivePrompt(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
