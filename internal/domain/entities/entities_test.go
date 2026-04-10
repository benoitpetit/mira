package entities

import (
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

func TestNewVerbatim(t *testing.T) {
	room := "test-room"
	v := NewVerbatim("test content", "test-wing", &room)

	if v.ID == uuid.Nil {
		t.Error("Verbatim should have a non-nil UUID")
	}
	if v.Content != "test content" {
		t.Errorf("Content = %s, want 'test content'", v.Content)
	}
	if v.Wing != "test-wing" {
		t.Errorf("Wing = %s, want 'test-wing'", v.Wing)
	}
	if v.Room == nil || *v.Room != "test-room" {
		t.Error("Room should be set to 'test-room'")
	}
	if v.TokenCount != 0 {
		t.Errorf("TokenCount should be 0 initially, got %d", v.TokenCount)
	}
	if v.Metadata == nil {
		t.Error("Metadata should be initialized")
	}
	if time.Since(v.CreatedAt) > time.Second {
		t.Error("CreatedAt should be recent")
	}
}

func TestNewVerbatimWithNilRoom(t *testing.T) {
	v := NewVerbatim("test", "wing", nil)
	if v.Room != nil {
		t.Error("Room should be nil when not provided")
	}
}

func TestVerbatimWithTokenCount(t *testing.T) {
	v := NewVerbatim("test", "wing", nil)
	v.WithTokenCount(42)
	if v.TokenCount != 42 {
		t.Errorf("TokenCount = %d, want 42", v.TokenCount)
	}
}

func TestNewFingerprint(t *testing.T) {
	verbatimID := uuid.New()
	fp := NewFingerprint(verbatimID, valueobjects.TypeDecision, "model-hash-123")

	if fp.ID == uuid.Nil {
		t.Error("Fingerprint should have a non-nil UUID")
	}
	if fp.VerbatimID != verbatimID {
		t.Error("VerbatimID should match")
	}
	if fp.Type != valueobjects.TypeDecision {
		t.Errorf("Type = %v, want TypeDecision", fp.Type)
	}
	if fp.ModelHash != "model-hash-123" {
		t.Errorf("ModelHash = %s, want 'model-hash-123'", fp.ModelHash)
	}
	if fp.Entities == nil {
		t.Error("Entities should be initialized")
	}
	if fp.Subjects == nil {
		t.Error("Subjects should be initialized")
	}
}

func TestFingerprintWithData(t *testing.T) {
	verbatimID := uuid.New()
	fp := NewFingerprint(verbatimID, valueobjects.TypeDecision, "hash")

	data := valueobjects.FingerprintData{
		Decision: "Use PostgreSQL",
		Subject:  []string{"database"},
		Entities: []string{"PostgreSQL", "MySQL"},
	}

	fp.WithData(data)

	if fp.Data.Decision != "Use PostgreSQL" {
		t.Error("Data.Decision not set correctly")
	}
	if len(fp.Entities) != 2 {
		t.Errorf("Entities length = %d, want 2", len(fp.Entities))
	}
	if len(fp.Subjects) != 1 || fp.Subjects[0] != "database" {
		t.Error("Subjects not set correctly")
	}
}

func TestFingerprintWithDataSetsDecision(t *testing.T) {
	fp := NewFingerprint(uuid.New(), valueobjects.TypeDecision, "hash")
	data := valueobjects.FingerprintData{
		Decision: "Migrate to cloud",
	}
	fp.WithData(data)

	if fp.Decision == nil || *fp.Decision != "Migrate to cloud" {
		t.Error("Decision pointer should be set")
	}
}

func TestFingerprintWithTokenEstimate(t *testing.T) {
	fp := NewFingerprint(uuid.New(), valueobjects.TypeFact, "hash")
	fp.WithTokenEstimate(150)

	if fp.TokenEstimate != 150 {
		t.Errorf("TokenEstimate = %d, want 150", fp.TokenEstimate)
	}
}

func TestFingerprintCalculateFactCount(t *testing.T) {
	tests := []struct {
		name     string
		data     valueobjects.FingerprintData
		expected int
	}{
		{
			name:     "empty data",
			data:     valueobjects.FingerprintData{},
			expected: 0,
		},
		{
			name: "with decision only",
			data: valueobjects.FingerprintData{
				Decision: "Use X",
			},
			expected: 1,
		},
		{
			name: "with rejected items",
			data: valueobjects.FingerprintData{
				Rejected: []string{"A", "B", "C"},
			},
			expected: 3,
		},
		{
			name: "with reasons",
			data: valueobjects.FingerprintData{
				Reason: []string{"R1", "R2"},
			},
			expected: 2,
		},
		{
			name: "with validated by",
			data: valueobjects.FingerprintData{
				ValidatedBy: "CTO",
			},
			expected: 1,
		},
		{
			name: "with assignee",
			data: valueobjects.FingerprintData{
				Assignee: "John",
			},
			expected: 1,
		},
		{
			name: "with deadline",
			data: valueobjects.FingerprintData{
				Deadline: "2024-12-31",
			},
			expected: 1,
		},
		{
			name: "with subject",
			data: valueobjects.FingerprintData{
				Subject: []string{"topic"},
			},
			expected: 1,
		},
		{
			name: "with empty subject",
			data: valueobjects.FingerprintData{
				Subject: []string{""},
			},
			expected: 0,
		},
		{
			name: "complete fingerprint",
			data: valueobjects.FingerprintData{
				Decision:    "Use X",
				Rejected:    []string{"Y"},
				Reason:      []string{"Z"},
				ValidatedBy: "Boss",
				Assignee:    "Me",
				Deadline:    "Soon",
				Subject:     []string{"Topic"},
			},
			expected: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := NewFingerprint(uuid.New(), valueobjects.TypeDecision, "hash")
			fp.WithData(tt.data)
			count := fp.CalculateFactCount()
			if count != tt.expected {
				t.Errorf("CalculateFactCount() = %d, want %d", count, tt.expected)
			}
			if fp.FactCount != tt.expected {
				t.Errorf("FactCount field = %d, want %d", fp.FactCount, tt.expected)
			}
		})
	}
}

