// Embedding entity - T2 layer (vector representation)
package entities

import (
	"time"

	"github.com/google/uuid"
)

// Embedding represents the vector representation of a verbatim (T2)
type Embedding struct {
	ID         uuid.UUID
	ModelHash  string
	Dim        int
	Vector     []float32
	Normalized bool
	CreatedAt  time.Time
}

// NewEmbedding creates a new embedding
func NewEmbedding(verbatimID uuid.UUID, modelHash string, vector []float32) *Embedding {
	return &Embedding{
		ID:         verbatimID,
		ModelHash:  modelHash,
		Dim:        len(vector),
		Vector:     vector,
		Normalized: false,
		CreatedAt:  time.Now(),
	}
}

// Normalized returns a normalized copy of the embedding
func (e *Embedding) WithNormalization() *Embedding {
	norm := float32(0)
	for _, v := range e.Vector {
		norm += v * v
	}
	if norm == 0 {
		e.Normalized = true
		return e
	}

	norm = 1.0 / sqrt(norm)
	for i := range e.Vector {
		e.Vector[i] *= norm
	}
	e.Normalized = true
	return e
}

func sqrt(x float32) float32 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}
