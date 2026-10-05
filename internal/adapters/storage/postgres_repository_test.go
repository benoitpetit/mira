package storage

import (
	"strings"
	"testing"

	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

var _ ports.CausalRelationBatchReader = (*PostgreSQLRepository)(nil)

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

func TestBeliefProjectionRefreshLocksTheBeliefRowBeforeReadingSupports(t *testing.T) {
	query := beliefProjectionLockQuery()
	if !strings.Contains(query, "FOR UPDATE") || !strings.Contains(query, "WHERE id=$1") {
		t.Fatalf("belief projection lock query = %q, want a parameterized row lock", query)
	}
}

func TestCandidateUUIDAcceptsPostgreSQLDriverRepresentations(t *testing.T) {
	want := uuid.New()
	for _, value := range []any{want, want[:], want.String(), []byte(want.String())} {
		got, err := candidateUUID(value, true)
		if err != nil {
			t.Fatalf("candidateUUID(%T): %v", value, err)
		}
		if got != want {
			t.Errorf("candidateUUID(%T) = %s, want %s", value, got, want)
		}
	}
}
