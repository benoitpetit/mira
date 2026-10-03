package interactors

import (
	"sort"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/google/uuid"
)

// reciprocalRankFusion merges dense and lexical search results using RRF.
// k is the RRF constant (default 60).
func reciprocalRankFusion(dense, lexical []*entities.Candidate, k int) []*entities.Candidate {
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
	denseSeen := make(map[uuid.UUID]bool, len(dense))
	denseRank := 0
	for _, c := range dense {
		if c == nil || c.Memory == nil {
			continue
		}
		id := c.ID()
		if denseSeen[id] {
			continue
		}
		denseSeen[id] = true
		addRetrievalSource(c, "dense")
		denseRank++
		scores[id] += 1.0 / (float64(k) + float64(denseRank))
		r := ranks[id]
		r.dense = denseRank
		ranks[id] = r
		candidatesByID[id] = c
	}

	// Assign lexical ranks
	lexicalSeen := make(map[uuid.UUID]bool, len(lexical))
	lexicalRank := 0
	for _, c := range lexical {
		if c == nil || c.Memory == nil {
			continue
		}
		id := c.ID()
		if lexicalSeen[id] {
			continue
		}
		lexicalSeen[id] = true
		lexicalRank++
		scores[id] += 1.0 / (float64(k) + float64(lexicalRank))
		r := ranks[id]
		r.lexical = lexicalRank
		ranks[id] = r
		if canonical, ok := candidatesByID[id]; ok {
			addRetrievalSource(canonical, "lexical")
		} else {
			candidatesByID[id] = c
			addRetrievalSource(c, "lexical")
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
			if bothI != bothJ {
				return bothI > bothJ
			}
			if rI.dense != rJ.dense {
				if rI.dense == 0 {
					return false
				}
				if rJ.dense == 0 {
					return true
				}
				return rI.dense < rJ.dense
			}
			if rI.lexical != rJ.lexical {
				if rI.lexical == 0 {
					return false
				}
				if rJ.lexical == 0 {
					return true
				}
				return rI.lexical < rJ.lexical
			}
			return results[i].candidate.ID().String() < results[j].candidate.ID().String()
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

func addRetrievalSource(candidate *entities.Candidate, source string) {
	if candidate == nil {
		return
	}
	for _, existing := range candidate.RetrievalSources {
		if existing == source {
			return
		}
	}
	candidate.RetrievalSources = append(candidate.RetrievalSources, source)
}
