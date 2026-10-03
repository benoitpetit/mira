// Stats value objects
package valueobjects

// Stats represents system statistics
type Stats struct {
	VerbatimCount    int            `json:"verbatim_count"`
	FingerprintCount int            `json:"fingerprint_count"`
	EmbeddingCount   int            `json:"embedding_count"`
	CausalNodeCount  int            `json:"causal_node_count"`
	CausalEdgeCount  int            `json:"causal_edge_count"`
	TotalTokens      int            `json:"total_tokens"`
	TypeCounts       map[string]int `json:"type_counts"`
	ActiveWings      []string       `json:"active_wings"`
}

// NewStats creates empty stats
func NewStats() *Stats {
	return &Stats{
		TypeCounts:  make(map[string]int),
		ActiveWings: make([]string, 0),
	}
}

// ArchiveResult represents the result of an archive operation
type ArchiveResult struct {
	SessionNotes   int `json:"session_notes"`
	DebugLogs      int `json:"debug_logs"`
	TokensArchived int `json:"tokens_archived"`
	// TokensFreed is retained as a deprecated compatibility alias. Archived
	// content remains stored; the value is only an estimate removed from active recall.
	TokensFreed int `json:"tokens_freed"`
}

// TimelineItem represents an item in the timeline
type TimelineItem struct {
	ID              string     `json:"id"`
	CursorID        string     `json:"-"`
	Timestamp       string     `json:"timestamp"`
	CursorTimestamp string     `json:"-"`
	Type            MemoryType `json:"type"`
	LifecycleState  string     `json:"lifecycle_state"`
	Summary         string     `json:"summary"`
	Wing            string     `json:"wing"`
}
