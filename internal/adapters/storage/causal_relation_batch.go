package storage

import "github.com/google/uuid"

// Keep each of the two endpoint lists below SQLite's conservative 999 bind
// variable limit, regardless of the SQL dialect using the shared batch size.
const causalRelationIDBatchSize = 400

func stableUniqueUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	unique := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}
