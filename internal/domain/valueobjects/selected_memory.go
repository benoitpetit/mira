// SelectedMemory value object
package valueobjects

import (
	"time"

	"github.com/google/uuid"
)

// SelectedMemory represents a selected memory with render mode
type SelectedMemory struct {
	CandidateID uuid.UUID  `json:"candidate_id"`
	VerbatimID  uuid.UUID  `json:"verbatim_id"`
	Mode        RenderMode `json:"mode"`
	TokenCost   int        `json:"token_cost"`
	Rendered    string     `json:"rendered"`
	Confidence  float64    `json:"confidence,omitempty"`
	SelectedAt  time.Time  `json:"selected_at"`
}

// NewSelectedMemory creates a selected memory
func NewSelectedMemory(candidateID, verbatimID uuid.UUID, mode RenderMode, tokenCost int, rendered string, confidence float64) *SelectedMemory {
	return &SelectedMemory{
		CandidateID: candidateID,
		VerbatimID:  verbatimID,
		Mode:        mode,
		TokenCost:   tokenCost,
		Rendered:    rendered,
		Confidence:  confidence,
		SelectedAt:  time.Now(),
	}
}
