// Service interfaces - Ports for external services
package ports

import (
	"context"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

// Embedder defines the interface for text embedding
type Embedder interface {
	Encode(ctx context.Context, text string) ([]float32, error)
}

// FingerprintExtractor defines the interface for fingerprint extraction (T0 -> T1/T2)
type FingerprintExtractor interface {
	ExtractPipeline(ctx context.Context, verbatim *entities.Verbatim, forcedType *valueobjects.MemoryType) (*entities.Fingerprint, *entities.Embedding, error)
	ModelHash() string
}

// CausalRelationDetector defines the interface for detecting causal relations
type CausalRelationDetector interface {
	DetectCausalRelations(ctx context.Context, newFp *entities.Fingerprint, recentFps []*entities.Fingerprint, verbatimContent string) ([]*entities.CausalEdge, error)
}

// VectorStore defines the interface for vector storage and search
type VectorStore interface {
	Search(ctx context.Context, vector []float32, limit int, wing, room *string) ([]*entities.Candidate, error)
	AddCandidate(ctx context.Context, candidate *entities.Candidate) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// OverlapCache defines the interface for caching pairwise overlap similarity
type OverlapCache interface {
	Get(ctx context.Context, idA, idB uuid.UUID) (float64, bool)
	Set(ctx context.Context, idA, idB uuid.UUID, similarity float64)
}

// CausalGraph defines the interface for causal graph operations (domain service)
type CausalGraph interface {
	HasEdge(ctx context.Context, fromID, toID uuid.UUID) bool
	GetParents(ctx context.Context, nodeID uuid.UUID, relations ...valueobjects.RelationType) ([]*entities.CausalNode, error)
	GetChildren(ctx context.Context, nodeID uuid.UUID, relations ...valueobjects.RelationType) ([]*entities.CausalNode, error)
}

// Extractor combines all extraction capabilities (for backward compatibility)
type Extractor interface {
	FingerprintExtractor
	Embedder
	CausalRelationDetector
}

// FingerprintRenderer defines the interface for rendering fingerprints
type FingerprintRenderer interface {
	RenderHeader(candidate *entities.Candidate) string
	RenderFingerprint(candidate *entities.Candidate) string
}

// Tokenizer defines the interface for tokenization
type Tokenizer interface {
	CountTokens(text string) int
}
