// Package entities provides the core domain entities for the memory system.
// Candidate entity for Context Budget Allocation
package entities

import "github.com/google/uuid"

// Candidate represents a potential memory for selection in CBA
type Candidate struct {
	Memory        *Fingerprint
	Verbatim      *Verbatim
	Embedding     []float32
	Score         float64
	Relevance     float64
	Density       float64
	Recency       float64
	SessionBoost  float64
	MaxOverlap    float64
	CausalPenalty float64
}

// NewCandidate creates a candidate from memory, verbatim and embedding
func NewCandidate(memory *Fingerprint, verbatim *Verbatim, embedding []float32) *Candidate {
	return &Candidate{
		Memory:    memory,
		Verbatim:  verbatim,
		Embedding: embedding,
	}
}

// ID returns the memory ID for convenience
func (c *Candidate) ID() uuid.UUID {
	return c.Memory.ID
}

// WithScores sets all computed scores
func (c *Candidate) WithScores(relevance, density, recency float64) *Candidate {
	c.Relevance = relevance
	c.Density = density
	c.Recency = recency
	return c
}
