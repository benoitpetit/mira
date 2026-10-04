package benchmark

import (
	"math"
	"testing"
)

func TestEvaluateRanking(t *testing.T) {
	tests := []struct {
		name                             string
		judgments                        []Judgment
		ranking                          []string
		wantRecall5, wantMRR, wantNDCG10 float64
	}{
		{"empty", []Judgment{{QueryID: "q", MemoryID: "a", Grade: 2}}, nil, 0, 0, 0},
		{"no relevant judgments", []Judgment{{QueryID: "q", MemoryID: "a", Grade: 0}}, []string{"a"}, 0, 0, 0},
		{"first relevant at rank one", []Judgment{{QueryID: "q", MemoryID: "a", Grade: 2}, {QueryID: "q", MemoryID: "b", Grade: 1}}, []string{"a", "b"}, 1, 1, 1},
		{"later relevant with duplicate", []Judgment{{QueryID: "q", MemoryID: "a", Grade: 1}, {QueryID: "q", MemoryID: "b", Grade: 2}}, []string{"x", "x", "y", "b"}, .5, 1.0 / 3.0, (3 / math.Log2(4)) / (3 + 1/math.Log2(3))},
		{"tied labels retain ideal nDCG", []Judgment{{QueryID: "q", MemoryID: "a", Grade: 1}, {QueryID: "q", MemoryID: "b", Grade: 1}}, []string{"b", "a"}, 1, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateRanking(tt.judgments, tt.ranking)
			if got.RecallAt5 != tt.wantRecall5 || got.RecallAt10 != tt.wantRecall5 || got.RecallAt20 != tt.wantRecall5 {
				t.Errorf("recall = (%v,%v,%v), want %v", got.RecallAt5, got.RecallAt10, got.RecallAt20, tt.wantRecall5)
			}
			if math.Abs(got.MRR-tt.wantMRR) > 1e-12 {
				t.Errorf("MRR = %v, want %v", got.MRR, tt.wantMRR)
			}
			if math.Abs(got.NDCGAt10-tt.wantNDCG10) > 1e-12 {
				t.Errorf("nDCG@10 = %v, want %v", got.NDCGAt10, tt.wantNDCG10)
			}
		})
	}
}
