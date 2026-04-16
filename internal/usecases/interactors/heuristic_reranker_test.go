package interactors

import (
	"context"
	"testing"
)

func TestHeuristicReranker(t *testing.T) {
	rr := NewHeuristicReranker()
	query := "database connection"
	candidates := []string{
		"How to establish a database connection",
		"The weather is sunny today",
		"Connection string for the database",
	}

	scores, err := rr.Rerank(context.Background(), query, candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scores) != len(candidates) {
		t.Fatalf("expected %d scores, got %d", len(candidates), len(scores))
	}

	// First and third candidates should score higher than the second
	if scores[0] <= scores[1] {
		t.Errorf("expected first candidate to score higher than second: %f vs %f", scores[0], scores[1])
	}
	if scores[2] <= scores[1] {
		t.Errorf("expected third candidate to score higher than second: %f vs %f", scores[2], scores[1])
	}
}

func TestHeuristicReranker_Empty(t *testing.T) {
	rr := NewHeuristicReranker()
	scores, err := rr.Rerank(context.Background(), "test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scores) != 0 {
		t.Errorf("expected empty scores, got %v", scores)
	}
}
