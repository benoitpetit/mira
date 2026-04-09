package types

import (
	"time"

	"github.com/google/uuid"
)

// MemoryType enum
type MemoryType string

const (
	TypeDecision    MemoryType = "decision"
	TypeFact        MemoryType = "fact"
	TypePreference  MemoryType = "preference"
	TypeSessionNote MemoryType = "session_note"
	TypeDebugLog    MemoryType = "debug_log"
)

// Decay rates (per day)
var DecayRates = map[MemoryType]float64{
	TypeDecision:    0.001,
	TypeFact:        0.005,
	TypePreference:  0.01,
	TypeSessionNote: 0.1,
	TypeDebugLog:    0.5,
}

// RelationType for causal graph
type RelationType string

const (
	RelBecause     RelationType = "BECAUSE"
	RelTriggered   RelationType = "TRIGGERED"
	RelContradicts RelationType = "CONTRADICTS"
	RelUpdates     RelationType = "UPDATES"
	RelResolves    RelationType = "RESOLVES"
)

// Verbatim T0
type Verbatim struct {
	ID         uuid.UUID      `db:"id"`
	Content    string         `db:"content"`
	TokenCount int            `db:"token_count"`
	CreatedAt  time.Time      `db:"created_at"`
	Wing       string         `db:"wing"`
	Room       *string        `db:"room"`
	Metadata   map[string]any `db:"metadata"`
}

// Fingerprint T1
type Fingerprint struct {
	ID            uuid.UUID       `db:"id"`
	VerbatimID    uuid.UUID       `db:"verbatim_id"`
	Type          MemoryType      `db:"ftype"`
	ExtractedAt   time.Time       `db:"extracted_at"`
	Entities      []string        `db:"entities"`
	Subjects      []string        `db:"subjects"`
	Decision      *string         `db:"decision"`
	Data          FingerprintData `db:"data"`
	FactCount     int             `db:"fact_count"`
	TokenEstimate int             `db:"token_estimate"`
	ModelHash     string          `db:"model_hash"`
}

type FingerprintData struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Date        string         `json:"date"`
	Entities    []string       `json:"entities"`
	Subject     []string       `json:"subject"`
	Decision    string         `json:"decision,omitempty"`
	Rejected    []string       `json:"rejected,omitempty"`
	Reason      []string       `json:"reason,omitempty"`
	ValidatedBy string         `json:"validated_by,omitempty"`
	Assignee    string         `json:"assignee,omitempty"`
	Deadline    string         `json:"deadline,omitempty"`
	VerbatimRef string         `json:"verbatim_ref"`
	Custom      map[string]any `json:"custom,omitempty"`
}

// Embedding T2 with versioning
type Embedding struct {
	ID         uuid.UUID `db:"id"`
	ModelHash  string    `db:"model_hash"`
	Dim        int       `db:"dim"`
	Vector     []float32 `db:"vector"`
	Normalized bool      `db:"normalized"`
	CreatedAt  time.Time `db:"created_at"`
}

// EmbeddingModel metadata
type EmbeddingModel struct {
	ModelHash string         `db:"model_hash"`
	ModelName string         `db:"model_name"`
	Dimension int            `db:"dimension"`
	CreatedAt time.Time      `db:"created_at"`
	Metadata  map[string]any `db:"metadata"`
}

// CausalNode
type CausalNode struct {
	ID        uuid.UUID `db:"id"`
	Type      string    `db:"node_type"`
	Summary   string    `db:"summary"`
	Timestamp time.Time `db:"timestamp"`
	Wing      string    `db:"wing"`
	Room      *string   `db:"room"`
}

// CausalEdge
type CausalEdge struct {
	FromID     uuid.UUID    `db:"from_id"`
	ToID       uuid.UUID    `db:"to_id"`
	Relation   RelationType `db:"relation"`
	Weight     float64      `db:"weight"`
	DetectedAt time.Time    `db:"detected_at"`
}

// RenderMode depends on budget, not overlap
type RenderMode int

const (
	ModeHeader      RenderMode = iota // T2: 2-5 tokens, budget < 100
	ModeFingerprint                   // T1: ~15% tokens, budget < 1000
	ModeVerbatim                      // T0: 100% tokens, sufficient budget
)

// Candidate for CBA
type Candidate struct {
	Memory        *Fingerprint
	Verbatim      *Verbatim
	Embedding     []float32
	Score         float64
	Relevance     float64
	Density       float64
	Recency       float64
	SessionBoost  float64
	MaxOverlap    float64
	CausalPenalty float64
}

// SelectedMemory represents a selected memory
type SelectedMemory struct {
	Candidate  *Candidate
	Mode       RenderMode
	TokenCost  int
	Rendered   string
	SelectedAt time.Time
}

// Stats for status
type Stats struct {
	VerbatimCount    int
	FingerprintCount int
	EmbeddingCount   int
	CausalNodeCount  int
	CausalEdgeCount  int
	TotalTokens      int
	TypeCounts       map[string]int
	ActiveWings      []string
}

// ArchiveResult archiving result
type ArchiveResult struct {
	SessionNotes int
	DebugLogs    int
	TokensFreed  int
}

// TimelineItem for timeline
type TimelineItem struct {
	ID        uuid.UUID
	Timestamp time.Time
	Type      MemoryType
	Summary   string
}

// OverlapCache interface for caching pairwise overlap similarity
type OverlapCache interface {
	Get(idA, idB uuid.UUID) (float64, bool)
	Set(idA, idB uuid.UUID, similarity float64)
}
