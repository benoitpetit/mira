package extraction

import (
	"context"
	"testing"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
)

func newTestExtractor(t *testing.T) *NativeExtractor {
	t.Helper()
	embedder := NewSimpleEmbedder(384)
	e, err := NewNativeExtractor(embedder, NativeExtractorOptions{ModelName: "test-model"})
	if err != nil {
		t.Fatalf("NewNativeExtractor failed: %v", err)
	}
	return e
}

func TestDetectCausalRelations_English(t *testing.T) {
	e := newTestExtractor(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		content string
		wantRel valueobjects.RelationType
	}{
		{"triggered", "This was done following the previous decision", valueobjects.RelTriggered},
		{"because", "We chose this because the old system failed", valueobjects.RelBecause},
		{"contradicts", "This new policy contradicts the earlier one", valueobjects.RelContradicts},
		{"updates", "The document updates the previous version", valueobjects.RelUpdates},
		{"resolves", "This fix resolves the memory leak", valueobjects.RelResolves},
	}

	recentFp := &entities.Fingerprint{Subjects: []string{"decision"}, Entities: []string{"system"}}
	recentFp.ID = [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	newFp := &entities.Fingerprint{Subjects: []string{"decision"}, Entities: []string{"system"}}
	newFp.ID = [16]byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edges, err := e.DetectCausalRelations(ctx, newFp, []*entities.Fingerprint{recentFp}, tt.content)
			if err != nil {
				t.Fatalf("DetectCausalRelations error: %v", err)
			}
			if len(edges) == 0 {
				t.Fatalf("expected at least one edge for %q", tt.content)
			}
			if edges[0].Relation != tt.wantRel {
				t.Errorf("got relation %q, want %q", edges[0].Relation, tt.wantRel)
			}
		})
	}
}

func TestDetectCausalRelations_French(t *testing.T) {
	e := newTestExtractor(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		content string
		wantRel valueobjects.RelationType
	}{
		{"triggered", "Cela a été fait suite à la décision précédente", valueobjects.RelTriggered},
		{"because", "Nous avons choisi cela parce que l'ancien système a échoué", valueobjects.RelBecause},
		{"contradicts", "Cette nouvelle politique contredit l'ancienne", valueobjects.RelContradicts},
		{"updates", "Le document met à jour la version précédente", valueobjects.RelUpdates},
		{"resolves", "Ce correctif résout le problème", valueobjects.RelResolves},
	}

	recentFp := &entities.Fingerprint{Subjects: []string{"decision"}, Entities: []string{"system"}}
	recentFp.ID = [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	newFp := &entities.Fingerprint{Subjects: []string{"decision"}, Entities: []string{"system"}}
	newFp.ID = [16]byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edges, err := e.DetectCausalRelations(ctx, newFp, []*entities.Fingerprint{recentFp}, tt.content)
			if err != nil {
				t.Fatalf("DetectCausalRelations error: %v", err)
			}
			if len(edges) == 0 {
				t.Fatalf("expected at least one edge for %q", tt.content)
			}
			if edges[0].Relation != tt.wantRel {
				t.Errorf("got relation %q, want %q", edges[0].Relation, tt.wantRel)
			}
		})
	}
}

func TestDetectCausalRelations_NoFalsePositive_However(t *testing.T) {
	e := newTestExtractor(t)
	ctx := context.Background()

	recentFp := &entities.Fingerprint{}
	recentFp.ID = [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	newFp := &entities.Fingerprint{}
	newFp.ID = [16]byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}

	content := "It's fast but also reliable however we need to check"
	edges, err := e.DetectCausalRelations(ctx, newFp, []*entities.Fingerprint{recentFp}, content)
	if err != nil {
		t.Fatalf("DetectCausalRelations error: %v", err)
	}

	for _, edge := range edges {
		if edge.Relation == valueobjects.RelContradicts {
			t.Errorf("'however' should not trigger RelContradicts, got edge with relation %q", edge.Relation)
		}
	}
}

func TestDetectType_Decision(t *testing.T) {
	e := newTestExtractor(t)
	content := "The team decided to adopt Kubernetes for deployment."
	got := e.detectType(content)
	if got != valueobjects.TypeDecision {
		t.Errorf("detectType(%q) = %q, want %q", content, got, valueobjects.TypeDecision)
	}
}

func TestDetectType_Preference(t *testing.T) {
	e := newTestExtractor(t)
	content := "I prefer using Go over Python for this service."
	got := e.detectType(content)
	if got != valueobjects.TypePreference {
		t.Errorf("detectType(%q) = %q, want %q", content, got, valueobjects.TypePreference)
	}
}

