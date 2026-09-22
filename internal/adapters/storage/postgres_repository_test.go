package storage

import (
	"testing"

	"github.com/google/uuid"
)

func TestPostgresPlaceholders(t *testing.T) {
	if got, want := postgresPlaceholders(3, 3), "$3, $4, $5"; got != want {
		t.Fatalf("postgresPlaceholders() = %q, want %q", got, want)
	}
}

func TestPostgresUUIDArgumentsAreDriverPortable(t *testing.T) {
	id := uuid.New()
	args := postgresUUIDArguments([]uuid.UUID{id})
	if len(args) != 1 || args[0] != id.String() {
		t.Fatalf("postgresUUIDArguments() = %#v, want UUID string", args)
	}
}
