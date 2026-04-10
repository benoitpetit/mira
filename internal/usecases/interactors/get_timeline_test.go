package interactors

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
)

// MockStatsRepositoryForTimeline pour les tests
type mockStatsRepositoryForTimeline struct {
	getTimelineFunc func(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string) ([]*valueobjects.TimelineItem, error)
}

func (m *mockStatsRepositoryForTimeline) GetStats(ctx context.Context) (*valueobjects.Stats, error) {
	return nil, nil
}

func (m *mockStatsRepositoryForTimeline) GetTimeline(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string) ([]*valueobjects.TimelineItem, error) {
	if m.getTimelineFunc != nil {
		return m.getTimelineFunc(ctx, wing, room, memType, since, until)
	}
	return nil, nil
}

func (m *mockStatsRepositoryForTimeline) ArchiveOldMemories(ctx context.Context) (*valueobjects.ArchiveResult, error) {
	return nil, nil
}

// createTestTimelineItems crée des éléments de timeline pour les tests
func createTestTimelineItems() []*valueobjects.TimelineItem {
	now := time.Now()
	return []*valueobjects.TimelineItem{
		{
			ID:        "id-1",
			Timestamp: now.Add(-2 * time.Hour).Format(time.RFC3339),
			Type:      valueobjects.TypeSessionNote,
			Summary:   "First session note",
		},
		{
			ID:        "id-2",
			Timestamp: now.Add(-1 * time.Hour).Format(time.RFC3339),
			Type:      valueobjects.TypeDecision,
			Summary:   "Important decision",
		},
		{
			ID:        "id-3",
			Timestamp: now.Format(time.RFC3339),
			Type:      valueobjects.TypeFact,
			Summary:   "Recent fact",
		},
	}
}

// TestGetTimeline_Execute test timeline avec filtre wing
func TestGetTimeline_Execute(t *testing.T) {
	ctx := context.Background()
	testItems := createTestTimelineItems()

	capturedWing := ""
	mockRepo := &mockStatsRepositoryForTimeline{
		getTimelineFunc: func(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string) ([]*valueobjects.TimelineItem, error) {
			capturedWing = wing
			return testItems, nil
		},
	}

	interactor := NewGetTimeline(mockRepo)
	input := GetTimelineInput{
		Wing: "test-wing",
	}

	output, err := interactor.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if output == nil {
		t.Fatal("Expected output, got nil")
	}

	if capturedWing != "test-wing" {
		t.Errorf("Expected wing 'test-wing', got '%s'", capturedWing)
	}

	if len(output.Items) != 3 {
		t.Errorf("Expected 3 items, got %d", len(output.Items))
	}

	// Vérifier l'ordre
	if output.Items[0].Summary != "First session note" {
		t.Errorf("Expected first item 'First session note', got '%s'", output.Items[0].Summary)
	}
}

// TestGetTimeline_WithRoomFilter test avec filtre room
func TestGetTimeline_WithRoomFilter(t *testing.T) {
	ctx := context.Background()
	testItems := []*valueobjects.TimelineItem{
		{ID: "id-1", Timestamp: time.Now().Format(time.RFC3339), Type: valueobjects.TypeSessionNote, Summary: "Room note"},
	}

	capturedRoom := ""
	mockRepo := &mockStatsRepositoryForTimeline{
		getTimelineFunc: func(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string) ([]*valueobjects.TimelineItem, error) {
			if room != nil {
				capturedRoom = *room
			}
			return testItems, nil
		},
	}

	interactor := NewGetTimeline(mockRepo)
	room := "test-room"
	input := GetTimelineInput{
		Wing: "test-wing",
		Room: &room,
	}

	output, err := interactor.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if capturedRoom != "test-room" {
		t.Errorf("Expected room 'test-room', got '%s'", capturedRoom)
	}

	if len(output.Items) != 1 {
		t.Errorf("Expected 1 item, got %d", len(output.Items))
	}
}

// TestGetTimeline_WithTypeFilter test avec filtre type
func TestGetTimeline_WithTypeFilter(t *testing.T) {
	ctx := context.Background()
	testItems := []*valueobjects.TimelineItem{
		{ID: "id-1", Timestamp: time.Now().Format(time.RFC3339), Type: valueobjects.TypeDecision, Summary: "Decision 1"},
		{ID: "id-2", Timestamp: time.Now().Add(-1 * time.Hour).Format(time.RFC3339), Type: valueobjects.TypeDecision, Summary: "Decision 2"},
	}

	capturedType := valueobjects.MemoryType("")
	mockRepo := &mockStatsRepositoryForTimeline{
		getTimelineFunc: func(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string) ([]*valueobjects.TimelineItem, error) {
			if memType != nil {
				capturedType = *memType
			}
			return testItems, nil
		},
	}

	interactor := NewGetTimeline(mockRepo)
	memType := valueobjects.TypeDecision
	input := GetTimelineInput{
		Wing: "test-wing",
		Type: &memType,
	}

	output, err := interactor.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if capturedType != valueobjects.TypeDecision {
		t.Errorf("Expected type 'decision', got '%s'", capturedType)
	}

	if len(output.Items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(output.Items))
	}

	// Vérifier que tous les items sont de type decision
	for _, item := range output.Items {
		if item.Type != valueobjects.TypeDecision {
			t.Errorf("Expected type 'decision', got '%s'", item.Type)
		}
	}
}

