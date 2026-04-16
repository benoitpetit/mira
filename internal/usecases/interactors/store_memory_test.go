package interactors

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// Mock Transaction
type mockTx struct{}

func (m *mockTx) Commit() error   { return nil }
func (m *mockTx) Rollback() error { return nil }
func (m *mockTx) Exec(query string, args ...interface{}) (sql.Result, error) { return nil, nil }
func (m *mockTx) Query(query string, args ...interface{}) (*sql.Rows, error) { return nil, nil }
func (m *mockTx) QueryRow(query string, args ...interface{}) *sql.Row { return nil }
func (m *mockTx) Prepare(query string) (*sql.Stmt, error) { return nil, nil }
func (m *mockTx) Stmt(stmt *sql.Stmt) *sql.Stmt { return nil }

// Mock Repository implementation
type mockStoreRepository struct {
	verbatims    map[uuid.UUID]*entities.Verbatim
	fingerprints map[uuid.UUID]*entities.Fingerprint
	embeddings   map[uuid.UUID]*entities.Embedding
	nodes        map[uuid.UUID]*entities.CausalNode
	edges        []*entities.CausalEdge
	mockTx       *mockTx
}

func newMockStoreRepository() *mockStoreRepository {
	return &mockStoreRepository{
		verbatims:    make(map[uuid.UUID]*entities.Verbatim),
		fingerprints: make(map[uuid.UUID]*entities.Fingerprint),
		embeddings:   make(map[uuid.UUID]*entities.Embedding),
		nodes:        make(map[uuid.UUID]*entities.CausalNode),
		edges:        make([]*entities.CausalEdge, 0),
		mockTx:       &mockTx{},
	}
}

func (m *mockStoreRepository) Begin() (*sql.Tx, error) {
	// Return nil - we'll handle this specially in the use case
	// by using non-tx methods when tx is nil
	return nil, nil
}

func (m *mockStoreRepository) StoreVerbatim(ctx context.Context, verbatim *entities.Verbatim) error {
	m.verbatims[verbatim.ID] = verbatim
	return nil
}

func (m *mockStoreRepository) StoreVerbatimTx(ctx context.Context, tx *sql.Tx, verbatim *entities.Verbatim) error {
	return m.StoreVerbatim(ctx, verbatim)
}

func (m *mockStoreRepository) GetVerbatimByID(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error) {
	return m.verbatims[id], nil
}

func (m *mockStoreRepository) StoreFingerprint(ctx context.Context, fp *entities.Fingerprint) error {
	m.fingerprints[fp.ID] = fp
	return nil
}

func (m *mockStoreRepository) StoreFingerprintTx(ctx context.Context, tx *sql.Tx, fp *entities.Fingerprint) error {
	return m.StoreFingerprint(ctx, fp)
}

func (m *mockStoreRepository) GetFingerprintByID(ctx context.Context, id uuid.UUID) (*entities.Fingerprint, error) {
	return m.fingerprints[id], nil
}

func (m *mockStoreRepository) GetFingerprintByVerbatimID(ctx context.Context, verbatimID uuid.UUID) (*entities.Fingerprint, error) {
	for _, fp := range m.fingerprints {
		if fp.VerbatimID == verbatimID {
			return fp, nil
		}
	}
	return nil, errors.New("fingerprint not found")
}

func (m *mockStoreRepository) GetRecentFingerprintsByWing(ctx context.Context, wing string, excludeID uuid.UUID, limit int) ([]*entities.Fingerprint, error) {
	return nil, nil
}

func (m *mockStoreRepository) GetRecentFingerprintsByWingTx(ctx context.Context, tx *sql.Tx, wing string, excludeID uuid.UUID, limit int) ([]*entities.Fingerprint, error) {
	return nil, nil
}

func (m *mockStoreRepository) StoreEmbedding(ctx context.Context, emb *entities.Embedding) error {
	m.embeddings[emb.ID] = emb
	return nil
}

func (m *mockStoreRepository) StoreEmbeddingTx(ctx context.Context, tx *sql.Tx, emb *entities.Embedding) error {
	return m.StoreEmbedding(ctx, emb)
}

func (m *mockStoreRepository) GetEmbeddingByID(ctx context.Context, id uuid.UUID) (*entities.Embedding, error) {
	return m.embeddings[id], nil
}

func (m *mockStoreRepository) AddNode(ctx context.Context, node *entities.CausalNode) error {
	m.nodes[node.ID] = node
	return nil
}

func (m *mockStoreRepository) AddNodeTx(ctx context.Context, tx *sql.Tx, node *entities.CausalNode) error {
	return m.AddNode(ctx, node)
}

func (m *mockStoreRepository) AddEdge(ctx context.Context, edge *entities.CausalEdge) error {
	m.edges = append(m.edges, edge)
	return nil
}

