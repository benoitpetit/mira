package benchmark

import (
	"math"
	"sort"
)

type QualityMetrics struct {
	RecallAt5  float64 `json:"recall_at_5"`
	RecallAt10 float64 `json:"recall_at_10"`
	RecallAt20 float64 `json:"recall_at_20"`
	MRR        float64 `json:"mrr"`
	NDCGAt10   float64 `json:"ndcg_at_10"`
}

func EvaluateRanking(judgments []Judgment, ranking []string) QualityMetrics {
	grades := make(map[string]int, len(judgments))
	relevant := 0
	for _, j := range judgments {
		grades[j.MemoryID] = j.Grade
		if j.Grade >= 1 {
			relevant++
		}
	}
	unique := make([]string, 0, len(ranking))
	seen := make(map[string]struct{}, len(ranking))
	for _, id := range ranking {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if relevant == 0 {
		return QualityMetrics{}
	}
	mrr := 0.0
	dcg := 0.0
	for i, id := range unique {
		grade := grades[id]
		if grade >= 1 {
			if mrr == 0 {
				mrr = 1 / float64(i+1)
			}
		}
		if i < 10 {
			dcg += gain(grade) / math.Log2(float64(i+2))
		}
	}
	ideal := make([]int, 0, len(judgments))
	for _, j := range judgments {
		ideal = append(ideal, j.Grade)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ideal)))
	idcg := 0.0
	for i, grade := range ideal {
		if i == 10 {
			break
		}
		idcg += gain(grade) / math.Log2(float64(i+2))
	}
	ndcg := 0.0
	if idcg > 0 {
		ndcg = dcg / idcg
	}
	return QualityMetrics{RecallAt5: recallAt(unique, grades, relevant, 5), RecallAt10: recallAt(unique, grades, relevant, 10), RecallAt20: recallAt(unique, grades, relevant, 20), MRR: mrr, NDCGAt10: ndcg}
}

func recallAt(ranking []string, grades map[string]int, relevant, k int) float64 {
	if relevant == 0 {
		return 0
	}
	if k > len(ranking) {
		k = len(ranking)
	}
	hits := 0
	for _, id := range ranking[:k] {
		if grades[id] >= 1 {
			hits++
		}
	}
	return float64(hits) / float64(relevant)
}

func gain(grade int) float64 {
	if grade < 0 {
		return 0
	}
	return math.Pow(2, float64(grade)) - 1
}
