package interactors

import (
	"context"
	"errors"
	"testing"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
)

// MockStatsRepository pour les tests
type mockStatsRepository struct {
	getStatsFunc func(ctx context.Context) (*valueobjects.Stats, error)
}

func (m *mockStatsRepository) GetStats(ctx context.Context) (*valueobjects.Stats, error) {
	if m.getStatsFunc != nil {
		return m.getStatsFunc(ctx)
	}
	return nil, nil
}

func (m *mockStatsRepository) GetTimeline(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string) ([]*valueobjects.TimelineItem, error) {
	return nil, nil
}

func (m *mockStatsRepository) ArchiveOldMemories(ctx context.Context) (*valueobjects.ArchiveResult, error) {
	return nil, nil
}

func (m *mockStatsRepository) ClearAll(ctx context.Context) error {
	return nil
}

func (m *mockStatsRepository) ClearByRoom(ctx context.Context, wing string, room *string) (int, error) {
	return 0, nil
}

// MockModelRepository pour les tests
type mockModelRepository struct {
	getAllModelsFunc func(ctx context.Context) ([]string, error)
}

func (m *mockModelRepository) RegisterModel(ctx context.Context, model *entities.EmbeddingModel) error {
	return nil
}

func (m *mockModelRepository) GetAllModels(ctx context.Context) ([]string, error) {
	if m.getAllModelsFunc != nil {
		return m.getAllModelsFunc(ctx)
	}
	return nil, nil
}

// TestGetStatus_Execute test que toutes les stats sont retournées
func TestGetStatus_Execute(t *testing.T) {
	ctx := context.Background()
	expectedStats := &valueobjects.Stats{
		VerbatimCount:    100,
		FingerprintCount: 95,
		EmbeddingCount:   95,
		CausalNodeCount:  50,
		CausalEdgeCount:  120,
		TotalTokens:      50000,
		TypeCounts: map[string]int{
			"session_note": 80,
			"decision":     10,
			"fact":         5,
		},
		ActiveWings: []string{"wing1", "wing2", "wing3"},
	}

	expectedModels := []string{"model1", "model2", "model3"}

	mockStatsRepo := &mockStatsRepository{
		getStatsFunc: func(ctx context.Context) (*valueobjects.Stats, error) {
			return expectedStats, nil
		},
	}

	mockModelRepo := &mockModelRepository{
		getAllModelsFunc: func(ctx context.Context) ([]string, error) {
			return expectedModels, nil
		},
	}

	interactor := NewGetStatus(mockStatsRepo, mockModelRepo)
	output, err := interactor.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if output == nil {
		t.Fatal("Expected output, got nil")
	}

	// Vérifier VerbatimCount
	if output.Stats.VerbatimCount != 100 {
		t.Errorf("Expected VerbatimCount 100, got %d", output.Stats.VerbatimCount)
	}

	// Vérifier FingerprintCount
	if output.Stats.FingerprintCount != 95 {
		t.Errorf("Expected FingerprintCount 95, got %d", output.Stats.FingerprintCount)
	}

	// Vérifier EmbeddingCount
	if output.Stats.EmbeddingCount != 95 {
		t.Errorf("Expected EmbeddingCount 95, got %d", output.Stats.EmbeddingCount)
	}

	// Vérifier CausalNodeCount
	if output.Stats.CausalNodeCount != 50 {
		t.Errorf("Expected CausalNodeCount 50, got %d", output.Stats.CausalNodeCount)
	}

	// Vérifier CausalEdgeCount
	if output.Stats.CausalEdgeCount != 120 {
		t.Errorf("Expected CausalEdgeCount 120, got %d", output.Stats.CausalEdgeCount)
	}

	// Vérifier TotalTokens
	if output.Stats.TotalTokens != 50000 {
		t.Errorf("Expected TotalTokens 50000, got %d", output.Stats.TotalTokens)
	}

	// Vérifier TypeCounts
	if len(output.Stats.TypeCounts) != 3 {
		t.Errorf("Expected 3 type counts, got %d", len(output.Stats.TypeCounts))
	}

	// Vérifier ActiveWings
	if len(output.Stats.ActiveWings) != 3 {
		t.Errorf("Expected 3 active wings, got %d", len(output.Stats.ActiveWings))
	}

	// Vérifier les modèles
	if len(output.Models) != 3 {
		t.Errorf("Expected 3 models, got %d", len(output.Models))
	}
}

