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

// MockVerbatimRepository for tests
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

func (m *mockVerbatimRepository) DeleteVerbatimByID(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockVerbatimRepository) DeleteVerbatimByIDTx(_ context.Context, _ *sql.Tx, _ uuid.UUID) error {
	return nil
}

func (m *mockVerbatimRepository) UpdateVerbatimSummary(_ context.Context, _ uuid.UUID, _ string, _ int) error {
	return nil
}

// MockFingerprintRepository for tests
type mockLoadFingerprintRepository struct {
	getFingerprintByIDFunc func(ctx context.Context, id uuid.UUID) (*entities.Fingerprint, error)
}

func (m *mockLoadFingerprintRepository) StoreFingerprint(ctx context.Context, fp *entities.Fingerprint) error {
	return nil
}

func (m *mockLoadFingerprintRepository) StoreFingerprintTx(ctx context.Context, tx *sql.Tx, fp *entities.Fingerprint) error {
	return nil
}

func (m *mockLoadFingerprintRepository) GetFingerprintByID(ctx context.Context, id uuid.UUID) (*entities.Fingerprint, error) {
	if m.getFingerprintByIDFunc != nil {
		return m.getFingerprintByIDFunc(ctx, id)
	}
	return nil, errors.New("not found")
}

func (m *mockLoadFingerprintRepository) GetFingerprintByVerbatimID(ctx context.Context, verbatimID uuid.UUID) (*entities.Fingerprint, error) {
	return nil, nil
}

func (m *mockLoadFingerprintRepository) GetRecentFingerprintsByWing(ctx context.Context, wing string, excludeID uuid.UUID, limit int) ([]*entities.Fingerprint, error) {
	return nil, nil
}

func (m *mockLoadFingerprintRepository) GetRecentFingerprintsByWingTx(ctx context.Context, tx *sql.Tx, wing string, excludeID uuid.UUID, limit int) ([]*entities.Fingerprint, error) {
	return nil, nil
}

// createTestVerbatim creates a test verbatim
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

// TestLoadMemory_Execute test loading existing verbatim
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

	interactor := NewLoadMemory(mockRepo, nil)
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

	// Check the ID
	if output.Verbatim.ID != testID {
		t.Errorf("Expected ID %s, got %s", testID, output.Verbatim.ID)
	}

	// Check the content
	if output.Verbatim.Content != testContent {
		t.Errorf("Expected content '%s', got '%s'", testContent, output.Verbatim.Content)
	}

	// Check the wing
	if output.Verbatim.Wing != "test-wing" {
		t.Errorf("Expected wing 'test-wing', got '%s'", output.Verbatim.Wing)
	}

	// Check the room
	if output.Verbatim.Room == nil || *output.Verbatim.Room != "test-room" {
		t.Error("Expected room to be 'test-room'")
	}

	// Check the TokenCount
	if output.Verbatim.TokenCount != len(testContent)/4 {
		t.Errorf("Expected TokenCount %d, got %d", len(testContent)/4, output.Verbatim.TokenCount)
	}

	// Check the metadata
	if len(output.Verbatim.Metadata) != 1 {
		t.Errorf("Expected 1 metadata entry, got %d", len(output.Verbatim.Metadata))
	}
}

