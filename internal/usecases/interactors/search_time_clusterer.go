package interactors

import (
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
			similar := false
			for _, member := range cluster {
				sim := util.CosineSimilarity(member.Embedding, candidates[j].Embedding)
				if sim >= threshold {
					similar = true
					break
				}
			}
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
		bestScore := best.Relevance * best.Density
		for _, c := range cluster[1:] {
			score := c.Relevance * c.Density
			if score > bestScore {
				bestScore = score
				best = c
			}
		}
		representatives = append(representatives, best)
	}
	return representatives
}