func TestDetectType_Fact(t *testing.T) {
	e := newTestExtractor(t)
	content := "The database connection pool max is 100 connections."
	got := e.detectType(content)
	if got != valueobjects.TypeFact {
		t.Errorf("detectType(%q) = %q, want %q", content, got, valueobjects.TypeFact)
	}
}

func TestDetectType_SessionNote(t *testing.T) {
	e := newTestExtractor(t)
	content := "We had a good discussion about the roadmap today."
	got := e.detectType(content)
	if got != valueobjects.TypeSessionNote {
		t.Errorf("detectType(%q) = %q, want %q", content, got, valueobjects.TypeSessionNote)
	}
}

func TestExtractEntities_EmailAndURL(t *testing.T) {
	e := newTestExtractor(t)
	content := "Contact us at support@example.com or visit https://example.com/help"
	tokens := e.tokenize(content)
	entities := e.extractEntities(tokens, content)

	hasEmail := false
	hasURL := false
	for _, ent := range entities {
		if ent == "support@example.com" {
			hasEmail = true
		}
		if ent == "https://example.com/help" {
			hasURL = true
		}
	}

	if !hasEmail {
		t.Error("expected email entity not found")
	}
	if !hasURL {
		t.Error("expected URL entity not found")
	}
}

func TestExtractEntities_Gazetteer(t *testing.T) {
	e := newTestExtractor(t)
	content := "Microsoft and Google are competing in Paris."
	tokens := e.tokenize(content)
	entities := e.extractEntities(tokens, content)

	hasMicrosoft := false
	hasParis := false
	for _, ent := range entities {
		if ent == "Microsoft" {
			hasMicrosoft = true
		}
		if ent == "Paris" {
			hasParis = true
		}
	}

	if !hasMicrosoft {
		t.Error("expected 'Microsoft' from gazetteer not found")
	}
	if !hasParis {
		t.Error("expected 'Paris' from gazetteer not found")
	}
}

func TestNegationDetection(t *testing.T) {
	e := newTestExtractor(t)

	tests := []struct {
		content  string
		negated  bool
	}{
		{"I do not like this approach.", true},
		{"We decided to use Kubernetes.", false},
		{"This is never going to work.", true},
		{"She doesn't approve the change.", true},
		{"The database max is 100.", false},
	}

	for _, tt := range tests {
		t.Run(tt.content, func(t *testing.T) {
			v := &entities.Verbatim{Content: tt.content}
			memType := e.detectType(tt.content)
			tokens := e.tokenize(tt.content)
			extractedEntities := e.extractEntities(tokens, tt.content)
			data := e.extractStructured(v, tokens, extractedEntities, memType)
			if data.Negated != tt.negated {
				t.Errorf("negation=%v, want %v", data.Negated, tt.negated)
			}
		})
	}
}

func TestValidateCrossT0T1(t *testing.T) {
	e := newTestExtractor(t)

	// Valid case: entities present in verbatim
	content := "Microsoft decided to adopt Kubernetes."
	v := &entities.Verbatim{Content: content, TokenCount: 10}
	memType := e.detectType(content)
	tokens := e.tokenize(content)
	extractedEntities := e.extractEntities(tokens, content)
	data := e.extractStructured(v, tokens, extractedEntities, memType)
	fp := entities.NewFingerprint(v.ID, memType, e.modelHash)
	fp.WithData(data).WithTokenEstimate(5)
	alerts := e.validateCrossT0T1(v, fp)
	if len(alerts) > 0 {
		t.Errorf("expected no alerts for coherent content, got %v", alerts)
	}

	// Invalid case: force a decision type but empty decision data
	v2 := &entities.Verbatim{Content: "Random note without decision.", TokenCount: 10}
	fp2 := entities.NewFingerprint(v2.ID, valueobjects.TypeDecision, e.modelHash)
	fp2.WithData(valueobjects.FingerprintData{Decision: ""}).WithTokenEstimate(5)
	alerts2 := e.validateCrossT0T1(v2, fp2)
	found := false
	for _, a := range alerts2 {
		if a == "type=Decision but no decision extracted" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'type=Decision but no decision extracted' alert, got %v", alerts2)
	}
}

