package interactors

import (
	"fmt"
	"strings"
	"sync"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/google/uuid"
)

// BeliefRegistry is a local, deterministic view over versioned assertions. It
// never replaces T0/T1/T2; it only resolves their latest coherent statement.
type BeliefRegistry struct {
	mu       sync.RWMutex
	beliefs  map[uuid.UUID]*entities.Belief
	feedback map[uuid.UUID]map[entities.BeliefFeedback]int
}

func NewBeliefRegistry() *BeliefRegistry {
	return &BeliefRegistry{beliefs: map[uuid.UUID]*entities.Belief{}, feedback: map[uuid.UUID]map[entities.BeliefFeedback]int{}}
}

func (r *BeliefRegistry) Upsert(belief entities.Belief) uuid.UUID {
	r.mu.Lock()
	defer r.mu.Unlock()
	if belief.ID == uuid.Nil {
		belief.ID = uuid.New()
	}
	if belief.Status == "" {
		belief.Status = entities.BeliefActive
	}
	if belief.Confidence < 0 {
		belief.Confidence = 0
	}
	if belief.Confidence > 1 {
		belief.Confidence = 1
	}
	if belief.Sources == nil {
		belief.Sources = []uuid.UUID{}
	}
	r.beliefs[belief.ID] = &belief
	return belief.ID
}

func (r *BeliefRegistry) Resolve(subject, predicate string) (*entities.Belief, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var best *entities.Belief
	for _, belief := range r.beliefs {
		if belief.Status != entities.BeliefActive || !strings.EqualFold(belief.Subject, subject) || !strings.EqualFold(belief.Predicate, predicate) {
			continue
		}
		if best == nil || belief.Confidence > best.Confidence || belief.UpdatedAt.After(best.UpdatedAt) {
			copy := *belief
			best = &copy
		}
	}
	if best == nil {
		return nil, fmt.Errorf("belief not found")
	}
	return best, nil
}

func (r *BeliefRegistry) Retract(id uuid.UUID, contested bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	belief, ok := r.beliefs[id]
	if !ok {
		return fmt.Errorf("belief not found")
	}
	if contested {
		belief.Status = entities.BeliefContested
	} else {
		belief.Status = entities.BeliefRevoked
	}
	return nil
}

func (r *BeliefRegistry) RecordFeedback(id uuid.UUID, feedback entities.BeliefFeedback) error {
	switch feedback {
	case entities.FeedbackUseful, entities.FeedbackStale, entities.FeedbackContradictory, entities.FeedbackIrrelevant:
	default:
		return fmt.Errorf("unsupported belief feedback %q", feedback)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.beliefs[id]; !ok {
		return fmt.Errorf("belief not found")
	}
	if r.feedback[id] == nil {
		r.feedback[id] = map[entities.BeliefFeedback]int{}
	}
	r.feedback[id][feedback]++
	return nil
}

func (r *BeliefRegistry) Calibration(id uuid.UUID) float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	counts := r.feedback[id]
	if len(counts) == 0 {
		return 1
	}
	useful := counts[entities.FeedbackUseful]
	stale := counts[entities.FeedbackStale]
	contradictory := counts[entities.FeedbackContradictory]
	irrelevant := counts[entities.FeedbackIrrelevant]
	total := useful + stale + contradictory + irrelevant
	if total == 0 {
		return 1
	}
	value := 1 + 0.2*float64(useful)/float64(total) - 0.2*float64(stale+contradictory+irrelevant)/float64(total)
	if value < 0.75 {
		return 0.75
	}
	if value > 1.2 {
		return 1.2
	}
	return value
}
