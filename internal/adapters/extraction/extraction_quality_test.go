package extraction

import (
	"testing"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
)

func TestAttachExtractionQualityRecordsBoundedProvenance(t *testing.T) {
	data := valueobjects.FingerprintData{}
	attachExtractionQuality(&data, "native", "model-hash", 1.4)

	if got := data.Custom["extraction_backend"]; got != "native" {
		t.Fatalf("backend = %v, want native", got)
	}
	if got := data.Custom["extraction_model"]; got != "model-hash" {
		t.Fatalf("model = %v, want model-hash", got)
	}
	if got := data.Custom["extraction_confidence"]; got != 1.0 {
		t.Fatalf("confidence = %v, want 1", got)
	}
	provenance, ok := data.Custom["extraction_provenance"].(map[string]any)
	if !ok || provenance["kind"] != "heuristic" {
		t.Fatalf("unexpected provenance: %#v", data.Custom["extraction_provenance"])
	}
}

func TestNativeExtractionConfidenceReflectsStructuredSignals(t *testing.T) {
	plain := nativeExtractionConfidence(valueobjects.FingerprintData{Type: "fact"})
	rich := nativeExtractionConfidence(valueobjects.FingerprintData{
		Type:     "decision",
		Entities: []string{"MIRA"},
		Subject:  []string{"retrieval"},
		Decision: "use hybrid search",
		Reason:   []string{"better recall"},
	})

	if rich <= plain {
		t.Fatalf("rich confidence %.2f should exceed plain confidence %.2f", rich, plain)
	}
	if rich > 1 || plain < 0 {
		t.Fatalf("confidence out of bounds: plain=%.2f rich=%.2f", plain, rich)
	}
}
