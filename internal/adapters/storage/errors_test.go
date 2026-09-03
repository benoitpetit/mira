package storage

import (
	"errors"
	"fmt"
	"testing"
)

func TestNotFoundErrorSurvivesWrapping(t *testing.T) {
	err := fmt.Errorf("load failed: %w", &NotFoundError{Resource: "verbatim"})
	var classified interface{ IsNotFound() bool }
	if !errors.As(err, &classified) || !classified.IsNotFound() {
		t.Fatalf("wrapped error was not classified as not found: %v", err)
	}
}
