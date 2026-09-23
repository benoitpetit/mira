package entities

import (
	"time"

	"github.com/google/uuid"
)

type BeliefStatus string

const (
	BeliefActive     BeliefStatus = "active"
	BeliefSuperseded BeliefStatus = "superseded"
	BeliefContested  BeliefStatus = "contested"
	BeliefRevoked    BeliefStatus = "revoked"
)

type Belief struct {
	ID         uuid.UUID    `json:"id"`
	Subject    string       `json:"subject"`
	Predicate  string       `json:"predicate"`
	Value      string       `json:"value"`
	ValidFrom  *time.Time   `json:"valid_from,omitempty"`
	ValidUntil *time.Time   `json:"valid_until,omitempty"`
	Confidence float64      `json:"confidence"`
	Sources    []uuid.UUID  `json:"sources"`
	Status     BeliefStatus `json:"status"`
	UpdatedAt  time.Time    `json:"updated_at"`
}

type BeliefFeedback string

const (
	FeedbackUseful        BeliefFeedback = "useful"
	FeedbackStale         BeliefFeedback = "stale"
	FeedbackContradictory BeliefFeedback = "contradictory"
	FeedbackIrrelevant    BeliefFeedback = "irrelevant"
)
