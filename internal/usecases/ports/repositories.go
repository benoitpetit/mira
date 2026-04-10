// Repository interfaces - Ports in Clean Architecture
package ports

import (
	"context"
	"database/sql"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

// VerbatimRepository defines the interface for verbatim storage
type VerbatimRepository interface {
	StoreVerbatim(ctx context.Context, verbatim *entities.Verbatim) error
	StoreVerbatimTx(ctx context.Context, tx *sql.Tx, verbatim *entities.Verbatim) error
	GetVerbatimByID(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error)
}

// FingerprintRepository defines the interface for fingerprint storage
type FingerprintRepository interface {
	StoreFingerprint(ctx context.Context, fp *entities.Fingerprint) error
	StoreFingerprintTx(ctx context.Context, tx *sql.Tx, fp *entities.Fingerprint) error
	GetFingerprintByID(ctx context.Context, id uuid.UUID) (*entities.Fingerprint, error)
	GetRecentFingerprintsByWing(ctx context.Context, wing string, excludeID uuid.UUID, limit int) ([]*entities.Fingerprint, error)
	GetRecentFingerprintsByWingTx(ctx context.Context, tx *sql.Tx, wing string, excludeID uuid.UUID, limit int) ([]*entities.Fingerprint, error)
}

// EmbeddingRepository defines the interface for embedding storage
type EmbeddingRepository interface {
	StoreEmbedding(ctx context.Context, emb *entities.Embedding) error
	StoreEmbeddingTx(ctx context.Context, tx *sql.Tx, emb *entities.Embedding) error
	GetEmbeddingByID(ctx context.Context, id uuid.UUID) (*entities.Embedding, error)
}

// CausalGraphRepository defines the interface for causal graph storage
type CausalGraphRepository interface {
	AddNode(ctx context.Context, node *entities.CausalNode) error
	AddNodeTx(ctx context.Context, tx *sql.Tx, node *entities.CausalNode) error
	AddEdge(ctx context.Context, edge *entities.CausalEdge) error
	AddEdgeTx(ctx context.Context, tx *sql.Tx, edge *entities.CausalEdge) error
	HasEdge(ctx context.Context, fromID, toID uuid.UUID) bool
	GetChain(ctx context.Context, id uuid.UUID, maxDepth int) ([]*entities.CausalNode, error)
	GetConsequences(ctx context.Context, id uuid.UUID, maxDepth int) ([]*entities.CausalNode, error)
	GetParents(ctx context.Context, nodeID uuid.UUID, relations ...valueobjects.RelationType) ([]*entities.CausalNode, error)
	GetChildren(ctx context.Context, nodeID uuid.UUID, relations ...valueobjects.RelationType) ([]*entities.CausalNode, error)
}

// ModelRepository defines the interface for embedding model storage
type ModelRepository interface {
	RegisterModel(ctx context.Context, model *entities.EmbeddingModel) error
	GetAllModels(ctx context.Context) ([]string, error)
}

// StatsRepository defines the interface for statistics
type StatsRepository interface {
	GetStats(ctx context.Context) (*valueobjects.Stats, error)
	GetTimeline(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string) ([]*valueobjects.TimelineItem, error)
	ArchiveOldMemories(ctx context.Context) (*valueobjects.ArchiveResult, error)
}

// TransactionManager defines the interface for transaction management
type TransactionManager interface {
	Begin() (*sql.Tx, error)
}

// Repository combines all repositories for atomic operations
type Repository interface {
	TransactionManager
	VerbatimRepository
	FingerprintRepository
	EmbeddingRepository
	CausalGraphRepository
	ModelRepository
	StatsRepository
}

// EmbeddingSource provides access to embeddings for vector stores
// This abstraction allows HNSWStore to depend on an interface rather than
// a concrete implementation, following the Dependency Inversion Principle
type EmbeddingSource interface {
	// GetCandidatesWithEmbeddings retrieves candidates (fingerprint, verbatim, embedding) by their IDs
	GetCandidatesWithEmbeddings(ids []uuid.UUID, wing, room *string) ([]*entities.Candidate, error)
	// GetAllEmbeddings retrieves all embeddings from the store
	GetAllEmbeddings() ([]*entities.Embedding, error)
}
