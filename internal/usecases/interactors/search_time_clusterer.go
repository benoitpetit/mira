package interactors

import (
	"sort"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/util"
)

// clusterCandidates groups candidates by cosine similarity >= threshold.
// Returns a slice of clusters, each cluster being a slice of candidates.
func clusterCandidates(candidates []*entities.Candidate, threshold float64) [][]*entities.Candidate {
	if len(candidates) == 0 {
		return nil
	}

	visited := make(map[int]bool)
	var clusters [][]*entities.Candidate

	for i := 0; i < len(candidates); i++ {
		if visited[i] {
			continue
		}
		cluster := []*entities.Candidate{candidates[i]}
		visited[i] = true

		for j := i + 1; j < len(candidates); j++ {
			if visited[j] {
				continue
			}
			// Compare with a stable representative to avoid single-link
			// transitive chains swallowing distant memories.
			similar := util.CosineSimilarity(cluster[0].Embedding, candidates[j].Embedding) >= threshold
			if similar {
				cluster = append(cluster, candidates[j])
				visited[j] = true
			}
		}
		clusters = append(clusters, cluster)
	}

	return clusters
}

// selectClusterRepresentatives picks one representative per cluster.
// For singletons, the candidate is kept as-is.
// For clusters, the candidate with the highest Relevance*Density is chosen.
func selectClusterRepresentatives(clusters [][]*entities.Candidate) []*entities.Candidate {
	var representatives []*entities.Candidate
	for _, cluster := range clusters {
		if len(cluster) == 0 {
			continue
		}
		if len(cluster) == 1 {
			representatives = append(representatives, cluster[0])
			continue
		}
		best := cluster[0]
		bestScore := clusterRepresentativeScore(best)
		for _, c := range cluster[1:] {
			score := clusterRepresentativeScore(c)
			if score > bestScore || (score == bestScore && c.ID().String() < best.ID().String()) {
				bestScore = score
				best = c
			}
		}
		representatives = append(representatives, best)
	}
	return representatives
}

func clusterRepresentativeScore(candidate *entities.Candidate) float64 {
	if candidate == nil {
		return 0
	}
	return candidate.Score
}

func earlyPruneCandidates(candidates []*entities.Candidate, relevanceFloor float64, fallbackLimit int) []*entities.Candidate {
	filtered := make([]*entities.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate != nil && (relevanceFloor <= 0 || clampRecallScore(candidate.Relevance) >= relevanceFloor) {
			filtered = append(filtered, candidate)
		}
	}
	if len(filtered) > 0 || len(candidates) == 0 || fallbackLimit <= 0 {
		return filtered
	}
	// Preserve the most relevant evidence for sparse queries, with a configured
	// bound and stable tie-breaking.
	sort.Slice(filtered, func(i, j int) bool {
		left, right := clampRecallScore(filtered[i].Relevance), clampRecallScore(filtered[j].Relevance)
		if left == right {
			return filtered[i].ID().String() < filtered[j].ID().String()
		}
		return left > right
	})
	// No candidate met the floor: build the fallback from all non-nil candidates.
	if len(filtered) == 0 {
		for _, candidate := range candidates {
			if candidate != nil {
				filtered = append(filtered, candidate)
			}
		}
		sort.Slice(filtered, func(i, j int) bool {
			left, right := clampRecallScore(filtered[i].Relevance), clampRecallScore(filtered[j].Relevance)
			if left == right {
				return filtered[i].ID().String() < filtered[j].ID().String()
			}
			return left > right
		})
	}
	if len(filtered) > fallbackLimit {
		filtered = filtered[:fallbackLimit]
	}
	return filtered
}
