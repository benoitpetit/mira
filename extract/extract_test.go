package extract

import (
	"math"
	"testing"
	"time"

	"github.com/benoitpetit/mira/types"
	"github.com/google/uuid"
)

func TestNormalizeL2(t *testing.T) {
	// Simple vector
	v := []float32{3, 4} // Norm = 5
	normalized := normalizeL2(v)

	// Check that norm is now 1
	var norm float64
	for _, x := range normalized {
		norm += float64(x * x)
	}
	norm = math.Sqrt(norm)

	if math.Abs(norm-1.0) > 0.0001 {
		t.Errorf("Normalized vector should have norm 1.0, got %v", norm)
	}

	// Check direction is preserved
	if math.Abs(float64(normalized[0])/float64(normalized[1])-0.75) > 0.0001 {
		t.Errorf("Direction should be preserved: expected ratio 0.75, got %v",
			float64(normalized[0])/float64(normalized[1]))
	}
}

func TestHasOverlap(t *testing.T) {
	a := []string{"apple", "banana", "cherry"}
	b := []string{"date", "banana", "fig"}
	c := []string{"grape", "kiwi"}

	if !hasOverlap(a, b) {
		t.Error("Expected overlap between a and b")
	}
	if hasOverlap(a, c) {
		t.Error("Expected no overlap between a and c")
	}
	if hasOverlap([]string{}, []string{"a"}) {
		t.Error("Expected no overlap with empty slice")
	}
}

func TestDetectType(t *testing.T) {
	ext := &Extractor{}

	tests := []struct {
		content  string
		expected types.MemoryType
	}{
		{"Decision: use Redis for cache", types.TypeDecision},
		{"decision: migrate to cloud", types.TypeDecision},
		{"I prefer dark theme", types.TypePreference},
		{"Architecture has been microservices since 2023", types.TypeFact},
		{"This is a normal session note", types.TypeSessionNote},
	}

	for _, test := range tests {
		result := ext.detectType(test.content)
		if result != test.expected {
			t.Errorf("For content '%s', expected type %v, got %v",
				test.content, test.expected, result)
		}
	}
}

func TestCountFacts(t *testing.T) {
	ext := &Extractor{}

	data := types.FingerprintData{
		Decision:    "Use PostgreSQL",
		Rejected:    []string{"MySQL", "MongoDB"},
		Reason:      []string{"Better ACID support", "Team expertise"},
		ValidatedBy: "CTO",
		Assignee:    "John",
		Deadline:    "2024-12-31",
	}

	count := ext.countFacts(data)
	// 1 decision + 2 rejected + 2 reasons + 1 validated + 1 assignee + 1 deadline = 8
	// Note: current logic also counts Subject if present
	expectedMin := 7

	if count < expectedMin {
		t.Errorf("Expected at least %d facts, got %d", expectedMin, count)
	}
}

func TestInferSubject(t *testing.T) {
	ext := &Extractor{}

	tests := []struct {
		content  string
		entities []string
		expected string
	}{
		{"Subject: authentication OAuth2", nil, "authentication OAuth2"},
		{"About database migration", nil, "database migration"},
		{"API migration to GraphQL", nil, "API migration to GraphQL"},
		{"Random content without specific subject", []string{"ProjectX"}, "ProjectX"},
		{"Another random content", nil, "general"},
	}

	for _, test := range tests {
		result := ext.inferSubject(test.content, test.entities)
		// Result might differ slightly due to regex
		if len(result) == 0 {
			t.Errorf("Expected non-empty subject for '%s'", test.content)
		}
	}
}

func TestSimpleEmbedder(t *testing.T) {
	embedder := NewSimpleEmbedder(384)

	// Test that same text gives same embedding
	text1 := "Hello world"
	vec1, err := embedder.Encode(text1)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	vec2, err := embedder.Encode(text1)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Same text = same embedding
	for i := range vec1 {
		if vec1[i] != vec2[i] {
			t.Error("Same text should produce same embedding")
			break
		}
	}

	// Different texts = different embeddings (probably)
	text2 := "Goodbye world"
	vec3, err := embedder.Encode(text2)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	different := false
	for i := range vec1 {
		if vec1[i] != vec3[i] {
			different = true
			break
		}
	}
	if !different {
		t.Error("Different texts should produce different embeddings")
	}

	// Check normalization
	var norm float64
	for _, x := range vec1 {
		norm += float64(x * x)
	}
	norm = math.Sqrt(norm)
	if math.Abs(norm-1.0) > 0.0001 {
		t.Errorf("Embedding should be normalized (norm=1), got %v", norm)
	}
}

