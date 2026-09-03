package util

import "testing"

func TestNormalizeFTS5Query(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "natural language punctuation", query: "What database was selected?", want: `"What" OR "database" OR "was" OR "selected"`},
		{name: "operators are text", query: `alpha OR beta -gamma`, want: `"alpha" OR "OR" OR "beta" OR "gamma"`},
		{name: "unicode words", query: "mémoire partagée", want: `"mémoire" OR "partagée"`},
		{name: "punctuation only", query: `?!...`, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeFTS5Query(tt.query); got != tt.want {
				t.Fatalf("NormalizeFTS5Query(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}
