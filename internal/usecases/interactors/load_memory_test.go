package interactors

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// MockVerbatimRepository pour les tests
type mockVerbatimRepository struct {
	getVerbatimByIDFunc func(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error)
}

func (m *mockVerbatimRepository) StoreVerbatim(ctx context.Context, verbatim *entities.Verbatim) error {
	return nil
}

func (m *mockVerbatimRepository) StoreVerbatimTx(ctx context.Context, tx *sql.Tx, verbatim *entities.Verbatim) error {
	return nil
}

func (m *mockVerbatimRepository) GetVerbatimByID(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error) {
	if m.getVerbatimByIDFunc != nil {
		return m.getVerbatimByIDFunc(ctx, id)
	}
	return nil, nil
}

// createTestVerbatim crée un verbatim de test
func createTestVerbatim(id uuid.UUID, content string) *entities.Verbatim {
	room := "test-room"
	return &entities.Verbatim{
		ID:         id,
		Content:    content,
		TokenCount: len(content) / 4,
		CreatedAt:  time.Now(),
		Wing:       "test-wing",
		Room:       &room,
		Metadata:   map[string]any{"key": "value"},
	}
}

// TestLoadMemory_Execute test chargement verbatim existant
func TestLoadMemory_Execute(t *testing.T) {
	ctx := context.Background()
	testID := uuid.New()
	testContent := "Test content for loading memory"
	testVerbatim := createTestVerbatim(testID, testContent)

	mockRepo := &mockVerbatimRepository{
		getVerbatimByIDFunc: func(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error) {
			if id == testID {
				return testVerbatim, nil
			}
			return nil, nil
		},
	}

	interactor := NewLoadMemory(mockRepo)
	input := LoadMemoryInput{
		ID: testID,
	}

	output, err := interactor.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if output == nil {
		t.Fatal("Expected output, got nil")
	}

	if output.Verbatim == nil {
		t.Fatal("Expected verbatim, got nil")
	}

	// Vérifier l'ID
	if output.Verbatim.ID != testID {
		t.Errorf("Expected ID %s, got %s", testID, output.Verbatim.ID)
	}

	// Vérifier le contenu
	if output.Verbatim.Content != testContent {
		t.Errorf("Expected content '%s', got '%s'", testContent, output.Verbatim.Content)
	}

	// Vérifier le wing
	if output.Verbatim.Wing != "test-wing" {
		t.Errorf("Expected wing 'test-wing', got '%s'", output.Verbatim.Wing)
	}

	// Vérifier le room
	if output.Verbatim.Room == nil || *output.Verbatim.Room != "test-room" {
		t.Error("Expected room to be 'test-room'")
	}

	// Vérifier le TokenCount
	if output.Verbatim.TokenCount != len(testContent)/4 {
		t.Errorf("Expected TokenCount %d, got %d", len(testContent)/4, output.Verbatim.TokenCount)
	}

	// Vérifier les métadonnées
	if len(output.Verbatim.Metadata) != 1 {
		t.Errorf("Expected 1 metadata entry, got %d", len(output.Verbatim.Metadata))
	}
}

// TestLoadMemory_NotFound test avec ID inexistant
func TestLoadMemory_NotFound(t *testing.T) {
	ctx := context.Background()
	testID := uuid.New()

	mockRepo := &mockVerbatimRepository{
		getVerbatimByIDFunc: func(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error) {
			// Return nil for unknown IDs - this is how the repository indicates not found
			return nil, nil
		},
	}

	interactor := NewLoadMemory(mockRepo)
	input := LoadMemoryInput{
		ID: testID,
	}

	output, err := interactor.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute should not fail for not found: %v", err)
	}

	if output == nil {
		t.Fatal("Expected output, got nil")
	}

	if output.Verbatim != nil {
		t.Error("Expected nil verbatim for not found")
	}
}

