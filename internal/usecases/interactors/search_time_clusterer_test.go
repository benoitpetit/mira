package interactors

import (
	"testing"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
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
	c2 := entities.NewCandidate(fp2, v2, []float32{1, 0, 0})
	c2.Relevance = 0.9
	c2.Density = 0.9
	c3 := entities.NewCandidate(fp3, v3, []float32{0, 1, 0})
	c3.Relevance = 0.6
	c3.Density = 0.6

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
