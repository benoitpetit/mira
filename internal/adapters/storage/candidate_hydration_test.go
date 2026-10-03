package storage

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestCandidateIDBatchesStayBelowLegacySQLiteAndPostgresParameterLimits(t *testing.T) {
	ids := make([]uuid.UUID, 1805)
	for i := range ids {
		ids[i] = uuid.New()
	}
	input := append([]uuid.UUID(nil), ids...)
	input = append(input, ids[0], ids[899], ids[900])

	batches := candidateIDBatches(input)
	if candidateIDsPerBatch > 999 {
		t.Fatalf("shared batch size = %d, exceeds the requested legacy SQLite compatibility cap", candidateIDsPerBatch)
	}
	var flattened []uuid.UUID
	for i, batch := range batches {
		if len(batch) == 0 || len(batch) > candidateIDsPerBatch {
			t.Fatalf("batch %d has %d IDs, want 1..%d", i, len(batch), candidateIDsPerBatch)
		}
		args := postgresUUIDArguments(batch)
		placeholders := postgresPlaceholders(1, len(args))
		if strings.Count(placeholders, "$") != len(args) || !strings.HasSuffix(placeholders, "$"+strconv.Itoa(len(args))) {
			t.Fatalf("PostgreSQL placeholder builder mismatch for batch %d with %d args: %q", i, len(args), placeholders)
		}
		flattened = append(flattened, batch...)
	}
	if !reflect.DeepEqual(flattened, ids) {
		t.Fatalf("batches changed first-seen ID order or failed to deduplicate: got %d IDs, want %d", len(flattened), len(ids))
	}
}
