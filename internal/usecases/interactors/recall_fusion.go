package interactors

import (
	"sort"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/google/uuid"
)

// reciprocalRankFusion merges dense and lexical search results using RRF.
// k is the RRF constant (default 60).
func reciprocalRankFusion(dense []*entities.Candidate, lexical []*entities.Candidate, k int) []*entities.Candidate {
	if k <= 0 {
		k = 60
	}

	scores := make(map[uuid.UUID]float64)
	ranks := make(map[uuid.UUID]struct {
		dense   int
		lexical int
	})
	candidatesByID := make(map[uuid.UUID]*entities.Candidate)

	// Assign dense ranks
	for i, c := range dense {
		id := c.ID()
		scores[id] += 1.0 / (float64(k) + float64(i+1))
		r := ranks[id]
		r.dense = i + 1
		ranks[id] = r
		candidatesByID[id] = c
	}

	// Assign lexical ranks
	for i, c := range lexical {
		id := c.ID()
		scores[id] += 1.0 / (float64(k) + float64(i+1))
		r := ranks[id]
		r.lexical = i + 1
		ranks[id] = r
		if _, ok := candidatesByID[id]; !ok {
			candidatesByID[id] = c
		}
	}

	// Build result list
	type result struct {
		candidate *entities.Candidate
		score     float64
	}
	var results []result
	for id, score := range scores {
		c := candidatesByID[id]
		results = append(results, result{candidate: c, score: score})
	}

	// Sort by RRF score descending
	sort.Slice(results, func(i, j int) bool {
		if results[i].score == results[j].score {
			// Tie-break: prefer candidates present in both lists
			rI := ranks[results[i].candidate.ID()]
			rJ := ranks[results[j].candidate.ID()]
			bothI := 0
			bothJ := 0
			if rI.dense > 0 {
				bothI++
			}
			if rI.lexical > 0 {
				bothI++
			}
			if rJ.dense > 0 {
				bothJ++
			}
			if rJ.lexical > 0 {
				bothJ++
			}
			return bothI > bothJ
		}
		return results[i].score > results[j].score
	})

	// Extract candidates
	fused := make([]*entities.Candidate, 0, len(results))
	for _, r := range results {
		fused = append(fused, r.candidate)
	}
	return fused
}
