package interactors

import (
	"testing"
)

func TestExpandQuery(t *testing.T) {
	tests := []struct {
		query       string
		numVariants int
		minLen      int
		maxLen      int
	}{
		{"hello world", 2, 1, 1},
		{"The quick brown fox jumps over the lazy dog", 3, 2, 4},
		{"", 3, 1, 1},
		{"short", 0, 1, 1},
	}

	for _, tt := range tests {
		variants := expandQuery(tt.query, tt.numVariants)
		if len(variants) < tt.minLen || len(variants) > tt.maxLen {
			t.Errorf("expandQuery(%q, %d) returned %d variants, want between %d and %d",
				tt.query, tt.numVariants, len(variants), tt.minLen, tt.maxLen)
		}
		// Original query must be first
		if len(variants) > 0 && variants[0] != tt.query {
			t.Errorf("expandQuery(%q, %d) first variant was %q, want original query",
				tt.query, tt.numVariants, variants[0])
		}
	}
}

func TestExpandQuery_RemovesStopWords(t *testing.T) {
	variants := expandQuery("The system is running", 3)
	foundNoStop := false
	for _, v := range variants {
		if v == "system running" {
			foundNoStop = true
		}
	}
	if !foundNoStop {
		t.Errorf("expected a variant without stop words, got %v", variants)
	}
}
