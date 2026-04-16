// Package ports defines repository interfaces - Ports in Clean Architecture
//
// These interfaces define the contract between the use cases (application layer)
// and the adapters (infrastructure layer). They follow the Dependency Inversion
// Principle: the inner layers (use cases) depend on these abstractions, not on
// concrete implementations.
//
// The Repository interface is composed of smaller, focused interfaces that can
// be used individually when only specific capabilities are needed.
package ports

import (
	"context"
	"database/sql"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

// VerbatimRepository defines the interface for verbatim (T0) storage.
// Verbatims contain the full raw content of memories.
type VerbatimRepository interface {
	// StoreVerbatim stores a verbatim in a new transaction.
	// Use StoreVerbatimTx when you need to store within an existing transaction.
	StoreVerbatim(ctx context.Context, verbatim *entities.Verbatim) error

	// StoreVerbatimTx stores a verbatim within an existing transaction.
	StoreVerbatimTx(ctx context.Context, tx *sql.Tx, verbatim *entities.Verbatim) error

	// GetVerbatimByID retrieves a verbatim by its unique identifier.
	// Returns an error if not found.
	GetVerbatimByID(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error)
}

// FingerprintRepository defines the interface for fingerprint (T1) storage.
// Fingerprints contain structured, extracted information from verbatims.
type FingerprintRepository interface {
	// StoreFingerprint stores a fingerprint in a new transaction.
	StoreFingerprint(ctx context.Context, fp *entities.Fingerprint) error

	// StoreFingerprintTx stores a fingerprint within an existing transaction.
	StoreFingerprintTx(ctx context.Context, tx *sql.Tx, fp *entities.Fingerprint) error

	// GetFingerprintByID retrieves a fingerprint by its unique identifier.
	GetFingerprintByID(ctx context.Context, id uuid.UUID) (*entities.Fingerprint, error)

	// GetFingerprintByVerbatimID retrieves a fingerprint by its associated verbatim ID.
	GetFingerprintByVerbatimID(ctx context.Context, verbatimID uuid.UUID) (*entities.Fingerprint, error)

	// GetRecentFingerprintsByWing retrieves recent fingerprints from a specific wing,
	// excluding the specified fingerprint ID.
	GetRecentFingerprintsByWing(ctx context.Context, wing string, excludeID uuid.UUID, limit int) ([]*entities.Fingerprint, error)

	// GetRecentFingerprintsByWingTx retrieves recent fingerprints within an existing transaction.
	GetRecentFingerprintsByWingTx(ctx context.Context, tx *sql.Tx, wing string, excludeID uuid.UUID, limit int) ([]*entities.Fingerprint, error)
}

// EmbeddingRepository defines the interface for embedding (T2) storage.
// Embeddings contain vector representations of verbatims for semantic search.
type EmbeddingRepository interface {
	// StoreEmbedding stores an embedding in a new transaction.
	StoreEmbedding(ctx context.Context, emb *entities.Embedding) error

	// StoreEmbeddingTx stores an embedding within an existing transaction.
	StoreEmbeddingTx(ctx context.Context, tx *sql.Tx, emb *entities.Embedding) error

	// GetEmbeddingByID retrieves an embedding by its unique identifier (same as verbatim ID).
	GetEmbeddingByID(ctx context.Context, id uuid.UUID) (*entities.Embedding, error)
}

// CausalGraphRepository defines the interface for causal graph storage.
// The causal graph tracks relationships between memories (causes, consequences, etc.).
type CausalGraphRepository interface {
	// AddNode adds a node to the causal graph in a new transaction.
	AddNode(ctx context.Context, node *entities.CausalNode) error

	// AddNodeTx adds a node within an existing transaction.
	AddNodeTx(ctx context.Context, tx *sql.Tx, node *entities.CausalNode) error

	// AddEdge adds a directed edge between two nodes in a new transaction.
	AddEdge(ctx context.Context, edge *entities.CausalEdge) error

	// AddEdgeTx adds an edge within an existing transaction.
	AddEdgeTx(ctx context.Context, tx *sql.Tx, edge *entities.CausalEdge) error

	// HasEdge checks if an edge exists between two nodes (in either direction).
	HasEdge(ctx context.Context, fromID, toID uuid.UUID) bool

	// GetChain retrieves the causal chain (ancestors) of a node up to maxDepth levels.
	// Performs a BFS traversal from the node to its parents.
	GetChain(ctx context.Context, id uuid.UUID, maxDepth int) ([]*entities.CausalNode, error)

	// GetConsequences retrieves the consequences (descendants) of a node up to maxDepth levels.
	// Performs a BFS traversal from the node to its children.
	GetConsequences(ctx context.Context, id uuid.UUID, maxDepth int) ([]*entities.CausalNode, error)

	// GetParents retrieves the direct parents of a node, optionally filtered by relation type.
	GetParents(ctx context.Context, nodeID uuid.UUID, relations ...valueobjects.RelationType) ([]*entities.CausalNode, error)

	// GetChildren retrieves the direct children of a node, optionally filtered by relation type.
	GetChildren(ctx context.Context, nodeID uuid.UUID, relations ...valueobjects.RelationType) ([]*entities.CausalNode, error)
}

// ModelRepository defines the interface for embedding model registration and retrieval.
type ModelRepository interface {
	// RegisterModel registers a new embedding model or updates its metadata.
	RegisterModel(ctx context.Context, model *entities.EmbeddingModel) error

	// GetAllModels returns the hashes of all registered models.
	GetAllModels(ctx context.Context) ([]string, error)
}

// StatsRepository defines the interface for statistics and analytics.
type StatsRepository interface {
	// GetStats retrieves comprehensive statistics about the stored memories.
	GetStats(ctx context.Context) (*valueobjects.Stats, error)

	// GetTimeline retrieves a chronological timeline of memories for a specific wing.
	GetTimeline(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string, limit int, cursor *string) ([]*valueobjects.TimelineItem, error)

	// ArchiveOldMemories archives memories that have exceeded their retention period
	// based on their type-specific decay rates.
	ArchiveOldMemories(ctx context.Context) (*valueobjects.ArchiveResult, error)

	// ClearAll removes all memories and related data from the store.
	ClearAll(ctx context.Context) error

	// ClearByRoom removes all memories and related data for a specific wing/room.
	// Returns the number of deleted verbatims.
	ClearByRoom(ctx context.Context, wing string, room *string) (int, error)
}

// TransactionManager defines the interface for database transaction management.
type TransactionManager interface {
	// Begin starts a new database transaction.
	// Returns nil for implementations that don't support transactions (e.g., some test mocks).
	Begin() (*sql.Tx, error)
}

// Repository combines all repository interfaces for atomic operations.
// This is the main interface used by interactors that need comprehensive data access.
//
// For interactors that only need specific capabilities, consider using the smaller
// interfaces (VerbatimRepository, FingerprintRepository, etc.) to reduce coupling.
type Repository interface {
	TransactionManager
	VerbatimRepository
	FingerprintRepository
	EmbeddingRepository
	CausalGraphRepository
	ModelRepository
	StatsRepository
	TagRepository
}

// EmbeddingSource provides access to embeddings for vector stores.
// This abstraction allows HNSWStore to depend on an interface rather than
// a concrete implementation, following the Dependency Inversion Principle.
type EmbeddingSource interface {
	// GetCandidatesWithEmbeddings retrieves complete candidates (fingerprint, verbatim, embedding)
	// by their IDs, optionally filtered by wing and room.
	GetCandidatesWithEmbeddings(ctx context.Context, ids []uuid.UUID, wing, room *string) ([]*entities.Candidate, error)

	// GetAllEmbeddings retrieves all embeddings from the store for index building.
	GetAllEmbeddings(ctx context.Context) ([]*entities.Embedding, error)

	// SearchLexical performs a full-text search using FTS5 or equivalent.
	// Returns candidates ranked by lexical relevance.
	SearchLexical(ctx context.Context, query string, limit int, wing, room *string) ([]*entities.Candidate, error)
}

// TagRepository defines the interface for tag-based memory indexing and retrieval.
type TagRepository interface {
	// StoreTags stores tags for a specific verbatim.
	StoreTags(ctx context.Context, verbatimID uuid.UUID, tags []string, tagType string) error

	// GetVerbatimsByTags retrieves verbatim IDs that match any of the given tags.
	GetVerbatimsByTags(ctx context.Context, tags []string, limit int) ([]uuid.UUID, error)

	// GetTagsForVerbatim retrieves all tags associated with a verbatim.
	GetTagsForVerbatim(ctx context.Context, verbatimID uuid.UUID) ([]string, error)
}
