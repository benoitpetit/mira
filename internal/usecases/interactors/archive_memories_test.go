package interactors

import (
	"context"
	"errors"
	"testing"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
)

// MockStatsRepositoryForArchive pour les tests
type mockStatsRepositoryForArchive struct {
	archiveOldMemoriesFunc func(ctx context.Context) (*valueobjects.ArchiveResult, error)
}

func (m *mockStatsRepositoryForArchive) GetStats(ctx context.Context) (*valueobjects.Stats, error) {
	return nil, nil
}

func (m *mockStatsRepositoryForArchive) GetTimeline(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string) ([]*valueobjects.TimelineItem, error) {
	return nil, nil
}

func (m *mockStatsRepositoryForArchive) ArchiveOldMemories(ctx context.Context) (*valueobjects.ArchiveResult, error) {
	if m.archiveOldMemoriesFunc != nil {
		return m.archiveOldMemoriesFunc(ctx)
	}
	return nil, nil
}

func (m *mockStatsRepositoryForArchive) ClearAll(ctx context.Context) error {
	return nil
}

func (m *mockStatsRepositoryForArchive) ClearByRoom(ctx context.Context, wing string, room *string) (int, error) {
	return 0, nil
}

// TestArchiveMemories_Execute test archivage avec vieilles mémoires
func TestArchiveMemories_Execute(t *testing.T) {
	ctx := context.Background()
	expectedResult := &valueobjects.ArchiveResult{
		SessionNotes: 10,
		DebugLogs:    25,
		TokensFreed:  5000,
	}

	mockRepo := &mockStatsRepositoryForArchive{
		archiveOldMemoriesFunc: func(ctx context.Context) (*valueobjects.ArchiveResult, error) {
			return expectedResult, nil
		},
	}

	interactor := NewArchiveMemories(mockRepo)
	output, err := interactor.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if output == nil {
		t.Fatal("Expected output, got nil")
	}

	if output.Result == nil {
		t.Fatal("Expected result, got nil")
	}

	// Vérifier SessionNotes
	if output.Result.SessionNotes != 10 {
		t.Errorf("Expected SessionNotes 10, got %d", output.Result.SessionNotes)
	}

	// Vérifier DebugLogs
	if output.Result.DebugLogs != 25 {
		t.Errorf("Expected DebugLogs 25, got %d", output.Result.DebugLogs)
	}

	// Vérifier TokensFreed
	if output.Result.TokensFreed != 5000 {
		t.Errorf("Expected TokensFreed 5000, got %d", output.Result.TokensFreed)
	}
}

// TestArchiveMemories_Empty test quand rien à archiver
func TestArchiveMemories_Empty(t *testing.T) {
	ctx := context.Background()
	emptyResult := &valueobjects.ArchiveResult{
		SessionNotes: 0,
		DebugLogs:    0,
		TokensFreed:  0,
	}

	mockRepo := &mockStatsRepositoryForArchive{
		archiveOldMemoriesFunc: func(ctx context.Context) (*valueobjects.ArchiveResult, error) {
			return emptyResult, nil
		},
	}

	interactor := NewArchiveMemories(mockRepo)
	output, err := interactor.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if output == nil {
		t.Fatal("Expected output, got nil")
	}

	if output.Result.SessionNotes != 0 {
		t.Errorf("Expected SessionNotes 0, got %d", output.Result.SessionNotes)
	}

	if output.Result.DebugLogs != 0 {
		t.Errorf("Expected DebugLogs 0, got %d", output.Result.DebugLogs)
	}

	if output.Result.TokensFreed != 0 {
		t.Errorf("Expected TokensFreed 0, got %d", output.Result.TokensFreed)
	}
}

