package valueobjects

import "github.com/google/uuid"

// ConsolidationCandidateCursor is the keyset cursor for active session-note
// candidates ordered by extraction time and fingerprint ID, both descending.
type ConsolidationCandidateCursor struct {
	ExtractedAt   float64
	FingerprintID uuid.UUID
}

// ConsolidationCandidatePage contains one stable page of eligible notes.
type ConsolidationCandidatePage struct {
	Items []*TimelineItem
	Next  *ConsolidationCandidateCursor
}