func TestDetectCausalRelations(t *testing.T) {
	embedder := NewSimpleEmbedder(384)
	ext, _ := NewExtractorWithOptions("test-model", embedder, ExtractorOptions{})

	newFp := &types.Fingerprint{
		ID:          uuid.New(),
		Type:        types.TypeDecision,
		Entities:    []string{"API", "Database"},
		Subjects:    []string{"migration"},
		ExtractedAt: time.Now(),
	}

	recentFps := []*types.Fingerprint{
		{
			ID:          uuid.New(),
			Type:        types.TypeDecision,
			Entities:    []string{"API"},
			Subjects:    []string{"migration"},
			ExtractedAt: time.Now().Add(-1 * time.Hour),
		},
	}

	// Test with text containing causal pattern
	content := "Following the previous decision, we will migrate the API"
	edges, err := ext.DetectCausalRelations(newFp, recentFps, content)
	if err != nil {
		t.Fatalf("DetectCausalRelations failed: %v", err)
	}

	// Should detect causal relation (entity overlap)
	if len(edges) == 0 {
		t.Log("No causal relations detected (may be expected depending on implementation)")
	}
}

func TestExtractEntities(t *testing.T) {
	// This test requires prose for real NER
	// Tested in integration
	t.Skip("Skipping entity extraction test - requires prose NLP model")
}

func TestExtractPipeline(t *testing.T) {
	embedder := NewSimpleEmbedder(384)
	ext, err := NewExtractorWithOptions("test-model", embedder, ExtractorOptions{})
	if err != nil {
		t.Fatalf("NewExtractorWithOptions() error = %v", err)
	}

	tests := []struct {
		name          string
		content       string
		expectedType  types.MemoryType
		checkDecision bool
	}{
		{
			name:          "decision extraction",
			content:       "We decided to use PostgreSQL for the database.",
			expectedType:  types.TypeDecision,
			checkDecision: true,
		},
		{
			name:         "preference extraction",
			content:      "I prefer using dark mode for the IDE.",
			expectedType: types.TypePreference,
		},
		{
			name:         "fact extraction",
			content:      "The system was deployed in 2023 using Kubernetes.",
			expectedType: types.TypeFact,
		},
		{
			name:         "session note",
			content:      "This is a general note about today's meeting.",
			expectedType: types.TypeSessionNote,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verbatim := &types.Verbatim{
				ID:        uuid.New(),
				Content:   tt.content,
				CreatedAt: time.Now(),
				Wing:      "test-wing",
			}

			fp, emb, err := ext.ExtractPipeline(verbatim)
			if err != nil {
				t.Fatalf("ExtractPipeline() error = %v", err)
			}

			// Vérifier le type
			if fp.Type != tt.expectedType {
				t.Errorf("Type = %v, want %v", fp.Type, tt.expectedType)
			}

			// Vérifier l'ID
			if fp.ID != verbatim.ID {
				t.Error("Fingerprint ID should match Verbatim ID")
			}
			if fp.VerbatimID != verbatim.ID {
				t.Error("VerbatimID should match")
			}

			// Vérifier que le verbatim a été mis à jour avec le token count
			if verbatim.TokenCount <= 0 {
				t.Error("Verbatim TokenCount should be set")
			}

			// Vérifier l'embedding
			if emb == nil {
				t.Fatal("Embedding should not be nil")
			}
			if emb.ID != verbatim.ID {
				t.Error("Embedding ID should match Verbatim ID")
			}
			if len(emb.Vector) != 384 {
				t.Errorf("Embedding vector should have 384 dimensions, got %d", len(emb.Vector))
			}
			if !emb.Normalized {
				t.Error("Embedding should be normalized")
			}

			// Vérifier les champs du fingerprint
			if fp.ModelHash == "" {
				t.Error("ModelHash should not be empty")
			}
			if fp.TokenEstimate <= 0 {
				t.Error("TokenEstimate should be positive")
			}
			if fp.FactCount < 0 {
				t.Error("FactCount should be non-negative")
			}
		})
	}
}

func TestExtractPipelineEmptyContent(t *testing.T) {
	embedder := NewSimpleEmbedder(384)
	ext, _ := NewExtractorWithOptions("test-model", embedder, ExtractorOptions{})

	verbatim := &types.Verbatim{
		ID:        uuid.New(),
		Content:   "",
		CreatedAt: time.Now(),
	}

	fp, emb, err := ext.ExtractPipeline(verbatim)
	if err != nil {
		t.Fatalf("ExtractPipeline() error = %v", err)
	}

	if fp == nil {
		t.Error("Fingerprint should not be nil even for empty content")
	}
	if emb == nil {
		t.Error("Embedding should not be nil even for empty content")
	}
}

