package interactors

import (
	"testing"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

func TestReciprocalRankFusion(t *testing.T) {
	v1 := entities.NewVerbatim("alpha", "w", nil)
	v1.ID = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	v2 := entities.NewVerbatim("beta", "w", nil)
	v2.ID = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	v3 := entities.NewVerbatim("gamma", "w", nil)
	v3.ID = uuid.MustParse("33333333-3333-3333-3333-333333333333")
	v4 := entities.NewVerbatim("delta", "w", nil)
	v4.ID = uuid.MustParse("44444444-4444-4444-4444-444444444444")

	fp1 := &entities.Fingerprint{ID: v1.ID, VerbatimID: v1.ID, Type: valueobjects.TypeFact}
	fp2 := &entities.Fingerprint{ID: v2.ID, VerbatimID: v2.ID, Type: valueobjects.TypeFact}
	fp3 := &entities.Fingerprint{ID: v3.ID, VerbatimID: v3.ID, Type: valueobjects.TypeFact}
	fp4 := &entities.Fingerprint{ID: v4.ID, VerbatimID: v4.ID, Type: valueobjects.TypeFact}

	dense := []*entities.Candidate{
		entities.NewCandidate(fp1, v1, []float32{1, 0, 0}),
		entities.NewCandidate(fp2, v2, []float32{0, 1, 0}),
		entities.NewCandidate(fp3, v3, []float32{0, 0, 1}),
	}
	lexical := []*entities.Candidate{
		entities.NewCandidate(fp2, v2, []float32{0, 1, 0}),
		entities.NewCandidate(fp4, v4, []float32{1, 1, 0}),
		entities.NewCandidate(fp1, v1, []float32{1, 0, 0}),
	}

	fused := reciprocalRankFusion(dense, lexical, 60)

	if len(fused) != 4 {
		t.Fatalf("expected 4 fused candidates, got %d", len(fused))
	}

	// v1 and v2 appear in both lists and should be top-2
	if fused[0].ID() != v1.ID && fused[0].ID() != v2.ID {
		t.Errorf("expected top candidate to be v1 or v2, got %s", fused[0].ID())
	}
	if fused[1].ID() != v1.ID && fused[1].ID() != v2.ID {
		t.Errorf("expected second candidate to be v1 or v2, got %s", fused[1].ID())
	}
}

func TestReciprocalRankFusion_EmptyLexical(t *testing.T) {
	v1 := entities.NewVerbatim("alpha", "w", nil)
	fp1 := &entities.Fingerprint{ID: v1.ID, VerbatimID: v1.ID, Type: valueobjects.TypeFact}
	dense := []*entities.Candidate{entities.NewCandidate(fp1, v1, []float32{1, 0, 0})}

	fused := reciprocalRankFusion(dense, nil, 60)
	if len(fused) != 1 || fused[0].ID() != v1.ID {
		t.Errorf("expected dense result when lexical is empty")
	}
}
