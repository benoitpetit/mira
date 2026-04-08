package vector

import (
	"math"
	"math/rand"
	"testing"

	"github.com/google/uuid"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float64
		tol      float64
	}{
		{
			name:     "identical vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 1.0,
			tol:      0.0001,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{0, 1, 0},
			expected: 0.0,
			tol:      0.0001,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{-1, 0, 0},
			expected: -1.0,
			tol:      0.0001,
		},
		{
			name:     "45 degree angle",
			a:        []float32{1, 0, 0},
			b:        []float32{0.7071, 0.7071, 0},
			expected: 0.7071,
			tol:      0.001,
		},
		{
			name:     "different lengths",
			a:        []float32{1, 0},
			b:        []float32{1, 0, 0},
			expected: 0.0,
			tol:      0.0001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cosineSimilarity(tt.a, tt.b)
			if math.Abs(result-tt.expected) > tt.tol {
				t.Errorf("cosineSimilarity() = %v, want %v (tol %v)", result, tt.expected, tt.tol)
			}
		})
	}
}

func TestCosineDistance(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}

	// Orthogonal vectors: similarity = 0, distance = 1
	dist := CosineDistance(a, b)
	if math.Abs(dist-1.0) > 0.0001 {
		t.Errorf("Expected distance 1.0 for orthogonal vectors, got %v", dist)
	}

	// Same vector: similarity = 1, distance = 0
	dist = CosineDistance(a, a)
	if math.Abs(dist-0.0) > 0.0001 {
		t.Errorf("Expected distance 0.0 for same vector, got %v", dist)
	}
}

func TestNormalize(t *testing.T) {
	vec := []float32{3, 4}
	normalized := Normalize(vec)

	// Check norm is 1
	var norm float64
	for _, v := range normalized {
		norm += float64(v * v)
	}
	norm = math.Sqrt(norm)
	if math.Abs(norm-1.0) > 0.0001 {
		t.Errorf("Expected norm 1.0, got %v", norm)
	}

	// Check direction is preserved
	if len(normalized) == 2 {
		ratio := float64(normalized[0]) / float64(normalized[1])
		expectedRatio := 0.75 // 3/4
		if math.Abs(ratio-expectedRatio) > 0.0001 {
			t.Errorf("Expected ratio %v, got %v", expectedRatio, ratio)
		}
	}
}

func TestNormalizeZeroVector(t *testing.T) {
	vec := []float32{0, 0, 0}
	normalized := Normalize(vec)

	// Should return same vector (or zero vector)
	for i, v := range normalized {
		if v != 0 {
			t.Errorf("Expected zero at index %d, got %v", i, v)
		}
	}
}

func TestSimpleOverlapCache(t *testing.T) {
	cache := NewSimpleOverlapCache()
	id1 := uuid.New()
	id2 := uuid.New()

	// Test Set and Get
	cache.Set(id1, id2, 0.75)

	val, ok := cache.Get(id1, id2)
	if !ok {
		t.Error("Expected to find value in cache")
	}
	if math.Abs(val-0.75) > 0.0001 {
		t.Errorf("Expected 0.75, got %v", val)
	}

	// Test reverse lookup (should be same due to key ordering)
	val, ok = cache.Get(id2, id1)
	if !ok {
		t.Error("Expected to find value with reversed IDs")
	}
	if math.Abs(val-0.75) > 0.0001 {
		t.Errorf("Expected 0.75 for reversed lookup, got %v", val)
	}

	// Test non-existent
	id3 := uuid.New()
	_, ok = cache.Get(id1, id3)
	if ok {
		t.Error("Should not find non-existent entry")
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		a, b, expected int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{5, 5, 5},
		{-1, 0, -1},
		{0, -1, -1},
	}

	for _, tt := range tests {
		result := min(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("min(%d, %d) = %d, expected %d", tt.a, tt.b, result, tt.expected)
		}
	}
}

// Benchmarks
func BenchmarkCosineSimilarity384(b *testing.B) {
	vec1 := make([]float32, 384)
	vec2 := make([]float32, 384)
	for i := 0; i < 384; i++ {
		vec1[i] = float32(i) / 384.0
		vec2[i] = float32(384-i) / 384.0
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cosineSimilarity(vec1, vec2)
	}
}

func BenchmarkNormalize(b *testing.B) {
	vec := make([]float32, 384)
	for i := 0; i < 384; i++ {
		vec[i] = float32(i)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Copy vector since Normalize modifies in place
		v := make([]float32, len(vec))
		copy(v, vec)
		Normalize(v)
	}
}

func BenchmarkSimpleOverlapCache(b *testing.B) {
	cache := NewSimpleOverlapCache()
	id1 := uuid.New()
	id2 := uuid.New()

	b.Run("Set", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			cache.Set(id1, id2, rand.Float64())
		}
	})

	b.Run("Get", func(b *testing.B) {
		cache.Set(id1, id2, 0.5)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cache.Get(id1, id2)
		}
	})

	b.Run("SetGet", func(b *testing.B) {
		ids := make([]uuid.UUID, 100)
		for i := range ids {
			ids[i] = uuid.New()
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			idx := i % 50
			if i%2 == 0 {
				cache.Set(ids[idx], ids[idx+1], rand.Float64())
			} else {
				cache.Get(ids[idx], ids[idx+1])
			}
		}
	})
}

func BenchmarkCosineDistance(b *testing.B) {
	vec1 := make([]float32, 384)
	vec2 := make([]float32, 384)
	for i := 0; i < 384; i++ {
		vec1[i] = rand.Float32()
		vec2[i] = rand.Float32()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CosineDistance(vec1, vec2)
	}
}