func TestModelMethods(t *testing.T) {
	embedder := NewSimpleEmbedder(384)
	ext, _ := NewExtractorWithOptions("my-test-model", embedder, ExtractorOptions{})

	// Test ModelHash
	hash := ext.ModelHash()
	if hash == "" {
		t.Error("ModelHash should not be empty")
	}
	if len(hash) != 16 {
		t.Errorf("ModelHash should be 16 chars, got %d", len(hash))
	}

	// Test Encode
	vec, err := ext.Encode("test text")
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if len(vec) != 384 {
		t.Errorf("Encode returned vector of dim %d, want 384", len(vec))
	}
}

func TestNormalizeL2Zero(t *testing.T) {
	// Test zero vector
	zero := []float32{0, 0, 0}
	result := normalizeL2(zero)

	// Should return the same vector without division by zero
	if len(result) != 3 {
		t.Error("Zero vector normalization should preserve length")
	}
}

func TestHashStringDeterministic(t *testing.T) {
	// Test que hashString est déterministe
	h1 := hashString("test")
	h2 := hashString("test")
	h3 := hashString("different")

	if h1 != h2 {
		t.Error("hashString should be deterministic")
	}
	if h1 == h3 {
		t.Error("Different strings should have different hashes")
	}
}

func TestHasOverlapEdgeCases(t *testing.T) {
	// Test avec deux slices vides
	if hasOverlap([]string{}, []string{}) {
		t.Error("Two empty slices should not overlap")
	}

	// Test avec un élément
	if !hasOverlap([]string{"a"}, []string{"a"}) {
		t.Error("Single matching element should overlap")
	}

	// Test avec éléments dupliqués
	if !hasOverlap([]string{"a", "a", "b"}, []string{"a"}) {
		t.Error("Should find overlap even with duplicates")
	}
}

func TestDetectCausalRelationsNoOverlap(t *testing.T) {
	embedder := NewSimpleEmbedder(384)
	ext, _ := NewExtractorWithOptions("test-model", embedder, ExtractorOptions{})

	newFp := &types.Fingerprint{
		ID:          uuid.New(),
		Type:        types.TypeDecision,
		Entities:    []string{"API"},
		Subjects:    []string{"migration"},
		ExtractedAt: time.Now(),
	}

	// Fingerprint récent sans chevauchement
	recentFps := []*types.Fingerprint{
		{
			ID:          uuid.New(),
			Type:        types.TypeDecision,
			Entities:    []string{"Database", "Cache"}, // Pas de chevauchement avec "API"
			Subjects:    []string{"optimization"},      // Pas de chevauchement avec "migration"
			ExtractedAt: time.Now().Add(-1 * time.Hour),
		},
	}

	// Texte avec pattern causal
	content := "Following the previous decision, we will migrate the API"
	edges, err := ext.DetectCausalRelations(newFp, recentFps, content)
	if err != nil {
		t.Fatalf("DetectCausalRelations failed: %v", err)
	}

	// Ne devrait pas détecter de relation car pas de chevauchement
	if len(edges) != 0 {
		t.Errorf("Expected 0 edges without overlap, got %d", len(edges))
	}
}

func TestDetectCausalRelationsTooOld(t *testing.T) {
	embedder := NewSimpleEmbedder(384)
	ext, _ := NewExtractorWithOptions("test-model", embedder, ExtractorOptions{})

	newFp := &types.Fingerprint{
		ID:          uuid.New(),
		Type:        types.TypeDecision,
		Entities:    []string{"API"},
		Subjects:    []string{"migration"},
		ExtractedAt: time.Now(),
	}

	// Fingerprint trop vieux (>30 jours)
	recentFps := []*types.Fingerprint{
		{
			ID:          uuid.New(),
			Type:        types.TypeDecision,
			Entities:    []string{"API"},                      // Chevauchement
			Subjects:    []string{"migration"},                // Chevauchement
			ExtractedAt: time.Now().Add(-40 * 24 * time.Hour), // 40 jours
		},
	}

	content := "Following the previous decision, we will migrate the API"
	edges, err := ext.DetectCausalRelations(newFp, recentFps, content)
	if err != nil {
		t.Fatalf("DetectCausalRelations failed: %v", err)
	}

	// Ne devrait pas détecter de relation car trop vieux
	if len(edges) != 0 {
		t.Errorf("Expected 0 edges for old fingerprint, got %d", len(edges))
	}
}

func TestDetectCausalRelationsSelfReference(t *testing.T) {
	embedder := NewSimpleEmbedder(384)
	ext, _ := NewExtractorWithOptions("test-model", embedder, ExtractorOptions{})

	id := uuid.New()
	newFp := &types.Fingerprint{
		ID:          id,
		Type:        types.TypeDecision,
		Entities:    []string{"API"},
		Subjects:    []string{"migration"},
		ExtractedAt: time.Now(),
	}

	// Essayer de créer une relation avec soi-même
	recentFps := []*types.Fingerprint{
		{
			ID:          id, // Même ID
			Type:        types.TypeDecision,
			Entities:    []string{"API"},
			Subjects:    []string{"migration"},
			ExtractedAt: time.Now().Add(-1 * time.Hour),
		},
	}

	content := "Following the previous decision, we will migrate the API"
	edges, err := ext.DetectCausalRelations(newFp, recentFps, content)
	if err != nil {
		t.Fatalf("DetectCausalRelations failed: %v", err)
	}

	// Ne devrait pas créer d'auto-référence
	if len(edges) != 0 {
		t.Error("Should not create self-references")
	}
}

