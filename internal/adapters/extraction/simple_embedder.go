// Simple embedder adapter (pseudo-random for testing)
package extraction

import (
	"context"
	"math"

	"github.com/benoitpetit/mira/internal/usecases/ports"
)

// SimpleEmbedder generates deterministic pseudo-random embeddings
type SimpleEmbedder struct {
	dim int
}

// NewSimpleEmbedder creates a new simple embedder
func NewSimpleEmbedder(dim int) *SimpleEmbedder {
	return &SimpleEmbedder{dim: dim}
}

// Encode implements Embedder
func (s *SimpleEmbedder) Encode(ctx context.Context, text string) ([]float32, error) {
	vec := make([]float32, s.dim)
	seed := hashString(text)
	for i := 0; i < s.dim; i++ {
		vec[i] = float32((seed+uint64(i)*6364136223846793005)&0xFFFFFFFF) / float32(0xFFFFFFFF)
	}
	return s.normalizeL2(vec), nil
}

func (s *SimpleEmbedder) normalizeL2(v []float32) []float32 {
	var norm float32
	for _, x := range v {
		norm += x * x
	}
	norm = float32(math.Sqrt(float64(norm)))

	if norm == 0 {
		return v
	}

	for i := range v {
		v[i] /= norm
	}
	return v
}

func hashString(s string) uint64 {
	var h uint64 = 14695981039346656037
	for _, c := range s {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return h
}

// Ensure SimpleEmbedder implements Embedder
var _ ports.Embedder = (*SimpleEmbedder)(nil)
