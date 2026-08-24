// Verbatim entity - T0 layer (immutable, append-only)
package entities

import (
	"time"

	"github.com/google/uuid"
)

// Verbatim represents the raw, unprocessed memory content (T0)
type Verbatim struct {
	ID                uuid.UUID      `json:"id"`
	Content           string         `json:"content"`
	TokenCount        int            `json:"token_count"`
	CreatedAt         time.Time      `json:"created_at"`
	Wing              string         `json:"wing"`
	Room              *string        `json:"room,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	Metrics           map[string]any `json:"metrics,omitempty"`
	Summary           *string        `json:"summary,omitempty"`
	SummaryTokenCount int            `json:"summary_token_count,omitempty"`
}

// NewVerbatim creates a new verbatim with generated ID
func NewVerbatim(content, wing string, room *string) *Verbatim {
	return &Verbatim{
		ID:        uuid.New(),
		Content:   content,
		Wing:      wing,
		Room:      room,
		CreatedAt: time.Now(),
		Metadata:  make(map[string]any),
		Metrics:   make(map[string]any),
	}
}

// WithTokenCount sets the token count (after tokenization)
func (v *Verbatim) WithTokenCount(count int) *Verbatim {
	v.TokenCount = count
	return v
}

// HasSummary returns true when a compressed summary is available.
func (v *Verbatim) HasSummary() bool {
	return v.Summary != nil && *v.Summary != ""
}
