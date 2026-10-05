package interactors

import (
	"testing"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

func TestClusterCandidates(t *testing.T) {
	v1 := entities.NewVerbatim("a", "w", nil)
	v2 := entities.NewVerbatim("b", "w", nil)
	v3 := entities.NewVerbatim("c", "w", nil)
	v4 := entities.NewVerbatim("d", "w", nil)

	fp1 := &entities.Fingerprint{ID: v1.ID, VerbatimID: v1.ID, Type: valueobjects.TypeFact}
	fp2 := &entities.Fingerprint{ID: v2.ID, VerbatimID: v2.ID, Type: valueobjects.TypeFact}
	fp3 := &entities.Fingerprint{ID: v3.ID, VerbatimID: v3.ID, Type: valueobjects.TypeFact}
	fp4 := &entities.Fingerprint{ID: v4.ID, VerbatimID: v4.ID, Type: valueobjects.TypeFact}

	// v1 and v2 are very similar, v3 and v4 are distinct
	candidates := []*entities.Candidate{
		entities.NewCandidate(fp1, v1, []float32{1, 0, 0, 0}),
		entities.NewCandidate(fp2, v2, []float32{0.99, 0.01, 0, 0}),
		entities.NewCandidate(fp3, v3, []float32{0, 1, 0, 0}),
		entities.NewCandidate(fp4, v4, []float32{0, 0, 1, 0}),
	}

	clusters := clusterCandidates(candidates, 0.95)
	if len(clusters) != 3 {
		t.Errorf("expected 3 clusters, got %d", len(clusters))
	}
}

func TestSelectClusterRepresentatives(t *testing.T) {
	v1 := entities.NewVerbatim("a", "w", nil)
	v2 := entities.NewVerbatim("b", "w", nil)
	v3 := entities.NewVerbatim("c", "w", nil)

	fp1 := &entities.Fingerprint{ID: v1.ID, VerbatimID: v1.ID, Type: valueobjects.TypeFact}
	fp2 := &entities.Fingerprint{ID: v2.ID, VerbatimID: v2.ID, Type: valueobjects.TypeFact}
	fp3 := &entities.Fingerprint{ID: v3.ID, VerbatimID: v3.ID, Type: valueobjects.TypeFact}

	c1 := entities.NewCandidate(fp1, v1, []float32{1, 0, 0})
	c1.Relevance = 0.5
	c1.Density = 0.5
	c1.Score = 0.25
	c2 := entities.NewCandidate(fp2, v2, []float32{1, 0, 0})
	c2.Relevance = 0.9
	c2.Density = 0.9
	c2.Score = 0.81
	c3 := entities.NewCandidate(fp3, v3, []float32{0, 1, 0})
	c3.Relevance = 0.6
	c3.Density = 0.6
	c3.Score = 0.36

	clusters := [][]*entities.Candidate{
		{c1, c2},
		{c3},
	}

	reps := selectClusterRepresentatives(clusters)
	if len(reps) != 2 {
		t.Fatalf("expected 2 representatives, got %d", len(reps))
	}
	if reps[0].ID() != v2.ID {
		t.Errorf("expected best candidate (v2) as representative, got %s", reps[0].ID())
	}
}

func TestSelectClusterRepresentativesUsesQueryScore(t *testing.T) {
	v1 := entities.NewVerbatim("low query score", "w", nil)
	v2 := entities.NewVerbatim("high query score", "w", nil)
	c1 := entities.NewCandidate(&entities.Fingerprint{ID: v1.ID}, v1, []float32{1, 0})
	c2 := entities.NewCandidate(&entities.Fingerprint{ID: v2.ID}, v2, []float32{1, 0})
	c1.Relevance, c1.Density, c1.Score = .95, .95, .10
	c2.Relevance, c2.Density, c2.Score = .60, .60, .90

	reps := selectClusterRepresentatives([][]*entities.Candidate{{c1, c2}})
	if len(reps) != 1 || reps[0].ID() != v2.ID {
		t.Fatalf("expected highest query-score candidate, got %+v", reps)
	}
}

func TestClusterCandidatesAvoidsTransitiveChains(t *testing.T) {
	vectors := [][]float32{{1, 0}, {0.94, 0.342}, {0.766, 0.643}}
	candidates := make([]*entities.Candidate, 0, len(vectors))
	for i, vector := range vectors {
		v := entities.NewVerbatim(string(rune('a'+i)), "w", nil)
		candidates = append(candidates, entities.NewCandidate(&entities.Fingerprint{ID: v.ID}, v, vector))
	}

	clusters := clusterCandidates(candidates, .93)
	if len(clusters) != 2 {
		t.Fatalf("expected representative-based clustering to produce 2 clusters, got %d", len(clusters))
	}
}

func TestEarlyPruneCandidatesUsesRelevanceFloor(t *testing.T) {
	low := &entities.Candidate{Memory: &entities.Fingerprint{ID: uuid.MustParse("11111111-1111-1111-1111-111111111111")}, Relevance: .2, Score: .99}
	high := &entities.Candidate{Memory: &entities.Fingerprint{ID: uuid.MustParse("22222222-2222-2222-2222-222222222222")}, Relevance: .7, Score: 0}

	filtered := earlyPruneCandidates([]*entities.Candidate{low, high}, .6, 5)
	if len(filtered) != 1 || filtered[0] != high {
		t.Fatalf("relevance floor should keep the zero-score relevant candidate only, got %+v", filtered)
	}
}

func TestEarlyPruneCandidatesFallbackUsesLimit(t *testing.T) {
	makeCandidate := func(id string, relevance float64) *entities.Candidate {
		return &entities.Candidate{Memory: &entities.Fingerprint{ID: uuid.MustParse(id)}, Relevance: relevance}
	}
	third := makeCandidate("33333333-3333-3333-3333-333333333333", .8)
	tiedHigh := makeCandidate("22222222-2222-2222-2222-222222222222", .9)
	lowestID := makeCandidate("11111111-1111-1111-1111-111111111111", .9)

	got := earlyPruneCandidates([]*entities.Candidate{third, tiedHigh, nil, lowestID}, .95, 2)
	if len(got) != 2 || got[0] != lowestID || got[1] != tiedHigh {
		t.Fatalf("fallback should keep the two highest relevance candidates, tied by UUID; got %+v", got)
	}
}

func TestClusterRepresentativeUsesCompositeScoreForZeroScore(t *testing.T) {
	zero := &entities.Candidate{Memory: &entities.Fingerprint{ID: uuid.MustParse("00000000-0000-0000-0000-000000000001")}, Relevance: 1, Density: 1}
	bestHighID := &entities.Candidate{Memory: &entities.Fingerprint{ID: uuid.MustParse("33333333-3333-3333-3333-333333333333")}, Relevance: .4, Density: .2, Score: .5}
	bestLowID := &entities.Candidate{Memory: &entities.Fingerprint{ID: uuid.MustParse("22222222-2222-2222-2222-222222222222")}, Relevance: .3, Density: .1, Score: .5}

	representatives := selectClusterRepresentatives([][]*entities.Candidate{{zero, bestHighID, bestLowID}})
	if len(representatives) != 1 || representatives[0] != bestLowID {
		t.Fatalf("representative should use composite score and break ties by UUID, got %+v", representatives)
	}
}