// TestLoadMemory_InvalidID test avec UUID invalide (cas impossible avec le type uuid.UUID)
// Ce test vérifie que le use case gère correctement les erreurs du repository
func TestLoadMemory_InvalidID(t *testing.T) {
	ctx := context.Background()
	mockRepo := &mockVerbatimRepository{
		getVerbatimByIDFunc: func(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error) {
			// Simuler une erreur de repository
			return nil, errors.New("invalid identifier format")
		},
	}

	interactor := NewLoadMemory(mockRepo)
	// Utiliser un UUID zéro qui pourrait être considéré comme invalide par certains systèmes
	input := LoadMemoryInput{
		ID: uuid.Nil,
	}

	output, err := interactor.Execute(ctx, input)
	if err == nil {
		t.Error("Expected error for invalid ID")
	}

	if output != nil {
		t.Error("Expected nil output on error")
	}
}

// TestLoadMemory_RepositoryError test erreur du repository
func TestLoadMemory_RepositoryError(t *testing.T) {
	ctx := context.Background()
	testID := uuid.New()

	mockRepo := &mockVerbatimRepository{
		getVerbatimByIDFunc: func(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error) {
			return nil, errors.New("database connection failed")
		},
	}

	interactor := NewLoadMemory(mockRepo)
	input := LoadMemoryInput{
		ID: testID,
	}

	output, err := interactor.Execute(ctx, input)
	if err == nil {
		t.Error("Expected error for repository failure, got nil")
	}

	if output != nil {
		t.Error("Expected nil output on error")
	}
}

// TestLoadMemory_MultipleCalls test plusieurs appels successifs
func TestLoadMemory_MultipleCalls(t *testing.T) {
	ctx := context.Background()
	id1 := uuid.New()
	id2 := uuid.New()

	verbatim1 := createTestVerbatim(id1, "Content 1")
	verbatim2 := createTestVerbatim(id2, "Content 2")

	storage := map[uuid.UUID]*entities.Verbatim{
		id1: verbatim1,
		id2: verbatim2,
	}

	mockRepo := &mockVerbatimRepository{
		getVerbatimByIDFunc: func(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error) {
			return storage[id], nil
		},
	}

	interactor := NewLoadMemory(mockRepo)

	// Premier appel
	output1, err := interactor.Execute(ctx, LoadMemoryInput{ID: id1})
	if err != nil {
		t.Fatalf("First Execute failed: %v", err)
	}
	if output1.Verbatim.Content != "Content 1" {
		t.Errorf("Expected 'Content 1', got '%s'", output1.Verbatim.Content)
	}

	// Deuxième appel
	output2, err := interactor.Execute(ctx, LoadMemoryInput{ID: id2})
	if err != nil {
		t.Fatalf("Second Execute failed: %v", err)
	}
	if output2.Verbatim.Content != "Content 2" {
		t.Errorf("Expected 'Content 2', got '%s'", output2.Verbatim.Content)
	}

	// Troisième appel (retour au premier)
	output3, err := interactor.Execute(ctx, LoadMemoryInput{ID: id1})
	if err != nil {
		t.Fatalf("Third Execute failed: %v", err)
	}
	if output3.Verbatim.Content != "Content 1" {
		t.Errorf("Expected 'Content 1', got '%s'", output3.Verbatim.Content)
	}
}

// BenchmarkLoadMemory benchmark le use case LoadMemory
func BenchmarkLoadMemory_Execute(b *testing.B) {
	ctx := context.Background()
	testID := uuid.New()
	testVerbatim := createTestVerbatim(testID, "Benchmark content")

	mockRepo := &mockVerbatimRepository{
		getVerbatimByIDFunc: func(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error) {
			return testVerbatim, nil
		},
	}

	interactor := NewLoadMemory(mockRepo)
	input := LoadMemoryInput{ID: testID}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := interactor.Execute(ctx, input)
		if err != nil {
			b.Fatalf("Execute failed: %v", err)
		}
	}
}

// Ensure interface is implemented
var _ ports.VerbatimRepository = (*mockVerbatimRepository)(nil)
