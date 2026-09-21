package extraction

import "github.com/benoitpetit/mira/internal/domain/valueobjects"

// attachExtractionQuality records how a fingerprint was produced without
// changing the stable T1 schema. The confidence is deliberately heuristic:
// it is a ranking signal and an audit hint, not a claim of truth.
func attachExtractionQuality(data *valueobjects.FingerprintData, backend, model string, confidence float64) {
	if data.Custom == nil {
		data.Custom = make(map[string]any)
	}
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}
	data.Custom["extraction_backend"] = backend
	data.Custom["extraction_model"] = model
	data.Custom["extraction_confidence"] = confidence
	data.Custom["extraction_provenance"] = map[string]any{
		"backend": backend,
		"model":   model,
		"kind":    "heuristic",
	}
}

func nativeExtractionConfidence(data valueobjects.FingerprintData) float64 {
	confidence := 0.35
	if data.Type != "" {
		confidence += 0.15
	}
	if len(data.Entities) > 0 {
		confidence += 0.10
	}
	if len(data.Subject) > 0 {
		confidence += 0.10
	}
	if data.Decision != "" || len(data.Rejected) > 0 || len(data.Reason) > 0 {
		confidence += 0.15
	}
	if data.Assignee != "" || data.ValidatedBy != "" || data.Deadline != "" {
		confidence += 0.10
	}
	if data.Negated {
		confidence += 0.05
	}
	if confidence > 0.95 {
		return 0.95
	}
	return confidence
}
