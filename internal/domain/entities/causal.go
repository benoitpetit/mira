// Causal graph entities
package entities

import (
	"time"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

// CausalNode represents a node in the causal graph
type CausalNode struct {
	ID        uuid.UUID `json:"id"`
	Type      string    `json:"type"`
	Summary   string    `json:"summary"`
	Timestamp time.Time `json:"timestamp"`
	Wing      string    `json:"wing"`
	Room      *string   `json:"room,omitempty"`
}

// NewCausalNode creates a new causal node
func NewCausalNode(id uuid.UUID, nodeType, summary, wing string, room *string) *CausalNode {
	return &CausalNode{
		ID:        id,
		Type:      nodeType,
		Summary:   summary,
		Timestamp: time.Now(),
		Wing:      wing,
		Room:      room,
	}
}

// CausalEdge represents a directed edge in the causal graph
type CausalEdge struct {
	FromID     uuid.UUID
	ToID       uuid.UUID
	Relation   valueobjects.RelationType
	Weight     float64
	DetectedAt time.Time
}

// NewCausalEdge creates a new causal edge
func NewCausalEdge(fromID, toID uuid.UUID, relation valueobjects.RelationType) *CausalEdge {
	return &CausalEdge{
		FromID:     fromID,
		ToID:       toID,
		Relation:   relation,
		Weight:     0.7,
		DetectedAt: time.Now(),
	}
}
