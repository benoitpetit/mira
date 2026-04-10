package util

import (
	"math"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float64
	}{
		{
			name:     "identical unit vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 1.0,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{-1, 0, 0},
			expected: -1.0,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{0, 1, 0},
			expected: 0.0,
		},
		{
			name:     "45 degree angle normalized",
			a:        []float32{1, 0},
			b:        []float32{0.707107, 0.707107},
			expected: 0.707107,
		},
		{
			name:     "different lengths",
			a:        []float32{1, 0, 0},
			b:        []float32{1, 0},
			expected: 0.0,
		},
		{
			name:     "empty vectors",
			a:        []float32{},
			b:        []float32{},
			expected: 0.0,
		},
		{
			name:     "zero vector",
			a:        []float32{0, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CosineSimilarity(tt.a, tt.b)
			diff := math.Abs(result - tt.expected)
			if diff > 0.0001 {
				t.Errorf("CosineSimilarity() = %v, want %v (diff: %v)", result, tt.expected, diff)
			}
		})
	}
}

func TestCosineSimilarityWithNormalizedVectors(t *testing.T) {
	// Using pre-normalized vectors (L2 norm = 1)
	a := []float32{0.707107, 0.707107, 0} // 45 degrees in XY plane, normalized
	b := []float32{1, 0, 0}               // Unit vector along X

	result := CosineSimilarity(a, b)
	expected := 0.707107

	if math.Abs(result-expected) > 0.0001 {
		t.Errorf("CosineSimilarity() = %v, want %v", result, expected)
	}
}

func BenchmarkCosineSimilarity(b *testing.B) {
	a := make([]float32, 384)
	vecB := make([]float32, 384)
	for i := 0; i < 384; i++ {
		a[i] = float32(i) / 384.0
		vecB[i] = float32(384-i) / 384.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CosineSimilarity(a, vecB)
	}
}
