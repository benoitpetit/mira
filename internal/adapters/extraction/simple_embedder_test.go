package extraction

import (
	"context"
	"math"
	"testing"
)

func TestNewSimpleEmbedder(t *testing.T) {
	s := NewSimpleEmbedder(384)
	if s == nil {
		t.Fatal("NewSimpleEmbedder returned nil")
	}
	if s.dim != 384 {
		t.Errorf("dim = %d, want 384", s.dim)
	}
}

func TestSimpleEmbedderEncode(t *testing.T) {
	s := NewSimpleEmbedder(384)

	// Test basic encoding
	vec, err := s.Encode(context.Background(), "test text")
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	if len(vec) != 384 {
		t.Errorf("vector length = %d, want 384", len(vec))
	}

	// Verify L2 normalization (norm should be 1)
	var norm float64
	for _, v := range vec {
		norm += float64(v * v)
	}
	if math.Abs(norm-1.0) > 0.0001 {
		t.Errorf("L2 norm = %f, want 1.0", norm)
	}
}

func TestSimpleEmbedderDeterministic(t *testing.T) {
	s := NewSimpleEmbedder(384)

	// Same text should produce same embedding
	vec1, _ := s.Encode(context.Background(), "hello world")
	vec2, _ := s.Encode(context.Background(), "hello world")

	for i := range vec1 {
		if vec1[i] != vec2[i] {
			t.Error("Embeddings should be deterministic for same text")
			break
		}
	}
}

func TestSimpleEmbedderDifferentTexts(t *testing.T) {
	s := NewSimpleEmbedder(384)

	// Different texts should produce different embeddings
	vec1, _ := s.Encode(context.Background(), "hello world")
	vec2, _ := s.Encode(context.Background(), "goodbye world")

	same := true
	for i := range vec1 {
		if vec1[i] != vec2[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("Different texts should produce different embeddings")
	}
}

func TestSimpleEmbedderEmptyText(t *testing.T) {
	s := NewSimpleEmbedder(384)

	vec, err := s.Encode(context.Background(), "")
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	if len(vec) != 384 {
		t.Errorf("vector length = %d, want 384", len(vec))
	}

	// Should still be normalized
	var norm float64
	for _, v := range vec {
		norm += float64(v * v)
	}
	if math.Abs(norm-1.0) > 0.0001 {
		t.Errorf("L2 norm = %f, want 1.0", norm)
	}
}

func TestSimpleEmbedderNormalizeL2(t *testing.T) {
	s := NewSimpleEmbedder(3)

	tests := []struct {
		name     string
		input    []float32
		expected float64
	}{
		{
			name:     "unit vector",
			input:    []float32{1, 0, 0},
			expected: 1.0,
		},
		{
			name:     "simple vector",
			input:    []float32{3, 4, 0},
			expected: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.normalizeL2(tt.input)

			var norm float64
			for _, v := range result {
				norm += float64(v * v)
			}
			if math.Abs(norm-tt.expected) > 0.0001 {
				t.Errorf("L2 norm = %f, want %f", norm, tt.expected)
			}
		})
	}
}

func TestSimpleEmbedderNormalizeL2ZeroVector(t *testing.T) {
	s := NewSimpleEmbedder(3)
	vec := []float32{0, 0, 0}

	result := s.normalizeL2(vec)

	// Zero vector should remain zero
	for i, v := range result {
		if v != 0 {
			t.Errorf("result[%d] = %f, want 0", i, v)
		}
	}
}

func TestHashString(t *testing.T) {
	// Same string should produce same hash
	h1 := hashString("test")
	h2 := hashString("test")
	if h1 != h2 {
		t.Error("hashString should be deterministic")
	}

	// Different strings should produce different hashes (highly likely)
	h3 := hashString("different")
	if h1 == h3 {
		t.Error("Different strings should produce different hashes")
	}

	// Empty string
	h4 := hashString("")
	if h4 == 0 {
		t.Error("Empty string hash should not be zero")
	}
}

func BenchmarkSimpleEmbedderEncode(b *testing.B) {
	s := NewSimpleEmbedder(384)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Encode(context.Background(), "benchmark text for embedding generation")
	}
}

func BenchmarkHashString(b *testing.B) {
	text := "benchmark text for hashing"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hashString(text)
	}
}