// TestIsCommonWord verifies the common-word filter.
func TestIsCommonWord(t *testing.T) {
	common := []string{"The", "A", "In", "On", "For", "We", "They", "Is", "Are", "Will", "Can"}
	for _, w := range common {
		if !isCommonWord(w) {
			t.Errorf("isCommonWord(%q): expected true", w)
		}
	}
	notCommon := []string{"Microsoft", "Paris", "Kubernetes", "backend", "decision"}
	for _, w := range notCommon {
		if isCommonWord(w) {
			t.Errorf("isCommonWord(%q): expected false", w)
		}
	}
}

// TestNormalizeL2 verifies that the resulting vector has unit L2 norm.
func TestNormalizeL2(t *testing.T) {
	e := newTestExtractor(t)

	vec := []float32{3, 4} // norm = 5 → normalized = [0.6, 0.8]
	out := e.normalizeL2(vec)
	var sumSq float64
	for _, v := range out {
		sumSq += float64(v * v)
	}
	if sumSq < 0.999 || sumSq > 1.001 {
		t.Errorf("expected unit norm, got sum-of-squares %f", sumSq)
	}

	// Zero vector should be returned unchanged (no division by zero)
	zero := []float32{0, 0, 0}
	outZero := e.normalizeL2(zero)
	for i, v := range outZero {
		if v != 0 {
			t.Errorf("zero vector[%d]: expected 0, got %f", i, v)
		}
	}
}

// TestModelHash verifies that ModelHash returns a non-empty deterministic hash.
func TestModelHash(t *testing.T) {
	e := newTestExtractor(t)
	h := e.ModelHash()
	if h == "" {
		t.Error("ModelHash should not be empty")
	}
	// Calling again should return the same value (deterministic).
	if e.ModelHash() != h {
		t.Error("ModelHash should be deterministic")
	}
}

// TestEncode delegates to the internal embedder; verify shape and no error.
func TestEncode(t *testing.T) {
	e := newTestExtractor(t)
	vec, err := e.Encode(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}
	if len(vec) != 384 {
		t.Errorf("Encode: expected vector of length 384, got %d", len(vec))
	}
}

