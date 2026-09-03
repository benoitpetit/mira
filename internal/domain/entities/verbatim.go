// Verbatim entity - T0 layer (immutable, append-only)
package entities

import (
	"time"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

// Verbatim represents the raw, unprocessed memory content (T0)
type Verbatim struct {
	ID                uuid.UUID               `json:"id"`
	Content           string                  `json:"content"`
	TokenCount        int                     `json:"token_count"`
	CreatedAt         time.Time               `json:"created_at"`
	ValidFrom         *time.Time              `json:"valid_from,omitempty"`
	ValidUntil        *time.Time              `json:"valid_until,omitempty"`
	Kind              valueobjects.MemoryKind `json:"kind"`
	Wing              string                  `json:"wing"`
	Room              *string                 `json:"room,omitempty"`
	Metadata          map[string]any          `json:"metadata,omitempty"`
	Metrics           map[string]any          `json:"metrics,omitempty"`
	Summary           *string                 `json:"summary,omitempty"`
	SummaryTokenCount int                     `json:"summary_token_count,omitempty"`
}

// IsValidAt reports whether this memory is valid at the requested time. Both
// bounds are inclusive: a fact ending at an instant is still valid at that
// instant, and excluded immediately afterwards.
func (v *Verbatim) IsValidAt(at time.Time) bool {
	if v.ValidFrom != nil && at.Before(*v.ValidFrom) {
		return false
	}
	return v.ValidUntil == nil || !at.After(*v.ValidUntil)
}

// NewVerbatim creates a new verbatim with generated ID
func NewVerbatim(content, wing string, room *string) *Verbatim {
	return &Verbatim{
		ID:        uuid.New(),
		Content:   content,
		Kind:      valueobjects.KindKnowledge,
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