// TestArchiveMemories_ByType test archivage par type (simule différents résultats par type)
func TestArchiveMemories_ByType(t *testing.T) {
	tests := []struct {
		name           string
		result         *valueobjects.ArchiveResult
		expectedNotes  int
		expectedLogs   int
		expectedTokens int
	}{
		{
			name: "session_notes_only",
			result: &valueobjects.ArchiveResult{
				SessionNotes: 50,
				DebugLogs:    0,
				TokensFreed:  2500,
			},
			expectedNotes:  50,
			expectedLogs:   0,
			expectedTokens: 2500,
		},
		{
			name: "debug_logs_only",
			result: &valueobjects.ArchiveResult{
				SessionNotes: 0,
				DebugLogs:    100,
				TokensFreed:  5000,
			},
			expectedNotes:  0,
			expectedLogs:   100,
			expectedTokens: 5000,
		},
		{
			name: "mixed_types",
			result: &valueobjects.ArchiveResult{
				SessionNotes: 30,
				DebugLogs:    70,
				TokensFreed:  8000,
			},
			expectedNotes:  30,
			expectedLogs:   70,
			expectedTokens: 8000,
		},
		{
			name: "large_archive",
			result: &valueobjects.ArchiveResult{
				SessionNotes: 1000,
				DebugLogs:    5000,
				TokensFreed:  1000000,
			},
			expectedNotes:  1000,
			expectedLogs:   5000,
			expectedTokens: 1000000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			mockRepo := &mockStatsRepositoryForArchive{
				archiveOldMemoriesFunc: func(ctx context.Context) (*valueobjects.ArchiveResult, error) {
					return tt.result, nil
				},
			}

			interactor := NewArchiveMemories(mockRepo)
			output, err := interactor.Execute(ctx)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			if output.Result.SessionNotes != tt.expectedNotes {
				t.Errorf("Expected SessionNotes %d, got %d", tt.expectedNotes, output.Result.SessionNotes)
			}

			if output.Result.DebugLogs != tt.expectedLogs {
				t.Errorf("Expected DebugLogs %d, got %d", tt.expectedLogs, output.Result.DebugLogs)
			}

			if output.Result.TokensFreed != tt.expectedTokens {
				t.Errorf("Expected TokensFreed %d, got %d", tt.expectedTokens, output.Result.TokensFreed)
			}
		})
	}
}

// TestArchiveMemories_RepositoryError test erreur du repository
func TestArchiveMemories_RepositoryError(t *testing.T) {
	ctx := context.Background()
	mockRepo := &mockStatsRepositoryForArchive{
		archiveOldMemoriesFunc: func(ctx context.Context) (*valueobjects.ArchiveResult, error) {
			return nil, errors.New("database error during archive")
		},
	}

	interactor := NewArchiveMemories(mockRepo)
	output, err := interactor.Execute(ctx)
	if err == nil {
		t.Error("Expected error for repository failure, got nil")
	}

	if output != nil {
		t.Error("Expected nil output on error")
	}
}

// TestArchiveMemories_MultipleExecutions test plusieurs exécutions successives
func TestArchiveMemories_MultipleExecutions(t *testing.T) {
	ctx := context.Background()
	callCount := 0
	results := []*valueobjects.ArchiveResult{
		{SessionNotes: 10, DebugLogs: 20, TokensFreed: 1000},
		{SessionNotes: 5, DebugLogs: 10, TokensFreed: 500},
		{SessionNotes: 0, DebugLogs: 0, TokensFreed: 0},
	}

	mockRepo := &mockStatsRepositoryForArchive{
		archiveOldMemoriesFunc: func(ctx context.Context) (*valueobjects.ArchiveResult, error) {
			if callCount < len(results) {
				result := results[callCount]
				callCount++
				return result, nil
			}
			return &valueobjects.ArchiveResult{}, nil
		},
	}

	interactor := NewArchiveMemories(mockRepo)

	// Première exécution
	output1, err := interactor.Execute(ctx)
	if err != nil {
		t.Fatalf("First Execute failed: %v", err)
	}
	if output1.Result.SessionNotes != 10 {
		t.Errorf("Expected SessionNotes 10, got %d", output1.Result.SessionNotes)
	}

	// Deuxième exécution
	output2, err := interactor.Execute(ctx)
	if err != nil {
		t.Fatalf("Second Execute failed: %v", err)
	}
	if output2.Result.SessionNotes != 5 {
		t.Errorf("Expected SessionNotes 5, got %d", output2.Result.SessionNotes)
	}

	// Troisième exécution
	output3, err := interactor.Execute(ctx)
	if err != nil {
		t.Fatalf("Third Execute failed: %v", err)
	}
	if output3.Result.SessionNotes != 0 {
		t.Errorf("Expected SessionNotes 0, got %d", output3.Result.SessionNotes)
	}

	if callCount != 3 {
		t.Errorf("Expected 3 calls to ArchiveOldMemories, got %d", callCount)
	}
}

// BenchmarkArchiveMemories benchmark le use case ArchiveMemories
func BenchmarkArchiveMemories_Execute(b *testing.B) {
	ctx := context.Background()
	result := &valueobjects.ArchiveResult{
		SessionNotes: 100,
		DebugLogs:    200,
		TokensFreed:  10000,
	}

	mockRepo := &mockStatsRepositoryForArchive{
		archiveOldMemoriesFunc: func(ctx context.Context) (*valueobjects.ArchiveResult, error) {
			return result, nil
		},
	}

	interactor := NewArchiveMemories(mockRepo)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := interactor.Execute(ctx)
		if err != nil {
			b.Fatalf("Execute failed: %v", err)
		}
	}
}

// Ensure interface is implemented
var _ ports.StatsRepository = (*mockStatsRepositoryForArchive)(nil)