func (m *mockStoreRepository) AddEdgeTx(ctx context.Context, tx *sql.Tx, edge *entities.CausalEdge) error {
	return m.AddEdge(ctx, edge)
}

func (m *mockStoreRepository) HasEdge(ctx context.Context, fromID, toID uuid.UUID) bool {
	for _, e := range m.edges {
		if (e.FromID == fromID && e.ToID == toID) || (e.FromID == toID && e.ToID == fromID) {
			return true
		}
	}
	return false
}

func (m *mockStoreRepository) GetChain(ctx context.Context, id uuid.UUID, maxDepth int) ([]*entities.CausalNode, error) {
	return nil, nil
}

func (m *mockStoreRepository) GetConsequences(ctx context.Context, id uuid.UUID, maxDepth int) ([]*entities.CausalNode, error) {
	return nil, nil
}

func (m *mockStoreRepository) GetParents(ctx context.Context, nodeID uuid.UUID, relations ...valueobjects.RelationType) ([]*entities.CausalNode, error) {
	return nil, nil
}

func (m *mockStoreRepository) GetChildren(ctx context.Context, nodeID uuid.UUID, relations ...valueobjects.RelationType) ([]*entities.CausalNode, error) {
	return nil, nil
}

func (m *mockStoreRepository) RegisterModel(ctx context.Context, model *entities.EmbeddingModel) error {
	return nil
}

func (m *mockStoreRepository) GetAllModels(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (m *mockStoreRepository) GetStats(ctx context.Context) (*valueobjects.Stats, error) {
	return nil, nil
}

func (m *mockStoreRepository) GetTimeline(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string, limit int, cursor *string) ([]*valueobjects.TimelineItem, error) {
	return nil, nil
}

func (m *mockStoreRepository) ArchiveOldMemories(ctx context.Context) (*valueobjects.ArchiveResult, error) {
	return nil, nil
}

func (m *mockStoreRepository) ClearAll(ctx context.Context) error {
	return nil
}

func (m *mockStoreRepository) ClearByRoom(ctx context.Context, wing string, room *string) (int, error) {
	return 0, nil
}

func (m *mockStoreRepository) StoreTags(ctx context.Context, verbatimID uuid.UUID, tags []string, tagType string) error {
	return nil
}

func (m *mockStoreRepository) GetVerbatimsByTags(ctx context.Context, tags []string, limit int) ([]uuid.UUID, error) {
	return nil, nil
}

func (m *mockStoreRepository) GetTagsForVerbatim(ctx context.Context, verbatimID uuid.UUID) ([]string, error) {
	return nil, nil
}

// Mock Extractor
type mockStoreExtractor struct{}

func (m *mockStoreExtractor) ExtractPipeline(ctx context.Context, verbatim *entities.Verbatim, forcedType *valueobjects.MemoryType) (*entities.Fingerprint, *entities.Embedding, error) {
	memType := valueobjects.TypeSessionNote
	if forcedType != nil {
		memType = *forcedType
	}
	fp := entities.NewFingerprint(verbatim.ID, memType, "test-model")
	fp.WithData(valueobjects.FingerprintData{
		Subject: []string{"test"},
	})
	emb := entities.NewEmbedding(verbatim.ID, "test-model", make([]float32, 384))
	return fp, emb, nil
}

func (m *mockStoreExtractor) Encode(ctx context.Context, text string) ([]float32, error) {
	return make([]float32, 384), nil
}

func (m *mockStoreExtractor) ModelHash() string {
	return "test-model"
}

func (m *mockStoreExtractor) DetectCausalRelations(ctx context.Context, newFp *entities.Fingerprint, recentFps []*entities.Fingerprint, verbatimContent string) ([]*entities.CausalEdge, error) {
	return nil, nil
}

// Mock Vector Store
type mockStoreVectorStore struct {
	candidates []*entities.Candidate
}

// Mock Logger
type mockLogger struct{}

func (m *mockLogger) Debug(_ string, _ ...interface{}) {}
func (m *mockLogger) Info(_ string, _ ...interface{})  {}
func (m *mockLogger) Warn(_ string, _ ...interface{})  {}
func (m *mockLogger) Error(_ string, _ error, _ ...interface{}) {}

func (m *mockStoreVectorStore) Search(ctx context.Context, vector []float32, limit int, wing, room *string) ([]*entities.Candidate, error) {
	return m.candidates, nil
}

func (m *mockStoreVectorStore) SearchLexical(ctx context.Context, query string, limit int, wing, room *string) ([]*entities.Candidate, error) {
	return nil, nil
}

func (m *mockStoreVectorStore) AddCandidate(ctx context.Context, candidate *entities.Candidate) error {
	m.candidates = append(m.candidates, candidate)
	return nil
}

func (m *mockStoreVectorStore) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockStoreVectorStore) ClearAll(ctx context.Context) error {
	return nil
}

