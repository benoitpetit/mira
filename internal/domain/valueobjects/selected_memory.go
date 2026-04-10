// SelectedMemory value object
package valueobjects

import (
	"time"

	"github.com/google/uuid"
)

// SelectedMemory represents a selected memory with render mode
type SelectedMemory struct {
	CandidateID uuid.UUID
	Mode        RenderMode
	TokenCost   int
	Rendered    string
	SelectedAt  time.Time
}

// NewSelectedMemory creates a selected memory
func NewSelectedMemory(candidateID uuid.UUID, mode RenderMode, tokenCost int, rendered string) *SelectedMemory {
	return &SelectedMemory{
		CandidateID: candidateID,
		Mode:        mode,
		TokenCost:   tokenCost,
		Rendered:    rendered,
		SelectedAt:  time.Now(),
	}
}
