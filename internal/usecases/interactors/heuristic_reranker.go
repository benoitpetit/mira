package interactors

import (
	"context"
	"strings"
)

// HeuristicReranker provides a lightweight, pure-Go reranking strategy.
// It scores candidates based on lexical overlap, density and length balance.
type HeuristicReranker struct{}

// NewHeuristicReranker creates a new heuristic reranker.
func NewHeuristicReranker() *HeuristicReranker {
	return &HeuristicReranker{}
}

// Rerank scores candidates against a query using a composite heuristic.
// Returns scores in [0, 1] in the same order as candidates.
func (r *HeuristicReranker) Rerank(ctx context.Context, query string, candidates []string) ([]float64, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		scores := make([]float64, len(candidates))
		for i := range scores {
			scores[i] = 0.5
		}
		return scores, nil
	}

	scores := make([]float64, len(candidates))
	for i, cand := range candidates {
		candTokens := tokenize(cand)
		if len(candTokens) == 0 {
			scores[i] = 0.0
			continue
		}

		// 1. Jaccard-like overlap
		overlap := 0
		for qt := range queryTokens {
			if candTokens[qt] {
				overlap++
			}
		}
		union := len(queryTokens) + len(candTokens) - overlap
		jaccard := 0.0
		if union > 0 {
			jaccard = float64(overlap) / float64(union)
		}

		// 2. Exact phrase presence bonus
		phraseBonus := 0.0
		if strings.Contains(strings.ToLower(cand), strings.ToLower(query)) {
			phraseBonus = 0.3
		}

		// 3. Length balance (prefer medium-length candidates)
		ql := float64(len(query))
		cl := float64(len(cand))
		lengthScore := 1.0
		if cl > 0 {
			ratio := ql / cl
			if ratio > 1.0 {
				ratio = 1.0 / ratio
			}
			lengthScore = ratio
		}

		score := jaccard*0.5 + phraseBonus + lengthScore*0.2
		if score > 1.0 {
			score = 1.0
		}
		scores[i] = score
	}

	return scores, nil
}

func tokenize(text string) map[string]bool {
	clean := punctuationRe.ReplaceAllString(strings.ToLower(text), " ")
	fields := strings.Fields(clean)
	tokens := make(map[string]bool, len(fields))
	for _, f := range fields {
		if len(f) >= 2 && !stopWords[f] {
			tokens[f] = true
		}
	}
	return tokens
}