func (m *mockStoreVectorStore) ClearByRoom(ctx context.Context, wing string, room *string) error {
	return nil
}

// TestStoreMemoryExecute tests the StoreMemory use case
func TestStoreMemoryExecute(t *testing.T) {
	repo := newMockStoreRepository()
	extractor := &mockStoreExtractor{}
	vectorStore := &mockStoreVectorStore{}
	
	interactor := NewStoreMemory(repo, extractor, extractor, vectorStore, nil, &mockLogger{})
	
	input := StoreMemoryInput{
		Content: "Test content for storage",
		Wing:    "test-wing",
		Room:    nil,
	}
	
	ctx := context.Background()
	output, err := interactor.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	
	// Verify output
	if output.FingerprintID == "" {
		t.Error("Expected FingerprintID to be set")
	}
	if output.Type != string(valueobjects.TypeSessionNote) {
		t.Errorf("Expected Type to be 'session_note', got '%s'", output.Type)
	}
	if output.ModelHash != "test-model" {
		t.Errorf("Expected ModelHash to be 'test-model', got '%s'", output.ModelHash)
	}
	
	// Verify repository state
	if len(repo.verbatims) != 1 {
		t.Errorf("Expected 1 verbatim in repository, got %d", len(repo.verbatims))
	}
	if len(repo.fingerprints) != 1 {
		t.Errorf("Expected 1 fingerprint in repository, got %d", len(repo.fingerprints))
	}
	if len(repo.embeddings) != 1 {
		t.Errorf("Expected 1 embedding in repository, got %d", len(repo.embeddings))
	}
	
	// Verify vector store
	if len(vectorStore.candidates) != 1 {
		t.Errorf("Expected 1 candidate in vector store, got %d", len(vectorStore.candidates))
	}
	
	// Verify causal node was created
	if len(repo.nodes) != 1 {
		t.Errorf("Expected 1 causal node, got %d", len(repo.nodes))
	}
}

// TestStoreMemoryWithRoom tests storing with a room
func TestStoreMemoryWithRoom(t *testing.T) {
	repo := newMockStoreRepository()
	extractor := &mockStoreExtractor{}
	vectorStore := &mockStoreVectorStore{}
	
	interactor := NewStoreMemory(repo, extractor, extractor, vectorStore, nil, &mockLogger{})
	
	room := "test-room"
	input := StoreMemoryInput{
		Content: "Test content",
		Wing:    "test-wing",
		Room:    &room,
	}
	
	ctx := context.Background()
	_, err := interactor.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	
	// Verify room was set
	for _, v := range repo.verbatims {
		if v.Room == nil || *v.Room != "test-room" {
			t.Error("Expected Room to be set to 'test-room'")
		}
	}
}

// BenchmarkStoreMemory benchmarks the StoreMemory use case
func BenchmarkStoreMemoryExecute(b *testing.B) {
	repo := newMockStoreRepository()
	extractor := &mockStoreExtractor{}
	vectorStore := &mockStoreVectorStore{}
	interactor := NewStoreMemory(repo, extractor, extractor, vectorStore, nil, &mockLogger{})
	
	input := StoreMemoryInput{
		Content: "Benchmark content",
		Wing:    "bench-wing",
		Room:    nil,
	}
	
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := interactor.Execute(ctx, input)
		if err != nil {
			b.Fatalf("Execute failed: %v", err)
		}
	}
}

func TestStoreMemoryInputValidate(t *testing.T) {
	tests := []struct {
		name    string
		input   StoreMemoryInput
		wantErr bool
	}{
		{
			name:    "valid",
			input:   StoreMemoryInput{Content: "Hello", Wing: "test-wing"},
			wantErr: false,
		},
		{
			name:    "empty content",
			input:   StoreMemoryInput{Content: "", Wing: "test-wing"},
			wantErr: true,
		},
		{
			name:    "invalid wing chars",
			input:   StoreMemoryInput{Content: "Hello", Wing: "test wing!"},
			wantErr: true,
		},
		{
			name:    "wing too long",
			input:   StoreMemoryInput{Content: "Hello", Wing: strings.Repeat("a", 101)},
			wantErr: true,
		},
		{
			name:    "invalid room chars",
			input:   StoreMemoryInput{Content: "Hello", Wing: "test", Room: strPtr("room@bad")},
			wantErr: true,
		},
		{
			name:    "invalid type",
			input:   StoreMemoryInput{Content: "Hello", Wing: "test", Type: func() *valueobjects.MemoryType { mt := valueobjects.MemoryType("unknown"); return &mt }()},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Ensure interfaces are implemented
var _ ports.Repository = (*mockStoreRepository)(nil)
var _ ports.Extractor = (*mockStoreExtractor)(nil)
var _ ports.VectorStore = (*mockStoreVectorStore)(nil)
