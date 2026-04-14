package valueobjects

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMemoryTypeIsValid(t *testing.T) {
	tests := []struct {
		mt       MemoryType
		expected bool
	}{
		{TypeDecision, true},
		{TypeFact, true},
		{TypePreference, true},
		{TypeSessionNote, true},
		{TypeDebugLog, true},
		{MemoryType("invalid"), false},
		{MemoryType(""), false},
	}

	for _, tt := range tests {
		if got := tt.mt.IsValid(); got != tt.expected {
			t.Errorf("%s.IsValid() = %v, want %v", tt.mt, got, tt.expected)
		}
	}
}

func TestMemoryTypeDecayRate(t *testing.T) {
	tests := []struct {
		mt       MemoryType
		expected float64
	}{
		{TypeDecision, 0.001},
		{TypeFact, 0.005},
		{TypePreference, 0.01},
		{TypeSessionNote, 0.1},
		{TypeDebugLog, 0.5},
		{MemoryType("unknown"), 0.1}, // default
	}

	for _, tt := range tests {
		if got := tt.mt.DecayRate(); got != tt.expected {
			t.Errorf("%s.DecayRate() = %f, want %f", tt.mt, got, tt.expected)
		}
	}
}

func TestMemoryTypeString(t *testing.T) {
	if got := TypeDecision.String(); got != "decision" {
		t.Errorf("TypeDecision.String() = %s, want 'decision'", got)
	}
	if got := TypeFact.String(); got != "fact" {
		t.Errorf("TypeFact.String() = %s, want 'fact'", got)
	}
}

func TestRelationTypeIsValid(t *testing.T) {
	tests := []struct {
		rt       RelationType
		expected bool
	}{
		{RelBecause, true},
		{RelTriggered, true},
		{RelContradicts, true},
		{RelUpdates, true},
		{RelResolves, true},
		{RelationType("invalid"), false},
		{RelationType(""), false},
	}

	for _, tt := range tests {
		if got := tt.rt.IsValid(); got != tt.expected {
			t.Errorf("%s.IsValid() = %v, want %v", tt.rt, got, tt.expected)
		}
	}
}

func TestRelationTypeString(t *testing.T) {
	if got := RelBecause.String(); got != "BECAUSE" {
		t.Errorf("RelBecause.String() = %s, want 'BECAUSE'", got)
	}
	if got := RelResolves.String(); got != "RESOLVES" {
		t.Errorf("RelResolves.String() = %s, want 'RESOLVES'", got)
	}
}

func TestRenderModeString(t *testing.T) {
	tests := []struct {
		rm       RenderMode
		expected string
	}{
		{ModeHeader, "HEADER"},
		{ModeFingerprint, "FINGERPRINT"},
		{ModeVerbatim, "VERBATIM"},
		{RenderMode(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := tt.rm.String(); got != tt.expected {
			t.Errorf("RenderMode(%d).String() = %s, want %s", tt.rm, got, tt.expected)
		}
	}
}

func TestNewSelectedMemory(t *testing.T) {
	id := uuid.New()
	vid := uuid.New()
	sm := NewSelectedMemory(id, vid, ModeFingerprint, 150, "rendered content")

	if sm.CandidateID != id {
		t.Error("CandidateID should match")
	}
	if sm.VerbatimID != vid {
		t.Error("VerbatimID should match")
	}
	if sm.Mode != ModeFingerprint {
		t.Errorf("Mode = %v, want ModeFingerprint", sm.Mode)
	}
	if sm.TokenCost != 150 {
		t.Errorf("TokenCost = %d, want 150", sm.TokenCost)
	}
	if sm.Rendered != "rendered content" {
		t.Errorf("Rendered = %s, want 'rendered content'", sm.Rendered)
	}
	if time.Since(sm.SelectedAt) > time.Second {
		t.Error("SelectedAt should be recent")
	}
}

func TestNewStats(t *testing.T) {
	stats := NewStats()

	if stats.TypeCounts == nil {
		t.Error("TypeCounts should be initialized")
	}
	if stats.ActiveWings == nil {
		t.Error("ActiveWings should be initialized")
	}
	if stats.VerbatimCount != 0 {
		t.Error("VerbatimCount should be 0")
	}
}

func TestArchiveResult(t *testing.T) {
	result := &ArchiveResult{
		SessionNotes: 10,
		DebugLogs:    5,
		TokensFreed:  15000,
	}

	if result.SessionNotes != 10 {
		t.Error("SessionNotes not set correctly")
	}
	if result.DebugLogs != 5 {
		t.Error("DebugLogs not set correctly")
	}
	if result.TokensFreed != 15000 {
		t.Error("TokensFreed not set correctly")
	}
}

func TestTimelineItem(t *testing.T) {
	item := &TimelineItem{
		ID:        "uuid-123",
		Timestamp: "2024-01-15 14:30",
		Type:      TypeDecision,
		Summary:   "Use PostgreSQL",
	}

	if item.ID != "uuid-123" {
		t.Error("ID not set correctly")
	}
	if item.Type != TypeDecision {
		t.Error("Type not set correctly")
	}
	if item.Summary != "Use PostgreSQL" {
		t.Error("Summary not set correctly")
	}
}

func TestFingerprintData(t *testing.T) {
	data := FingerprintData{
		ID:          "fp-123",
		Type:        "decision",
		Date:        "2024-01-15T14:30:00Z",
		Entities:    []string{"PostgreSQL", "MySQL"},
		Subject:     []string{"database", "migration"},
		Decision:    "Use PostgreSQL",
		Rejected:    []string{"MySQL", "MongoDB"},
		Reason:      []string{"ACID", "Team expertise"},
		ValidatedBy: "CTO",
		Assignee:    "John",
		Deadline:    "2024-03-01",
		VerbatimRef: "T0:uuid-456",
		Custom: map[string]any{
			"priority": "high",
		},
	}

	if data.ID != "fp-123" {
		t.Error("ID not set")
	}
	if len(data.Entities) != 2 {
		t.Errorf("Entities length = %d, want 2", len(data.Entities))
	}
	if data.Decision != "Use PostgreSQL" {
		t.Error("Decision not set")
	}
	if data.Custom["priority"] != "high" {
		t.Error("Custom field not set")
	}
}
