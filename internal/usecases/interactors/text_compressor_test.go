package interactors

import (
	"strings"
	"testing"
)

func TestCompressText_FillerPhrases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(string) bool
	}{
		{
			name:  "in order to",
			input: "Use JWT in order to secure the API.",
			check: func(s string) bool { return strings.Contains(s, "to secure") && !strings.Contains(s, "in order to") },
		},
		{
			name:  "please note that",
			input: "Please note that the database connection pool is set to 100.",
			check: func(s string) bool { return !strings.Contains(s, "Please note that") },
		},
		{
			name:  "due to the fact that",
			input: "We switched due to the fact that performance was poor.",
			check: func(s string) bool { return strings.Contains(s, "because") && !strings.Contains(s, "due to the fact that") },
		},
		{
			name:  "in order to lowercase",
			input: "We decided in order to reduce latency.",
			check: func(s string) bool { return !strings.Contains(s, "in order to") },
		},
		{
			name:  "preserves technical content",
			input: "Fixed nil pointer dereference in user.go:42 in order to prevent crash.",
			check: func(s string) bool {
				return strings.Contains(s, "nil pointer") && strings.Contains(s, "user.go:42")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompressText(tt.input)
			if !tt.check(result) {
				t.Errorf("CompressText(%q) = %q, check failed", tt.input, result)
			}
		})
	}
}

func TestCompressText_ShorterThanInput(t *testing.T) {
	input := "In order to implement the authentication system, please note that we need to use JWT tokens. " +
		"Due to the fact that the previous system was insecure, we have been forced to migrate. " +
		"It is important to note that all existing sessions will be invalidated in the event that the migration succeeds."
	result := CompressText(input)
	if len(result) >= len(input) {
		t.Errorf("CompressText result (%d chars) not shorter than input (%d chars)\nresult: %q", len(result), len(input), result)
	}
}

func TestCompressText_EmptyInput(t *testing.T) {
	if CompressText("") != "" {
		t.Error("CompressText(\"\") should return \"\"")
	}
}

func TestCompressText_PreservesContent(t *testing.T) {
	// Technical content should survive unchanged
	input := "PostgreSQL connection pool max_connections=100 SET search_path=public"
	result := CompressText(input)
	if !strings.Contains(result, "PostgreSQL") || !strings.Contains(result, "max_connections=100") {
		t.Errorf("CompressText modified technical content: %q", result)
	}
}

func TestEstimateSummaryTokens(t *testing.T) {
	if EstimateSummaryTokens("hello world foo") != 3 {
		t.Error("EstimateSummaryTokens: expected 3 words")
	}
	if EstimateSummaryTokens("") != 0 {
		t.Error("EstimateSummaryTokens: expected 0 for empty")
	}
}
