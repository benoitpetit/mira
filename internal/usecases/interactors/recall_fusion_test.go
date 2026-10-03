package interactors

import (
	"sort"
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

func TestReciprocalRankFusion_DuplicateWithinSourceCountsOnlyOnce(t *testing.T) {
	makeCandidate := func(id string) *entities.Candidate {
		parsed := uuid.MustParse(id)
		return &entities.Candidate{
			Memory:   &entities.Fingerprint{ID: parsed, Type: valueobjects.TypeFact},
			Verbatim: &entities.Verbatim{ID: parsed},
		}
	}
	a := makeCandidate("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	b := makeCandidate("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")

	fused := reciprocalRankFusion([]*entities.Candidate{a, a, a, b}, []*entities.Candidate{b}, 1)
	if len(fused) != 2 {
		t.Fatalf("got %d fused candidates, want 2", len(fused))
	}
	if fused[0].ID() != b.ID() {
		t.Fatalf("duplicate source entry inflated %s above candidate present in both lists %s", fused[1].ID(), b.ID())
	}
}

func TestReciprocalRankFusion_RetainsBothSearchSources(t *testing.T) {
	id := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	makeCandidate := func(content string) *entities.Candidate {
		return &entities.Candidate{
			Memory:   &entities.Fingerprint{ID: id, Type: valueobjects.TypeFact},
			Verbatim: &entities.Verbatim{ID: id, Content: content},
		}
	}
	dense := makeCandidate("dense hydration")
	lexical := makeCandidate("lexical hydration")

	fused := reciprocalRankFusion([]*entities.Candidate{dense}, []*entities.Candidate{lexical}, 60)
	if len(fused) != 1 {
		t.Fatalf("got %d copies of one memory, want one", len(fused))
	}

	got := fused[0].RetrievalSources
	if len(got) != 2 {
		t.Fatalf("retained %d sources, want dense and lexical", len(got))
	}
	if got[0] != "dense" || got[1] != "lexical" {
		t.Fatalf("sources = %v, want [dense lexical]", got)
	}
}

func TestReciprocalRankFusionMatchesDeterministicReferenceSelector(t *testing.T) {
	ids := []string{
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
		"33333333-3333-3333-3333-333333333333",
		"44444444-4444-4444-4444-444444444444",
	}
	makeCandidate := func(index int) *entities.Candidate {
		id := uuid.MustParse(ids[index])
		return &entities.Candidate{Memory: &entities.Fingerprint{ID: id}, Verbatim: &entities.Verbatim{ID: id}}
	}
	dense := []*entities.Candidate{makeCandidate(0), makeCandidate(1), makeCandidate(0), makeCandidate(2)}
	lexical := []*entities.Candidate{makeCandidate(2), makeCandidate(0), makeCandidate(3)}
	const k = 2

	type reference struct {
		id             uuid.UUID
		score          float64
		dense, lexical int
	}
	byID := make(map[uuid.UUID]*reference)
	addRanks := func(candidates []*entities.Candidate, isDense bool) {
		seen := make(map[uuid.UUID]bool)
		rank := 0
		for _, candidate := range candidates {
			id := candidate.ID()
			if seen[id] {
				continue
			}
			seen[id] = true
			rank++
			entry := byID[id]
			if entry == nil {
				entry = &reference{id: id}
				byID[id] = entry
			}
			entry.score += 1 / float64(k+rank)
			if isDense {
				entry.dense = rank
			} else {
				entry.lexical = rank
			}
		}
	}
	addRanks(dense, true)
	addRanks(lexical, false)
	want := make([]*reference, 0, len(byID))
	for _, entry := range byID {
		want = append(want, entry)
	}
	sort.Slice(want, func(i, j int) bool {
		left, right := want[i], want[j]
		if left.score != right.score {
			return left.score > right.score
		}
		leftSources, rightSources := 0, 0
		if left.dense > 0 {
			leftSources++
		}
		if left.lexical > 0 {
			leftSources++
		}
		if right.dense > 0 {
			rightSources++
		}
		if right.lexical > 0 {
			rightSources++
		}
		if leftSources != rightSources {
			return leftSources > rightSources
		}
		if left.dense != right.dense {
			if left.dense == 0 {
				return false
			}
			if right.dense == 0 {
				return true
			}
			return left.dense < right.dense
		}
		if left.lexical != right.lexical {
			if left.lexical == 0 {
				return false
			}
			if right.lexical == 0 {
				return true
			}
			return left.lexical < right.lexical
		}
		return left.id.String() < right.id.String()
	})

	for run := 0; run < 20; run++ {
		got := reciprocalRankFusion(dense, lexical, k)
		if len(got) != len(want) {
			t.Fatalf("run %d: got %d candidates, want %d", run, len(got), len(want))
		}
		for index := range want {
			if got[index].ID() != want[index].id {
				t.Fatalf("run %d position %d: got %s, want reference %s", run, index, got[index].ID(), want[index].id)
			}
		}
	}
}