// TestExtractPipeline_EndToEnd verifies the full pipeline returns consistent results.
func TestExtractPipeline_EndToEnd(t *testing.T) {
	e := newTestExtractor(t)
	ctx := context.Background()

	v := &entities.Verbatim{
		Content: "We decided to migrate to PostgreSQL because the old SQLite setup was too slow.",
		Wing:    "engineering",
	}
	v.ID = [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	fp, emb, err := e.ExtractPipeline(ctx, v, nil)
	if err != nil {
		t.Fatalf("ExtractPipeline failed: %v", err)
	}
	if fp == nil {
		t.Fatal("expected non-nil Fingerprint")
	}
	if emb == nil {
		t.Fatal("expected non-nil Embedding")
	}
	if fp.VerbatimID != v.ID {
		t.Errorf("fingerprint VerbatimID: want %s, got %s", v.ID, fp.VerbatimID)
	}
	if emb.ID != v.ID {
		t.Errorf("embedding ID: want %s, got %s", v.ID, emb.ID)
	}
	// Should detect a decision ("decided to")
	if fp.Type != valueobjects.TypeDecision {
		t.Errorf("expected TypeDecision, got %s", fp.Type)
	}
	// Embedding should be unit-norm (normalizeL2 applied)
	var sumSq float64
	for _, vv := range emb.Vector {
		sumSq += float64(vv * vv)
	}
	if sumSq < 0.999 || sumSq > 1.001 {
		t.Errorf("embedding should be L2-normalized, sum-of-squares=%f", sumSq)
	}
}

// TestExtractPipeline_ForcedType verifies that forcedType overrides auto-detection.
func TestExtractPipeline_ForcedType(t *testing.T) {
	e := newTestExtractor(t)
	ctx := context.Background()

	v := &entities.Verbatim{Content: "Some random note.", Wing: "engineering"}
	v.ID = [16]byte{9, 8, 7}

	forced := valueobjects.TypeFact
	fp, _, err := e.ExtractPipeline(ctx, v, &forced)
	if err != nil {
		t.Fatalf("ExtractPipeline failed: %v", err)
	}
	if fp.Type != valueobjects.TypeFact {
		t.Errorf("expected forced TypeFact, got %s", fp.Type)
	}
}

// ── hasSemanticOverlap ────────────────────────────────────────────────────────

func TestHasSemanticOverlap_SubjectMatch(t *testing.T) {
	a := &entities.Fingerprint{Subjects: []string{"golang"}, Entities: []string{}}
	b := &entities.Fingerprint{Subjects: []string{"golang"}, Entities: []string{}}
	if !hasSemanticOverlap(a, b) {
		t.Error("expected overlap via shared subject")
	}
}

func TestHasSemanticOverlap_EntityInBMatchesSubjectInA(t *testing.T) {
	// a has subject "golang", b has entity "golang" but no shared subject → entity loop covers it
	a := &entities.Fingerprint{Subjects: []string{"golang"}, Entities: []string{}}
	b := &entities.Fingerprint{Subjects: []string{}, Entities: []string{"golang"}}
	if !hasSemanticOverlap(a, b) {
		t.Error("expected overlap: entity in b matches subject in a")
	}
}

func TestHasSemanticOverlap_EntityMatch(t *testing.T) {
	// a has only entities (no subjects), b has matching entity → covers the entity check in b
	a := &entities.Fingerprint{Subjects: []string{}, Entities: []string{"PostgreSQL"}}
	b := &entities.Fingerprint{Subjects: []string{}, Entities: []string{"PostgreSQL"}}
	if !hasSemanticOverlap(a, b) {
		t.Error("expected overlap via shared entity")
	}
}

func TestHasSemanticOverlap_NoOverlap(t *testing.T) {
	a := &entities.Fingerprint{Subjects: []string{"golang"}, Entities: []string{"Redis"}}
	b := &entities.Fingerprint{Subjects: []string{"python"}, Entities: []string{"MongoDB"}}
	if hasSemanticOverlap(a, b) {
		t.Error("expected no overlap")
	}
}

func TestHasSemanticOverlap_EmptyFingerprints(t *testing.T) {
	a := &entities.Fingerprint{}
	b := &entities.Fingerprint{}
	if hasSemanticOverlap(a, b) {
		t.Error("expected no overlap for empty fingerprints")
	}
}

// ── validateCrossT0T1 additional branches ────────────────────────────────────

func TestValidateCrossT0T1_EntityNotInVerbatim(t *testing.T) {
	e := newTestExtractor(t)
	v := &entities.Verbatim{Content: "We adopted Kubernetes.", TokenCount: 5}
	fp := entities.NewFingerprint(v.ID, valueobjects.TypeFact, e.ModelHash())
	// inject an entity that is NOT in the verbatim content
	fp.WithData(valueobjects.FingerprintData{Entities: []string{"PostgreSQL"}}).WithTokenEstimate(3)
	alerts := e.validateCrossT0T1(v, fp)
	found := false
	for _, a := range alerts {
		if a == `entity "PostgreSQL" not found in verbatim` {
			found = true
		}
	}
	if !found {
		t.Errorf("expected entity-not-found alert, got %v", alerts)
	}
}

func TestValidateCrossT0T1_FactWithDecision(t *testing.T) {
	e := newTestExtractor(t)
	v := &entities.Verbatim{Content: "The team decided to use Redis.", TokenCount: 10}
	fp := entities.NewFingerprint(v.ID, valueobjects.TypeFact, e.ModelHash())
	fp.WithData(valueobjects.FingerprintData{Decision: "use Redis"}).WithTokenEstimate(5)
	alerts := e.validateCrossT0T1(v, fp)
	found := false
	for _, a := range alerts {
		if a == "type=Fact but decision field is populated" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'type=Fact but decision field is populated' alert, got %v", alerts)
	}
}

func TestValidateCrossT0T1_PreferenceNoPattern(t *testing.T) {
	e := newTestExtractor(t)
	v := &entities.Verbatim{Content: "We adopted Kubernetes for deployment.", TokenCount: 8}
	fp := entities.NewFingerprint(v.ID, valueobjects.TypePreference, e.ModelHash())
	fp.WithData(valueobjects.FingerprintData{}).WithTokenEstimate(4)
	alerts := e.validateCrossT0T1(v, fp)
	found := false
	for _, a := range alerts {
		if a == "type=Preference but no preference pattern matched" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected preference-pattern alert, got %v", alerts)
	}
}

func TestValidateCrossT0T1_TokenRatioExceeded(t *testing.T) {
	e := newTestExtractor(t)
	v := &entities.Verbatim{Content: "Short note.", TokenCount: 2}
	fp := entities.NewFingerprint(v.ID, valueobjects.TypeFact, e.ModelHash())
	// TokenEstimate > verbatim.TokenCount*10 → 2*10=20, so 25 triggers alert
	fp.WithData(valueobjects.FingerprintData{}).WithTokenEstimate(25)
	alerts := e.validateCrossT0T1(v, fp)
	found := false
	for _, a := range alerts {
		if len(a) > 0 && a[:2] == "T1" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected token ratio alert, got %v", alerts)
	}
}

// ── extractStructured pattern branches ───────────────────────────────────────

func TestExtractStructured_DecisionPattern(t *testing.T) {
	e := newTestExtractor(t)
	content := "Decision: use PostgreSQL for persistence."
	v := &entities.Verbatim{Content: content}
	tokens := e.tokenize(content)
	ents := e.extractEntities(tokens, content)
	data := e.extractStructured(v, tokens, ents, valueobjects.TypeDecision)
	if data.Decision == "" {
		t.Errorf("expected non-empty Decision, got empty")
	}
}

func TestExtractStructured_RejectionPattern(t *testing.T) {
	e := newTestExtractor(t)
	content := "We chose Redis rather than Memcached, DynamoDB."
	v := &entities.Verbatim{Content: content}
	tokens := e.tokenize(content)
	ents := e.extractEntities(tokens, content)
	data := e.extractStructured(v, tokens, ents, valueobjects.TypeDecision)
	if len(data.Rejected) == 0 {
		t.Errorf("expected non-empty Rejected, got empty")
	}
}

func TestExtractStructured_ReasonPattern(t *testing.T) {
	e := newTestExtractor(t)
	content := "We adopted Go because it compiles fast."
	v := &entities.Verbatim{Content: content}
	tokens := e.tokenize(content)
	ents := e.extractEntities(tokens, content)
	data := e.extractStructured(v, tokens, ents, valueobjects.TypeDecision)
	if len(data.Reason) == 0 {
		t.Errorf("expected non-empty Reason, got empty")
	}
}

func TestExtractStructured_AssigneePattern(t *testing.T) {
	e := newTestExtractor(t)
	content := "Assigned to John for the migration task."
	v := &entities.Verbatim{Content: content}
	tokens := e.tokenize(content)
	ents := e.extractEntities(tokens, content)
	data := e.extractStructured(v, tokens, ents, valueobjects.TypeFact)
	if data.Assignee == "" {
		t.Errorf("expected non-empty Assignee, got empty")
	}
}

func TestExtractStructured_ValidationPattern(t *testing.T) {
	e := newTestExtractor(t)
	content := "John validated the architecture decision."
	v := &entities.Verbatim{Content: content}
	tokens := e.tokenize(content)
	ents := e.extractEntities(tokens, content)
	data := e.extractStructured(v, tokens, ents, valueobjects.TypeDecision)
	if data.ValidatedBy == "" {
		t.Errorf("expected non-empty ValidatedBy, got empty")
	}
}

func TestExtractStructured_DeadlinePattern(t *testing.T) {
	e := newTestExtractor(t)
	content := "Deadline: end of Q4 sprint."
	v := &entities.Verbatim{Content: content}
	tokens := e.tokenize(content)
	ents := e.extractEntities(tokens, content)
	data := e.extractStructured(v, tokens, ents, valueobjects.TypeFact)
	if data.Deadline == "" {
		t.Errorf("expected non-empty Deadline, got empty")
	}
}

func TestExtractStructured_SubjectPatternAndEntityFallback(t *testing.T) {
	e := newTestExtractor(t)

	// Subject via pattern
	content := "Subject: database migration plan."
	v := &entities.Verbatim{Content: content}
	tokens := e.tokenize(content)
	ents := e.extractEntities(tokens, content)
	data := e.extractStructured(v, tokens, ents, valueobjects.TypeFact)
	if len(data.Subject) == 0 {
		t.Errorf("expected non-empty Subject from pattern, got empty")
	}

	// Subject via entity fallback (no pattern match, but entities present)
	content2 := "Microsoft released a new version."
	v2 := &entities.Verbatim{Content: content2}
	tokens2 := e.tokenize(content2)
	ents2 := e.extractEntities(tokens2, content2)
	data2 := e.extractStructured(v2, tokens2, ents2, valueobjects.TypeFact)
	if len(data2.Subject) == 0 {
		t.Errorf("expected non-empty Subject from entity fallback, got empty")
	}

	// Subject via type-based default (no pattern, no entities)
	v3 := &entities.Verbatim{Content: "Something happened."}
	tokens3 := e.tokenize("Something happened.")
	data3 := e.extractStructured(v3, tokens3, []string{}, valueobjects.TypeDecision)
	if len(data3.Subject) == 0 || data3.Subject[0] != "Decision" {
		t.Errorf("expected default Subject='Decision', got %v", data3.Subject)
	}
}
