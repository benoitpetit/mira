package storage

import "github.com/google/uuid"

// candidateIDsPerBatch stays below SQLite's historical 999-variable default.
// This is a conservative shared batch size, not a guarantee for every possible
// SQLite build or backend-specific parameter limit.
const candidateIDsPerBatch = 900

// candidateIDBatches preserves first-seen order while restoring SQL IN's set
// semantics across chunks. Both SQLite and PostgreSQL use these same bounded
// ID lists; callers that need ranking restore it after hydration.
func candidateIDBatches(ids []uuid.UUID) [][]uuid.UUID {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[uuid.UUID]struct{}, len(ids))
	unique := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}

	batches := make([][]uuid.UUID, 0, (len(unique)+candidateIDsPerBatch-1)/candidateIDsPerBatch)
	for start := 0; start < len(unique); start += candidateIDsPerBatch {
		end := start + candidateIDsPerBatch
		if end > len(unique) {
			end = len(unique)
		}
		batches = append(batches, unique[start:end])
	}
	return batches
}
