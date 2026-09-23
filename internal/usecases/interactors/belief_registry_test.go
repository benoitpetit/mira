package interactors

import (
	"testing"

	"github.com/benoitpetit/mira/internal/domain/entities"
)

func TestBeliefRegistryResolvesAndBoundsFeedbackCalibration(t *testing.T) {
	registry := NewBeliefRegistry()
	id := registry.Upsert(entities.Belief{Subject: "Mira", Predicate: "style", Value: "direct", Confidence: .8})
	if _, err := registry.Resolve("Mira", "style"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if err := registry.RecordFeedback(id, entities.FeedbackContradictory); err != nil {
			t.Fatal(err)
		}
	}
	if got := registry.Calibration(id); got < .75 || got > 1.2 {
		t.Fatalf("calibration escaped bounds: %.3f", got)
	}
	if err := registry.Retract(id, true); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve("Mira", "style"); err == nil {
		t.Fatal("contested belief was still resolved")
	}
}