// TestGetTimeline_DateRange test avec since/until
func TestGetTimeline_DateRange(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	testItems := []*valueobjects.TimelineItem{
		{
			ID:        "id-1",
			Timestamp: now.Add(-1 * time.Hour).Format(time.RFC3339),
			Type:      valueobjects.TypeSessionNote,
			Summary:   "Within range",
		},
	}

	capturedSince := ""
	capturedUntil := ""

	mockRepo := &mockStatsRepositoryForTimeline{
		getTimelineFunc: func(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string) ([]*valueobjects.TimelineItem, error) {
			if since != nil {
				capturedSince = *since
			}
			if until != nil {
				capturedUntil = *until
			}
			return testItems, nil
		},
	}

	interactor := NewGetTimeline(mockRepo)
	since := now.Add(-2 * time.Hour).Format(time.RFC3339)
	until := now.Format(time.RFC3339)
	input := GetTimelineInput{
		Wing:  "test-wing",
		Since: &since,
		Until: &until,
	}

	output, err := interactor.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if capturedSince != since {
		t.Errorf("Expected since '%s', got '%s'", since, capturedSince)
	}

	if capturedUntil != until {
		t.Errorf("Expected until '%s', got '%s'", until, capturedUntil)
	}

	if len(output.Items) != 1 {
		t.Errorf("Expected 1 item, got %d", len(output.Items))
	}
}

// TestGetTimeline_EmptyResult test quand aucun résultat
func TestGetTimeline_EmptyResult(t *testing.T) {
	ctx := context.Background()
	mockRepo := &mockStatsRepositoryForTimeline{
		getTimelineFunc: func(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string) ([]*valueobjects.TimelineItem, error) {
			return []*valueobjects.TimelineItem{}, nil
		},
	}

	interactor := NewGetTimeline(mockRepo)
	input := GetTimelineInput{
		Wing: "non-existent-wing",
	}

	output, err := interactor.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if output == nil {
		t.Fatal("Expected output, got nil")
	}

	if len(output.Items) != 0 {
		t.Errorf("Expected 0 items, got %d", len(output.Items))
	}
}

// TestGetTimeline_RepositoryError test erreur du repository
func TestGetTimeline_RepositoryError(t *testing.T) {
	ctx := context.Background()
	mockRepo := &mockStatsRepositoryForTimeline{
		getTimelineFunc: func(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string) ([]*valueobjects.TimelineItem, error) {
			return nil, errors.New("database error")
		},
	}

	interactor := NewGetTimeline(mockRepo)
	input := GetTimelineInput{
		Wing: "test-wing",
	}

	output, err := interactor.Execute(ctx, input)
	if err == nil {
		t.Error("Expected error for repository failure, got nil")
	}

	if output != nil {
		t.Error("Expected nil output on error")
	}
}

// TestGetTimeline_AllFilters test avec tous les filtres combinés
func TestGetTimeline_AllFilters(t *testing.T) {
	ctx := context.Background()
	testItems := []*valueobjects.TimelineItem{
		{ID: "id-1", Timestamp: time.Now().Format(time.RFC3339), Type: valueobjects.TypeDebugLog, Summary: "Debug log"},
	}

	capturedWing := ""
	capturedRoom := ""
	capturedType := valueobjects.MemoryType("")
	capturedSince := ""
	capturedUntil := ""

	mockRepo := &mockStatsRepositoryForTimeline{
		getTimelineFunc: func(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string) ([]*valueobjects.TimelineItem, error) {
			capturedWing = wing
			if room != nil {
				capturedRoom = *room
			}
			if memType != nil {
				capturedType = *memType
			}
			if since != nil {
				capturedSince = *since
			}
			if until != nil {
				capturedUntil = *until
			}
			return testItems, nil
		},
	}

	interactor := NewGetTimeline(mockRepo)
	room := "debug-room"
	memType := valueobjects.TypeDebugLog
	since := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	until := time.Now().Format(time.RFC3339)

	input := GetTimelineInput{
		Wing:  "debug-wing",
		Room:  &room,
		Type:  &memType,
		Since: &since,
		Until: &until,
	}

	_, err := interactor.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if capturedWing != "debug-wing" {
		t.Errorf("Expected wing 'debug-wing', got '%s'", capturedWing)
	}

	if capturedRoom != "debug-room" {
		t.Errorf("Expected room 'debug-room', got '%s'", capturedRoom)
	}

	if capturedType != valueobjects.TypeDebugLog {
		t.Errorf("Expected type 'debug_log', got '%s'", capturedType)
	}

	if capturedSince != since {
		t.Errorf("Expected since '%s', got '%s'", since, capturedSince)
	}

	if capturedUntil != until {
		t.Errorf("Expected until '%s', got '%s'", until, capturedUntil)
	}
}

// Ensure interface is implemented
var _ ports.StatsRepository = (*mockStatsRepositoryForTimeline)(nil)
