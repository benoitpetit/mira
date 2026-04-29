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
