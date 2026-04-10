//go:build windows
// +build windows

package vector

import (
	"context"
	"errors"

	"github.com/benoitpetit/mira/internal/adapters/storage"
	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

// HNSWStore is not supported on Windows due to dependencies
// The library uses unix-specific file operations (renameio.TempFile)
// On Windows, MIRA falls back to SQLiteVectorStore

type HNSWStore struct {
	notSupported bool
}

// HNSWOptions configures the HNSW index
type HNSWOptions struct {
	M              int
	Ml             float64
	EfConstruction int
	EfSearch       int
}

// DefaultHNSWOptions returns default HNSW options (not supported on Windows)
func DefaultHNSWOptions() HNSWOptions {
	return HNSWOptions{}
}

// NewHNSWStore creates a new HNSW index (not supported on Windows)
func NewHNSWStore(store *storage.SQLiteRepository, dimension int, indexPath string, opts HNSWOptions) (*HNSWStore, error) {
	return nil, errors.New("HNSW vector store is not supported on Windows. Use SQLiteVectorStore instead")
}

// Search searches for nearest neighbors (not supported on Windows)
func (h *HNSWStore) Search(ctx context.Context, vector []float32, limit int, wing, room *string) ([]*entities.Candidate, error) {
	return nil, errors.New("HNSW vector store is not supported on Windows")
}

// AddCandidate adds a candidate to the index (not supported on Windows)
func (h *HNSWStore) AddCandidate(ctx context.Context, c *entities.Candidate) error {
	return errors.New("HNSW vector store is not supported on Windows")
}

// Delete removes a vector from the index (not supported on Windows)
func (h *HNSWStore) Delete(ctx context.Context, id uuid.UUID) error {
	return errors.New("HNSW vector store is not supported on Windows")
}

// Stats returns index statistics (not supported on Windows)
func (h *HNSWStore) Stats() int {
	return 0
}

// IsReady returns whether the index is ready (not supported on Windows)
func (h *HNSWStore) IsReady() bool {
	return false
}

// BuildFromStore builds the index from the store (not supported on Windows)
func (h *HNSWStore) BuildFromStore(ctx context.Context) error {
	return errors.New("HNSW vector store is not supported on Windows")
}

// Save persists the index (not supported on Windows)
func (h *HNSWStore) Save() error {
	return nil
}

// Load restores the index (not supported on Windows)
func (h *HNSWStore) Load() error {
	return errors.New("HNSW vector store is not supported on Windows")
}

// EmbeddingSource interface methods (not supported on Windows)
func (h *HNSWStore) GetEmbeddings(ctx context.Context, modelHash string) ([]entities.Embedding, error) {
	return nil, errors.New("HNSW vector store is not supported on Windows")
}

func (h *HNSWStore) GetEmbeddingByID(ctx context.Context, id uuid.UUID) (*entities.Embedding, error) {
	return nil, errors.New("HNSW vector store is not supported on Windows")
}

// VectorStore interface methods (not supported on Windows)
func (h *HNSWStore) SearchWithFilters(ctx context.Context, vector []float32, limit int, wing, room *string, since, until *int64) ([]entities.Candidate, error) {
	return nil, errors.New("HNSW vector store is not supported on Windows")
}

func (h *HNSWStore) SearchCandidates(ctx context.Context, vector []float32, limit int, minSimilarity float64) ([]entities.Candidate, error) {
	return nil, errors.New("HNSW vector store is not supported on Windows")
}

func (h *HNSWStore) GetStats(ctx context.Context) (map[string]interface{}, error) {
	return map[string]interface{}{
		"error": "HNSW not supported on Windows",
	}, nil
}

func (h *HNSWStore) AddEmbedding(ctx context.Context, embedding *entities.Embedding) error {
	return errors.New("HNSW vector store is not supported on Windows")
}

func (h *HNSWStore) DeleteEmbedding(ctx context.Context, id uuid.UUID) error {
	return errors.New("HNSW vector store is not supported on Windows")
}

func (h *HNSWStore) RenderMemory(ctx context.Context, fp *entities.Fingerprint, verbatim *entities.Verbatim, mode valueobjects.RenderMode) (*valueobjects.SelectedMemory, error) {
	return nil, errors.New("HNSW vector store is not supported on Windows")
}
