// Package ports defines service interfaces - Ports for external services
//
// These interfaces abstract external services and infrastructure concerns
// such as embedding generation, vector search, metrics collection, and logging.
// They enable testing through mocks and allow swapping implementations.
package ports

import (
	"context"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

// Embedder defines the interface for text embedding generation.
// Implementations may use local models (e.g., Cybertron) or external APIs.
type Embedder interface {
	// Encode converts text into a dense vector representation.
	// The returned vector dimension must match the embedder's configured dimension.
	Encode(ctx context.Context, text string) ([]float32, error)
}

// FingerprintExtractor defines the interface for fingerprint extraction (T0 -> T1/T2).
// This is the main pipeline for transforming raw text into structured memory representations.
type FingerprintExtractor interface {
	// ExtractPipeline processes a verbatim through the complete extraction pipeline:
	// 1. Type detection (if forcedType is nil)
	// 2. Entity extraction
	// 3. Subject inference
	// 4. Embedding generation
	// Returns the generated fingerprint and embedding.
	ExtractPipeline(ctx context.Context, verbatim *entities.Verbatim, forcedType *valueobjects.MemoryType) (*entities.Fingerprint, *entities.Embedding, error)

	// ModelHash returns a unique identifier for the embedding model being used.
	// This is used to track which model generated each embedding.
	ModelHash() string
}

// CausalRelationDetector defines the interface for detecting causal relations between memories.
type CausalRelationDetector interface {
	// DetectCausalRelations analyzes a new fingerprint against recent fingerprints
	// to identify potential causal relationships.
	// Returns a list of edges representing detected causal relations.
	DetectCausalRelations(ctx context.Context, newFp *entities.Fingerprint, recentFps []*entities.Fingerprint, verbatimContent string) ([]*entities.CausalEdge, error)
}

// VectorStore defines the interface for vector storage and similarity search.
// Implementations may use HNSW for approximate nearest neighbor search or
// brute-force SQLite scanning as a fallback.
type VectorStore interface {
	// Search performs a similarity search for vectors closest to the query vector.
	// Returns up to 'limit' candidates, optionally filtered by wing and room.
	Search(ctx context.Context, vector []float32, limit int, wing, room *string) ([]*entities.Candidate, error)

	// SearchLexical performs a full-text search using FTS5 or equivalent.
	// Returns candidates ranked by lexical relevance.
	SearchLexical(ctx context.Context, query string, limit int, wing, room *string) ([]*entities.Candidate, error)

	// AddCandidate adds a candidate to the vector index for future searches.
	// This is typically called after storing a memory in the database.
	AddCandidate(ctx context.Context, candidate *entities.Candidate) error

	// Delete removes a candidate from the vector index.
	Delete(ctx context.Context, id uuid.UUID) error

	// ClearAll removes all vectors from the index.
	ClearAll(ctx context.Context) error

	// ClearByRoom removes all vectors belonging to a specific wing/room from the index.
	ClearByRoom(ctx context.Context, wing string, room *string) error
}

// OverlapCache defines the interface for caching pairwise overlap similarity.
// This optimization avoids recomputing cosine similarity between the same pairs
// of embeddings multiple times during the CBA (Context Budget Allocation) process.
type OverlapCache interface {
	// Get retrieves a cached similarity value between two memory IDs.
	// Returns the similarity and true if found, false otherwise.
	Get(ctx context.Context, idA, idB uuid.UUID) (float64, bool)

	// Set stores a similarity value between two memory IDs.
	Set(ctx context.Context, idA, idB uuid.UUID, similarity float64)
}

// CausalGraph defines the interface for causal graph operations used by the CBA algorithm.
// This is a read-only view of the causal graph for use in the recall process.
type CausalGraph interface {
	// HasEdge checks if a causal edge exists between two memories (in either direction).
	HasEdge(ctx context.Context, fromID, toID uuid.UUID) bool

	// GetParents retrieves the direct parents (causes) of a memory.
	GetParents(ctx context.Context, nodeID uuid.UUID, relations ...valueobjects.RelationType) ([]*entities.CausalNode, error)

	// GetChildren retrieves the direct children (consequences) of a memory.
	GetChildren(ctx context.Context, nodeID uuid.UUID, relations ...valueobjects.RelationType) ([]*entities.CausalNode, error)
}

// Extractor combines all extraction capabilities into a single interface.
// This is provided for backward compatibility and convenience.
//
// Deprecated: Consider using the more specific interfaces (FingerprintExtractor,
// Embedder, CausalRelationDetector) for better testability and reduced coupling.
type Extractor interface {
	FingerprintExtractor
	Embedder
	CausalRelationDetector
}

// FingerprintRenderer defines the interface for rendering fingerprints to text.
// Different render modes produce different levels of detail (header, fingerprint, verbatim).
type FingerprintRenderer interface {
	// RenderHeader renders a minimal header reference to the candidate.
	RenderHeader(candidate *entities.Candidate) string

	// RenderFingerprint renders the structured fingerprint data.
	RenderFingerprint(candidate *entities.Candidate) string
}

// Tokenizer defines the interface for text tokenization.
type Tokenizer interface {
	// CountTokens returns the number of tokens in the given text.
	// The exact definition of a "token" depends on the implementation
	// (e.g., words, BPE tokens, etc.).
	CountTokens(text string) int
}

// Reranker defines the interface for reranking candidate texts against a query.
type Reranker interface {
	// Rerank scores a list of candidate texts against a query.
	// Returns scores in the same order as candidates.
	Rerank(ctx context.Context, query string, candidates []string) ([]float64, error)
}

// Logger defines the interface for structured logging.
// Implementations may use standard log, zerolog, slog, or other logging libraries.
type Logger interface {
	// Debug logs a debug message with optional key-value pairs.
	// Debug messages are typically disabled in production.
	Debug(msg string, keysAndValues ...interface{})

	// Info logs an informational message with optional key-value pairs.
	Info(msg string, keysAndValues ...interface{})

	// Warn logs a warning message with optional key-value pairs.
	// Warnings indicate potential issues that don't prevent operation.
	Warn(msg string, keysAndValues ...interface{})

	// Error logs an error message with optional key-value pairs.
	// Errors indicate failures that may affect operation.
	Error(msg string, err error, keysAndValues ...interface{})
}
