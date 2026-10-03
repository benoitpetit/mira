package interactors_test

import (
	"context"
	"errors"
	"testing"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/interactors"
	"github.com/google/uuid"
)

// mockCompressRepo is a minimal Repository mock for compress tests.
type mockCompressRepo struct {
	verbatims    map[uuid.UUID]*entities.Verbatim
	summaries    map[uuid.UUID]string
	orderedIDs   []uuid.UUID
	failUpdateID uuid.UUID
	updateErr    error
}

func newMockCompressRepo() *mockCompressRepo {
	return &mockCompressRepo{
		verbatims: make(map[uuid.UUID]*entities.Verbatim),
		summaries: make(map[uuid.UUID]string),
	}
}

func (m *mockCompressRepo) addSessionNote(content, wing string, tokens int) *entities.Verbatim {
	v := entities.NewVerbatim(content, wing, nil)
	v.TokenCount = tokens
	m.verbatims[v.ID] = v
	m.orderedIDs = append(m.orderedIDs, v.ID)
	return v
}

func (m *mockCompressRepo) GetTimeline(_ context.Context, wing string, _ *string, _ *valueobjects.MemoryType, _, _ *string, _ int, _ *string) ([]*valueobjects.TimelineItem, error) {
	var items []*valueobjects.TimelineItem
	for _, id := range m.orderedIDs {
		v := m.verbatims[id]
		if v.Wing == wing || wing == "" {
			items = append(items, &valueobjects.TimelineItem{
				ID:   id.String(),
				Type: valueobjects.TypeSessionNote,
			})
		}
	}
	return items, nil
}

func (m *mockCompressRepo) GetVerbatimByID(_ context.Context, id uuid.UUID) (*entities.Verbatim, error) {
	v, ok := m.verbatims[id]
	if !ok {
		return nil, context.DeadlineExceeded // any error
	}
	return v, nil
}

func (m *mockCompressRepo) UpdateVerbatimSummary(_ context.Context, id uuid.UUID, summary string, _ int) error {
	if id == m.failUpdateID {
		return m.updateErr
	}
	m.summaries[id] = summary
	return nil
}

func TestCompressMemories_Execute_CompressesLargeSessionNotes(t *testing.T) {
	repo := newMockCompressRepo()
	longContent := "In order to implement the feature, please note that we need to use JWT tokens. " +
		"Due to the fact that performance was poor, we have been forced to migrate. " +
		"It is important to note that all existing sessions will be invalidated in the event that migration succeeds. " +
		"This is a long note about what we did in the session today and why it matters."
	v := repo.addSessionNote(longContent, "myproject", 50) // 50 tokens > default minTokens 30

	uc := interactors.NewCompressMemories(repo, repo)
	out, err := uc.Execute(context.Background(), interactors.CompressMemoriesInput{
		Wing:      "myproject",
		MinTokens: 30,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.CompressedCount != 1 {
		t.Errorf("CompressedCount = %d, want 1", out.CompressedCount)
	}
	if out.TokensSaved <= 0 {
		t.Errorf("TokensSaved = %d, want > 0", out.TokensSaved)
	}
	if _, ok := repo.summaries[v.ID]; !ok {
		t.Error("expected summary to be stored")
	}
	if len(repo.summaries[v.ID]) >= len(longContent) {
		t.Errorf("summary (%d chars) not shorter than input (%d chars)", len(repo.summaries[v.ID]), len(longContent))
	}
}

func TestCompressMemories_Execute_SkipsShortNotes(t *testing.T) {
	repo := newMockCompressRepo()
	repo.addSessionNote("Short note.", "wing1", 5) // 5 tokens < minTokens 20

	uc := interactors.NewCompressMemories(repo, repo)
	out, err := uc.Execute(context.Background(), interactors.CompressMemoriesInput{
		Wing:      "wing1",
		MinTokens: 20,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.CompressedCount != 0 {
		t.Errorf("CompressedCount = %d, want 0", out.CompressedCount)
	}
}

func TestCompressMemories_Execute_SkipsAlreadyCompressed(t *testing.T) {
	repo := newMockCompressRepo()
	existing := "already compressed"
	v := repo.addSessionNote("Some long session note content here for testing.", "wing1", 50)
	v.Summary = &existing
	v.SummaryTokenCount = 3

	uc := interactors.NewCompressMemories(repo, repo)
	out, err := uc.Execute(context.Background(), interactors.CompressMemoriesInput{
		Wing:      "wing1",
		MinTokens: 10,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.CompressedCount != 0 {
		t.Errorf("CompressedCount = %d, want 0 (already compressed)", out.CompressedCount)
	}
}

func TestCompressMemories_Execute_DryRun(t *testing.T) {
	repo := newMockCompressRepo()
	longContent := "In order to do something, please note that we need JWT. Due to the fact that the old system failed, we migrated everything."
	repo.addSessionNote(longContent, "proj", 40)

	uc := interactors.NewCompressMemories(repo, repo)
	out, err := uc.Execute(context.Background(), interactors.CompressMemoriesInput{
		Wing:      "proj",
		MinTokens: 10,
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.CompressedCount != 1 {
		t.Errorf("CompressedCount = %d, want 1 (dry-run counted)", out.CompressedCount)
	}
	if !out.Estimated {
		t.Error("Estimated = false, want true for dry-run output")
	}
	if len(repo.summaries) != 0 {
		t.Errorf("expected no summaries written in dry-run, got %d", len(repo.summaries))
	}
}

func TestCompressMemories_Execute_PersistFailureReturnsAccuratePartialOutput(t *testing.T) {
	repo := newMockCompressRepo()
	content := "In order to implement this feature, please note that we need JWT tokens. Due to the fact that performance was poor, we migrated the complete system."
	first := repo.addSessionNote(content, "proj", 50)
	second := repo.addSessionNote(content, "proj", 60)
	cause := errors.New("disk unavailable")
	repo.failUpdateID = second.ID
	repo.updateErr = cause

	uc := interactors.NewCompressMemories(repo, repo)
	out, err := uc.Execute(context.Background(), interactors.CompressMemoriesInput{Wing: "proj", MinTokens: 10})
	if !errors.Is(err, cause) {
		t.Fatalf("Execute error = %v, want wrapped persistence error %v", err, cause)
	}
	if out == nil {
		t.Fatal("Execute output = nil, want partial output for first persisted summary")
	}
	if out.CompressedCount != 1 {
		t.Errorf("CompressedCount = %d, want 1 persisted summary", out.CompressedCount)
	}
	wantSaved := first.TokenCount - interactors.EstimateSummaryTokens(interactors.CompressText(content))
	if out.TokensSaved != wantSaved {
		t.Errorf("TokensSaved = %d, want %d for persisted summary only", out.TokensSaved, wantSaved)
	}
	if len(repo.summaries) != 1 {
		t.Errorf("persisted summaries = %d, want 1", len(repo.summaries))
	}
	if _, ok := repo.summaries[first.ID]; !ok {
		t.Error("first summary should remain persisted")
	}
	if _, ok := repo.summaries[second.ID]; ok {
		t.Error("failed summary must not be reported as persisted")
	}
}