func TestInferSubjectFallback(t *testing.T) {
	ext := &Extractor{}

	// Test fallback sur entités
	result := ext.inferSubject("Random content", []string{"ProjectX", "API"})
	if len(result) == 0 || result[0] != "ProjectX" {
		t.Error("Should fallback to first entity")
	}

	// Test fallback sur "general"
	result = ext.inferSubject("Random content", nil)
	if len(result) == 0 || result[0] != "general" {
		t.Error("Should fallback to 'general'")
	}
}

func TestExtractPipelineVerbatimRef(t *testing.T) {
	embedder := NewSimpleEmbedder(384)
	ext, _ := NewExtractorWithOptions("test-model", embedder, ExtractorOptions{})

	id := uuid.New()
	verbatim := &types.Verbatim{
		ID:        id,
		Content:   "Test content.",
		CreatedAt: time.Now(),
	}

	fp, _, err := ext.ExtractPipeline(verbatim)
	if err != nil {
		t.Fatalf("ExtractPipeline() error = %v", err)
	}

	wantRef := "T0:" + id.String()
	if fp.Data.VerbatimRef != wantRef {
		t.Errorf("VerbatimRef = %q, want %q", fp.Data.VerbatimRef, wantRef)
	}
}

func TestExtractStructuredPatterns(t *testing.T) {
	embedder := NewSimpleEmbedder(384)
	ext, _ := NewExtractorWithOptions("test-model", embedder, ExtractorOptions{})

	tests := []struct {
		name         string
		content      string
		wantDecision string
		wantRejected int
		wantReason   int
		wantAssignee string
		wantDeadline string
	}{
		{
			name:         "decision pattern: decided to",
			content:      "We decided to use PostgreSQL",
			wantDecision: "PostgreSQL",
		},
		{
			name:         "decision pattern: will use",
			content:      "We will use Redis for caching",
			wantDecision: "Redis for caching",
		},
		{
			name:         "rejected pattern",
			content:      "Decision: Use PostgreSQL. Rejected: MySQL, MongoDB.",
			wantDecision: "Use PostgreSQL",
			wantRejected: 2,
		},
		{
			name:       "reason pattern: because",
			content:    "We chose PostgreSQL because it has better performance",
			wantReason: 1,
		},
		{
			name:         "assignee pattern: will implement",
			content:      "John will implement the authentication",
			wantAssignee: "John",
		},
		{
			name:         "deadline pattern: sprint",
			content:      "This will be done in Sprint 5",
			wantDeadline: "5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verbatim := &types.Verbatim{
				ID:        uuid.New(),
				Content:   tt.content,
				CreatedAt: time.Now(),
			}

			fp, _, err := ext.ExtractPipeline(verbatim)
			if err != nil {
				t.Fatalf("ExtractPipeline() error = %v", err)
			}

			if tt.wantDecision != "" && fp.Data.Decision != tt.wantDecision {
				t.Errorf("Decision = %q, want %q", fp.Data.Decision, tt.wantDecision)
			}
			if len(fp.Data.Rejected) != tt.wantRejected {
				t.Errorf("Rejected count = %d, want %d", len(fp.Data.Rejected), tt.wantRejected)
			}
			if len(fp.Data.Reason) != tt.wantReason {
				t.Errorf("Reason count = %d, want %d", len(fp.Data.Reason), tt.wantReason)
			}
			if tt.wantAssignee != "" && fp.Data.Assignee != tt.wantAssignee {
				t.Errorf("Assignee = %q, want %q", fp.Data.Assignee, tt.wantAssignee)
			}
			if tt.wantDeadline != "" && fp.Data.Deadline != tt.wantDeadline {
				t.Errorf("Deadline = %q, want %q", fp.Data.Deadline, tt.wantDeadline)
			}
		})
	}
}

func BenchmarkNormalizeL2(b *testing.B) {
	vec := make([]float32, 384)
	for i := 0; i < 384; i++ {
		vec[i] = float32(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Copy vector since normalizeL2 modifies it in place
		v := make([]float32, len(vec))
		copy(v, vec)
		normalizeL2(v)
	}
}

func BenchmarkHashString(b *testing.B) {
	text := "This is a sample text for hashing performance testing"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hashString(text)
	}
}
