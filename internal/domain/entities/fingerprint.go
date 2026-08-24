// Fingerprint entity - T1 layer (structured extraction)
package entities

import (
	"time"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

// Fingerprint represents the structured extraction from verbatim (T1)
type Fingerprint struct {
	ID            uuid.UUID
	VerbatimID    uuid.UUID
	Type          valueobjects.MemoryType
	ExtractedAt   time.Time
	Entities      []string
	Subjects      []string
	Decision      *string
	Data          valueobjects.FingerprintData
	FactCount     int
	TokenEstimate int
	ModelHash     string
}

// NewFingerprint creates a fingerprint from a verbatim
func NewFingerprint(verbatimID uuid.UUID, memType valueobjects.MemoryType, modelHash string) *Fingerprint {
	return &Fingerprint{
		ID:          uuid.New(),
		VerbatimID:  verbatimID,
		Type:        memType,
		ExtractedAt: time.Now(),
		Entities:    make([]string, 0),
		Subjects:    make([]string, 0),
		ModelHash:   modelHash,
	}
}

// WithData sets the fingerprint data and updates fact count
func (f *Fingerprint) WithData(data valueobjects.FingerprintData) *Fingerprint {
	f.Data = data
	f.Entities = data.Entities
	f.Subjects = data.Subject
	if data.Decision != "" {
		f.Decision = &data.Decision
	}
	return f
}

// WithTokenEstimate sets the estimated token count
func (f *Fingerprint) WithTokenEstimate(tokens int) *Fingerprint {
	f.TokenEstimate = tokens
	return f
}

// CalculateFactCount computes the number of facts in this fingerprint
func (f *Fingerprint) CalculateFactCount() int {
	count := 0
	if f.Data.Decision != "" {
		count++
	}
	count += len(f.Data.Rejected)
	count += len(f.Data.Reason)
	if f.Data.ValidatedBy != "" {
		count++
	}
	if f.Data.Assignee != "" {
		count++
	}
	if f.Data.Deadline != "" {
		count++
	}
	if len(f.Data.Subject) > 0 && f.Data.Subject[0] != "" {
		count++
	}
	f.FactCount = count
	return count
}