func TestNewEmbedding(t *testing.T) {
	verbatimID := uuid.New()
	vector := []float32{0.1, 0.2, 0.3, 0.4}

	emb := NewEmbedding(verbatimID, "model-v1", vector)

	if emb.ID != verbatimID {
		t.Error("ID should match verbatimID")
	}
	if emb.ModelHash != "model-v1" {
		t.Errorf("ModelHash = %s, want 'model-v1'", emb.ModelHash)
	}
	if emb.Dim != 4 {
		t.Errorf("Dim = %d, want 4", emb.Dim)
	}
	if len(emb.Vector) != 4 {
		t.Errorf("Vector length = %d, want 4", len(emb.Vector))
	}
	if emb.Normalized {
		t.Error("Normalized should be false initially")
	}
}

func TestEmbeddingWithNormalization(t *testing.T) {
	vector := []float32{3.0, 4.0, 0.0} // Norm should be 5
	emb := NewEmbedding(uuid.New(), "model", vector)

	emb.WithNormalization()

	if !emb.Normalized {
		t.Error("Normalized should be true after WithNormalization")
	}

	// Check L2 norm is 1
	var norm float32
	for _, v := range emb.Vector {
		norm += v * v
	}
	if norm < 0.99 || norm > 1.01 {
		t.Errorf("L2 norm = %f, want ~1.0", norm)
	}
}

func TestEmbeddingWithNormalizationZeroVector(t *testing.T) {
	vector := []float32{0.0, 0.0, 0.0}
	emb := NewEmbedding(uuid.New(), "model", vector)

	emb.WithNormalization()

	if !emb.Normalized {
		t.Error("Normalized should be true even for zero vector")
	}
}

func TestNewCandidate(t *testing.T) {
	fp := NewFingerprint(uuid.New(), valueobjects.TypeFact, "hash")
	v := NewVerbatim("content", "wing", nil)
	embedding := []float32{0.1, 0.2}

	c := NewCandidate(fp, v, embedding)

	if c.Memory != fp {
		t.Error("Memory should be set")
	}
	if c.Verbatim != v {
		t.Error("Verbatim should be set")
	}
	if len(c.Embedding) != 2 {
		t.Error("Embedding should be set")
	}
	if c.Score != 0 {
		t.Error("Score should be 0 initially")
	}
}

