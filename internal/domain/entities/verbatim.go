// Verbatim entity - T0 layer (immutable, append-only)
package entities

import (
	"time"

	"github.com/google/uuid"
)

// Verbatim represents the raw, unprocessed memory content (T0)
type Verbatim struct {
	ID         uuid.UUID
	Content    string
	TokenCount int
	CreatedAt  time.Time
	Wing       string
	Room       *string
	Metadata   map[string]any
	Metrics    map[string]any
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