// TestGetStatus_EmptyDatabase test avec base vide
func TestGetStatus_EmptyDatabase(t *testing.T) {
	ctx := context.Background()
	emptyStats := &valueobjects.Stats{
		VerbatimCount:    0,
		FingerprintCount: 0,
		EmbeddingCount:   0,
		CausalNodeCount:  0,
		CausalEdgeCount:  0,
		TotalTokens:      0,
		TypeCounts:       make(map[string]int),
		ActiveWings:      []string{},
	}

	mockStatsRepo := &mockStatsRepository{
		getStatsFunc: func(ctx context.Context) (*valueobjects.Stats, error) {
			return emptyStats, nil
		},
	}

	mockModelRepo := &mockModelRepository{
		getAllModelsFunc: func(ctx context.Context) ([]string, error) {
			return []string{}, nil
		},
	}

	interactor := NewGetStatus(mockStatsRepo, mockModelRepo)
	output, err := interactor.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if output == nil {
		t.Fatal("Expected output, got nil")
	}

	if output.Stats.VerbatimCount != 0 {
		t.Errorf("Expected VerbatimCount 0, got %d", output.Stats.VerbatimCount)
	}

	if output.Stats.FingerprintCount != 0 {
		t.Errorf("Expected FingerprintCount 0, got %d", output.Stats.FingerprintCount)
	}

	if output.Stats.EmbeddingCount != 0 {
		t.Errorf("Expected EmbeddingCount 0, got %d", output.Stats.EmbeddingCount)
	}

	if len(output.Models) != 0 {
		t.Errorf("Expected 0 models, got %d", len(output.Models))
	}
}

// TestGetStatus_WithModels test que les modèles sont listés
func TestGetStatus_WithModels(t *testing.T) {
	ctx := context.Background()
	stats := &valueobjects.Stats{
		VerbatimCount:    10,
		FingerprintCount: 10,
		EmbeddingCount:   10,
		TypeCounts:       make(map[string]int),
		ActiveWings:      []string{"main"},
	}

	models := []string{"nomic-embed-text-v1", "all-MiniLM-L6-v2", "custom-model"}

	mockStatsRepo := &mockStatsRepository{
		getStatsFunc: func(ctx context.Context) (*valueobjects.Stats, error) {
			return stats, nil
		},
	}

	mockModelRepo := &mockModelRepository{
		getAllModelsFunc: func(ctx context.Context) ([]string, error) {
			return models, nil
		},
	}

	interactor := NewGetStatus(mockStatsRepo, mockModelRepo)
	output, err := interactor.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(output.Models) != 3 {
		t.Errorf("Expected 3 models, got %d", len(output.Models))
	}

	for i, model := range models {
		if output.Models[i] != model {
			t.Errorf("Expected model[%d] to be '%s', got '%s'", i, model, output.Models[i])
		}
	}
}

// TestGetStatus_StatsError test erreur lors de la récupération des stats
func TestGetStatus_StatsError(t *testing.T) {
	ctx := context.Background()
	mockStatsRepo := &mockStatsRepository{
		getStatsFunc: func(ctx context.Context) (*valueobjects.Stats, error) {
			return nil, errors.New("database error")
		},
	}

	mockModelRepo := &mockModelRepository{
		getAllModelsFunc: func(ctx context.Context) ([]string, error) {
			return []string{"model1"}, nil
		},
	}

	interactor := NewGetStatus(mockStatsRepo, mockModelRepo)
	output, err := interactor.Execute(ctx)
	if err == nil {
		t.Error("Expected error for stats failure, got nil")
	}

	if output != nil {
		t.Error("Expected nil output on error")
	}
}

// TestGetStatus_ModelsError test erreur lors de la récupération des modèles (fallback)
func TestGetStatus_ModelsError(t *testing.T) {
	ctx := context.Background()
	stats := &valueobjects.Stats{
		VerbatimCount:    10,
		FingerprintCount: 10,
		EmbeddingCount:   10,
		TypeCounts:       make(map[string]int),
		ActiveWings:      []string{"main"},
	}

	mockStatsRepo := &mockStatsRepository{
		getStatsFunc: func(ctx context.Context) (*valueobjects.Stats, error) {
			return stats, nil
		},
	}

	mockModelRepo := &mockModelRepository{
		getAllModelsFunc: func(ctx context.Context) ([]string, error) {
			return nil, errors.New("models error")
		},
	}

	interactor := NewGetStatus(mockStatsRepo, mockModelRepo)
	output, err := interactor.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute should not fail when models error: %v", err)
	}

	// Fallback to "unknown"
	if len(output.Models) != 1 {
		t.Errorf("Expected 1 model (unknown), got %d", len(output.Models))
	}

	if output.Models[0] != "unknown" {
		t.Errorf("Expected model to be 'unknown', got '%s'", output.Models[0])
	}
}

// Ensure interfaces are implemented
var _ ports.StatsRepository = (*mockStatsRepository)(nil)
var _ ports.ModelRepository = (*mockModelRepository)(nil)