func TestCandidateID(t *testing.T) {
	fp := NewFingerprint(uuid.New(), valueobjects.TypeFact, "hash")
	v := NewVerbatim("content", "wing", nil)
	c := NewCandidate(fp, v, nil)

	if c.ID() != fp.ID {
		t.Error("ID() should return fingerprint ID")
	}
}

func TestCandidateWithScores(t *testing.T) {
	fp := NewFingerprint(uuid.New(), valueobjects.TypeFact, "hash")
	v := NewVerbatim("content", "wing", nil)
	c := NewCandidate(fp, v, nil)

	c.WithScores(0.8, 0.6, 0.9)

	if c.Relevance != 0.8 {
		t.Errorf("Relevance = %f, want 0.8", c.Relevance)
	}
	if c.Density != 0.6 {
		t.Errorf("Density = %f, want 0.6", c.Density)
	}
	if c.Recency != 0.9 {
		t.Errorf("Recency = %f, want 0.9", c.Recency)
	}
}

func TestNewCausalNode(t *testing.T) {
	id := uuid.New()
	node := NewCausalNode(id, "decision", "Use PostgreSQL", "backend", nil)

	if node.ID != id {
		t.Error("ID should match")
	}
	if node.Type != "decision" {
		t.Errorf("Type = %s, want 'decision'", node.Type)
	}
	if node.Summary != "Use PostgreSQL" {
		t.Errorf("Summary = %s, want 'Use PostgreSQL'", node.Summary)
	}
	if node.Wing != "backend" {
		t.Errorf("Wing = %s, want 'backend'", node.Wing)
	}
	if node.Room != nil {
		t.Error("Room should be nil")
	}
	if time.Since(node.Timestamp) > time.Second {
		t.Error("Timestamp should be recent")
	}
}

func TestNewCausalEdge(t *testing.T) {
	fromID := uuid.New()
	toID := uuid.New()
	relation := valueobjects.RelBecause

	edge := NewCausalEdge(fromID, toID, relation)

	if edge.FromID != fromID {
		t.Error("FromID should match")
	}
	if edge.ToID != toID {
		t.Error("ToID should match")
	}
	if edge.Relation != relation {
		t.Error("Relation should match")
	}
	if edge.Weight != 0.7 {
		t.Errorf("Weight = %f, want 0.7", edge.Weight)
	}
	if time.Since(edge.DetectedAt) > time.Second {
		t.Error("DetectedAt should be recent")
	}
}

func TestNewEmbeddingModel(t *testing.T) {
	model := NewEmbeddingModel("sentence-transformers/all-MiniLM-L6-v2", 384)

	if model.ModelName != "sentence-transformers/all-MiniLM-L6-v2" {
		t.Error("ModelName not set correctly")
	}
	if model.Dimension != 384 {
		t.Errorf("Dimension = %d, want 384", model.Dimension)
	}
	if model.ModelHash == "" {
		t.Error("ModelHash should be computed")
	}
	if model.Metadata == nil {
		t.Error("Metadata should be initialized")
	}
	if time.Since(model.CreatedAt) > time.Second {
		t.Error("CreatedAt should be recent")
	}
}

func TestEmbeddingModelWithMetadata(t *testing.T) {
	model := NewEmbeddingModel("test-model", 128)
	model.WithMetadata("batch_size", 32)
	model.WithMetadata("framework", "pytorch")

	if model.Metadata["batch_size"] != 32 {
		t.Error("batch_size metadata not set")
	}
	if model.Metadata["framework"] != "pytorch" {
		t.Error("framework metadata not set")
	}
}

func TestComputeModelHash(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"short-model", "short-model"},
		{"very-long-model-name-that-exceeds-sixteen-characters", "very-long-model-"},
		{"exactly-sixteen", "exactly-sixteen"},
	}

	for _, tt := range tests {
		result := computeModelHash(tt.input)
		if result != tt.expected {
			t.Errorf("computeModelHash(%s) = %s, want %s", tt.input, result, tt.expected)
		}
	}
}
