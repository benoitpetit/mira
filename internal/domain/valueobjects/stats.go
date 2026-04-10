// Stats value objects
package valueobjects

// Stats represents system statistics
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

// NewStats creates empty stats
func NewStats() *Stats {
	return &Stats{
		TypeCounts:  make(map[string]int),
		ActiveWings: make([]string, 0),
	}
}

// ArchiveResult represents the result of an archive operation
type ArchiveResult struct {
	SessionNotes int
	DebugLogs    int
	TokensFreed  int
}

// TimelineItem represents an item in the timeline
type TimelineItem struct {
	ID        string
	Timestamp string
	Type      MemoryType
	Summary   string
}
