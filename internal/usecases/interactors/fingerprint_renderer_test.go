package interactors

import (
	"strings"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

func TestNewDefaultFingerprintRenderer(t *testing.T) {
	r := NewDefaultFingerprintRenderer()
	if r == nil {
		t.Fatal("NewDefaultFingerprintRenderer returned nil")
	}
}

func TestRenderHeader(t *testing.T) {
	r := NewDefaultFingerprintRenderer()
	candidate := createTestCandidateForRenderer()

	result := r.RenderHeader(candidate)

	// Should contain type, date, wing, and ref
	if !strings.Contains(result, string(candidate.Memory.Type)) {
		t.Error("Header should contain memory type")
	}
	if !strings.Contains(result, candidate.Verbatim.Wing) {
		t.Error("Header should contain wing")
	}
	if !strings.Contains(result, candidate.Memory.Data.VerbatimRef) {
		t.Error("Header should contain verbatim ref")
	}
}

func TestRenderFingerprint(t *testing.T) {
	r := NewDefaultFingerprintRenderer()
	candidate := createTestCandidateForRenderer()

	result := r.RenderFingerprint(candidate)

	// Should contain subject
	if !strings.Contains(result, "database") {
		t.Error("Fingerprint should contain subject")
	}
	// Should contain decision
	if !strings.Contains(result, "Use PostgreSQL") {
		t.Error("Fingerprint should contain decision")
	}
	// Should contain rejected
	if !strings.Contains(result, "MySQL") {
		t.Error("Fingerprint should contain rejected items")
	}
	// Should contain reason
	if !strings.Contains(result, "ACID") {
		t.Error("Fingerprint should contain reason")
	}
	// Should contain assignee
	if !strings.Contains(result, "@John") {
		t.Error("Fingerprint should contain assignee")
	}
	// Should contain deadline
	if !strings.Contains(result, "due:Sprint 5") {
		t.Error("Fingerprint should contain deadline")
	}
	// Should contain ref
	if !strings.Contains(result, "T0:") {
		t.Error("Fingerprint should contain verbatim ref")
	}
}

func TestRenderFingerprintMinimal(t *testing.T) {
	r := NewDefaultFingerprintRenderer()
	candidate := &entities.Candidate{
		Memory: &entities.Fingerprint{
			Type: valueobjects.TypeFact,
			Data: valueobjects.FingerprintData{
				Subject:     []string{"simple"},
				VerbatimRef: "T0:abc",
			},
		},
		Verbatim: &entities.Verbatim{
			CreatedAt: time.Now(),
			Wing:      "test",
		},
	}

	result := r.RenderFingerprint(candidate)

	if !strings.Contains(result, "simple") {
		t.Error("Should contain subject")
	}
	// Should not contain optional fields
	if strings.Contains(result, "rejected") {
		t.Error("Should not contain rejected when empty")
	}
}

func TestRenderFingerprintEmptyDate(t *testing.T) {
	r := NewDefaultFingerprintRenderer()
	candidate := &entities.Candidate{
		Memory: &entities.Fingerprint{
			Type: valueobjects.TypeFact,
			Data: valueobjects.FingerprintData{
				Subject:     []string{"test"},
				Date:        "",
				VerbatimRef: "T0:abc",
			},
		},
		Verbatim: &entities.Verbatim{
			CreatedAt: time.Now(),
			Wing:      "test",
		},
	}

	result := r.RenderFingerprint(candidate)

	if !strings.Contains(result, "unknown") {
		t.Error("Should show 'unknown' for empty date")
	}
}

func TestRenderFingerprintShortDate(t *testing.T) {
	r := NewDefaultFingerprintRenderer()
	candidate := &entities.Candidate{
		Memory: &entities.Fingerprint{
			Type: valueobjects.TypeFact,
			Data: valueobjects.FingerprintData{
				Subject:     []string{"test"},
				Date:        "2024-01-15T14:30:00Z",
				VerbatimRef: "T0:abc",
			},
		},
		Verbatim: &entities.Verbatim{
			CreatedAt: time.Now(),
			Wing:      "test",
		},
	}

	result := r.RenderFingerprint(candidate)

	// Should truncate to 10 chars (YYYY-MM-DD)
	if strings.Contains(result, "2024-01-15T14:30:00Z") {
		t.Error("Should truncate date to YYYY-MM-DD")
	}
	if !strings.Contains(result, "2024-01-15") {
		t.Error("Should contain truncated date")
	}
}

func createTestCandidateForRenderer() *entities.Candidate {
	return &entities.Candidate{
		Memory: &entities.Fingerprint{
			ID:   uuid.New(),
			Type: valueobjects.TypeDecision,
			Data: valueobjects.FingerprintData{
				Subject:     []string{"database"},
				Decision:    "Use PostgreSQL",
				Rejected:    []string{"MySQL", "MongoDB"},
				Reason:      []string{"ACID", "Team expertise"},
				Assignee:    "John",
				Deadline:    "Sprint 5",
				Date:        "2024-01-15T14:30:00Z",
				VerbatimRef: "T0:" + uuid.New().String(),
			},
		},
		Verbatim: &entities.Verbatim{
			ID:        uuid.New(),
			CreatedAt: time.Now(),
			Wing:      "backend",
			Room:      stringPtr("database"),
		},
	}
}

func stringPtr(s string) *string {
	return &s
}

func BenchmarkRenderHeader(b *testing.B) {
	r := NewDefaultFingerprintRenderer()
	candidate := createTestCandidateForRenderer()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.RenderHeader(candidate)
	}
}

func BenchmarkRenderFingerprint(b *testing.B) {
	r := NewDefaultFingerprintRenderer()
	candidate := createTestCandidateForRenderer()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.RenderFingerprint(candidate)
	}
}