// TestLoadMemory_NotFound test with non-existent ID
func TestLoadMemory_NotFound(t *testing.T) {
	ctx := context.Background()
	testID := uuid.New()

	mockRepo := &mockVerbatimRepository{
		getVerbatimByIDFunc: func(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error) {
			// Return nil for unknown IDs - this is how the repository indicates not found
			return nil, nil
		},
	}

	interactor := NewLoadMemory(mockRepo, nil)
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

// TestLoadMemory_InvalidID test with invalid UUID (impossible case with the uuid.UUID type)
// This test verifies that the use case handles repository errors correctly
func TestLoadMemory_InvalidID(t *testing.T) {
	ctx := context.Background()
	mockRepo := &mockVerbatimRepository{
		getVerbatimByIDFunc: func(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error) {
			// Simulate a repository error
			return nil, errors.New("invalid identifier format")
		},
	}

	interactor := NewLoadMemory(mockRepo, nil)
	// Use a zero UUID that might be considered invalid by some systems
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

// TestLoadMemory_RepositoryError test repository error
func TestLoadMemory_RepositoryError(t *testing.T) {
	ctx := context.Background()
	testID := uuid.New()

	mockRepo := &mockVerbatimRepository{
		getVerbatimByIDFunc: func(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error) {
			return nil, errors.New("database connection failed")
		},
	}

	interactor := NewLoadMemory(mockRepo, nil)
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

// TestLoadMemory_MultipleCalls test multiple successive calls
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

	interactor := NewLoadMemory(mockRepo, nil)

	// First call
	output1, err := interactor.Execute(ctx, LoadMemoryInput{ID: id1})
	if err != nil {
		t.Fatalf("First Execute failed: %v", err)
	}
	if output1.Verbatim.Content != "Content 1" {
		t.Errorf("Expected 'Content 1', got '%s'", output1.Verbatim.Content)
	}

	// Second call
	output2, err := interactor.Execute(ctx, LoadMemoryInput{ID: id2})
	if err != nil {
		t.Fatalf("Second Execute failed: %v", err)
	}
	if output2.Verbatim.Content != "Content 2" {
		t.Errorf("Expected 'Content 2', got '%s'", output2.Verbatim.Content)
	}

	// Third call (back to the first)
	output3, err := interactor.Execute(ctx, LoadMemoryInput{ID: id1})
	if err != nil {
		t.Fatalf("Third Execute failed: %v", err)
	}
	if output3.Verbatim.Content != "Content 1" {
		t.Errorf("Expected 'Content 1', got '%s'", output3.Verbatim.Content)
	}
}

// BenchmarkLoadMemory benchmarks the LoadMemory use case
func BenchmarkLoadMemory_Execute(b *testing.B) {
	ctx := context.Background()
	testID := uuid.New()
	testVerbatim := createTestVerbatim(testID, "Benchmark content")

	mockRepo := &mockVerbatimRepository{
		getVerbatimByIDFunc: func(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error) {
			return testVerbatim, nil
		},
	}

	interactor := NewLoadMemory(mockRepo, nil)
	input := LoadMemoryInput{ID: testID}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := interactor.Execute(ctx, input)
		if err != nil {
			b.Fatalf("Execute failed: %v", err)
		}
	}
}

// TestLoadMemory_ByFingerprintID test loading by FingerprintID
func TestLoadMemory_ByFingerprintID(t *testing.T) {
	ctx := context.Background()
	verbatimID := uuid.New()
	fingerprintID := uuid.New()
	testContent := "Test content loaded by fingerprint ID"
	testVerbatim := createTestVerbatim(verbatimID, testContent)

	mockVerbatimRepo := &mockVerbatimRepository{
		getVerbatimByIDFunc: func(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error) {
			if id == verbatimID {
				return testVerbatim, nil
			}
			return nil, errors.New("verbatim not found")
		},
	}

	mockFpRepo := &mockLoadFingerprintRepository{
		getFingerprintByIDFunc: func(ctx context.Context, id uuid.UUID) (*entities.Fingerprint, error) {
			if id == fingerprintID {
				return &entities.Fingerprint{
					ID:         fingerprintID,
					VerbatimID: verbatimID,
				}, nil
			}
			return nil, errors.New("fingerprint not found")
		},
	}

	interactor := NewLoadMemory(mockVerbatimRepo, mockFpRepo)
	input := LoadMemoryInput{ID: fingerprintID}

	output, err := interactor.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if output.Verbatim == nil {
		t.Fatal("Expected verbatim, got nil")
	}

	if output.Verbatim.ID != verbatimID {
		t.Errorf("Expected verbatim ID %s, got %s", verbatimID, output.Verbatim.ID)
	}

	if output.Verbatim.Content != testContent {
		t.Errorf("Expected content '%s', got '%s'", testContent, output.Verbatim.Content)
	}
}

// Ensure interface is implemented
var _ ports.VerbatimRepository = (*mockVerbatimRepository)(nil)
