package util

import "testing"

func TestComputeModelHash(t *testing.T) {
	tests := []struct {
		name      string
		modelName string
	}{
		{"non-empty", "sentence-transformers/all-MiniLM-L6-v2"},
		{"empty", ""},
		{"short", "llama3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := ComputeModelHash(tt.modelName)
			// Must be exactly 16 hex chars (8 bytes)
			if len(h) != 16 {
				t.Errorf("ComputeModelHash(%q) len = %d, want 16", tt.modelName, len(h))
			}
			// Must be deterministic
			h2 := ComputeModelHash(tt.modelName)
			if h != h2 {
				t.Errorf("ComputeModelHash not deterministic: %q != %q", h, h2)
			}
		})
	}

	// Different inputs must produce different hashes
	h1 := ComputeModelHash("model-a")
	h2 := ComputeModelHash("model-b")
	if h1 == h2 {
		t.Error("different model names produced the same hash")
	}
}
