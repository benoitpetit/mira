package extraction

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

func newOllamaExtractor(t *testing.T, endpoint string) *OllamaExtractor {
	t.Helper()
	embedder := NewSimpleEmbedder(384)
	ext, err := NewOllamaExtractor(embedder, OllamaExtractorOptions{
		Endpoint:        endpoint,
		Model:           "test-model",
		Timeout:         5 * time.Second,
		FallbackOnError: false,
		NativeOptions:   NativeExtractorOptions{ModelName: "test-model"},
	})
	if err != nil {
		t.Fatalf("NewOllamaExtractor: %v", err)
	}
	return ext
}

func fakeOllamaServer(response string, statusCode int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(response))
	}))
}

func makeVerbatim(content string) *entities.Verbatim {
	wing := "test-wing"
	room := "test-room"
	v := entities.NewVerbatim(content, wing, &room)
	return v
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestOllamaExtractor_SuccessfulExtraction(t *testing.T) {
	extracted := ollamaFingerprintResult{
		Type:     "decision",
		Entities: []string{"Alice", "Bob"},
		Subject:  []string{"deployment"},
		Decision: "Deploy to production",
		Reason:   []string{"tests pass"},
	}
	respJSON, _ := json.Marshal(extracted)
	ollamaResp := ollamaResponse{Response: string(respJSON), Done: true}
	body, _ := json.Marshal(ollamaResp)

	srv := fakeOllamaServer(string(body), http.StatusOK)
	defer srv.Close()

	ext := newOllamaExtractor(t, srv.URL)
	verbatim := makeVerbatim("Deploy to production because tests pass")

	fp, emb, err := ext.ExtractPipeline(context.Background(), verbatim, nil)
	if err != nil {
		t.Fatalf("ExtractPipeline: %v", err)
	}
	if fp == nil || emb == nil {
		t.Fatal("expected non-nil fingerprint and embedding")
	}
	if fp.Type != valueobjects.TypeDecision {
		t.Errorf("Type = %q, want %q", fp.Type, valueobjects.TypeDecision)
	}
	if len(emb.Vector) != 384 {
		t.Errorf("embedding dim = %d, want 384", len(emb.Vector))
	}
}

func TestOllamaExtractor_ForcedType(t *testing.T) {
	extracted := ollamaFingerprintResult{
		Type:     "fact",
		Entities: []string{"system"},
		Subject:  []string{"test"},
	}
	respJSON, _ := json.Marshal(extracted)
	ollamaResp := ollamaResponse{Response: string(respJSON), Done: true}
	body, _ := json.Marshal(ollamaResp)

	srv := fakeOllamaServer(string(body), http.StatusOK)
	defer srv.Close()

	ext := newOllamaExtractor(t, srv.URL)
	verbatim := makeVerbatim("some preference")
	forced := valueobjects.TypePreference

	fp, _, err := ext.ExtractPipeline(context.Background(), verbatim, &forced)
	if err != nil {
		t.Fatalf("ExtractPipeline: %v", err)
	}
	if fp.Type != valueobjects.TypePreference {
		t.Errorf("Type = %q, want forced %q", fp.Type, valueobjects.TypePreference)
	}
}

func TestOllamaExtractor_InvalidType_DefaultsToFact(t *testing.T) {
	extracted := ollamaFingerprintResult{
		Type:     "garbage_type",
		Entities: []string{},
		Subject:  []string{},
	}
	respJSON, _ := json.Marshal(extracted)
	ollamaResp := ollamaResponse{Response: string(respJSON), Done: true}
	body, _ := json.Marshal(ollamaResp)

	srv := fakeOllamaServer(string(body), http.StatusOK)
	defer srv.Close()

	ext := newOllamaExtractor(t, srv.URL)
	fp, _, err := ext.ExtractPipeline(context.Background(), makeVerbatim("anything"), nil)
	if err != nil {
		t.Fatalf("ExtractPipeline: %v", err)
	}
	if fp.Type != valueobjects.TypeFact {
		t.Errorf("expected fallback to fact, got %q", fp.Type)
	}
}

func TestOllamaExtractor_HTTPError_NoFallback(t *testing.T) {
	srv := fakeOllamaServer(`{"error":"model not found"}`, http.StatusInternalServerError)
	defer srv.Close()

	ext := newOllamaExtractor(t, srv.URL)
	_, _, err := ext.ExtractPipeline(context.Background(), makeVerbatim("test"), nil)
	if err == nil {
		t.Error("expected error on HTTP 500")
	}
}

func TestOllamaExtractor_HTTPError_WithFallback(t *testing.T) {
	srv := fakeOllamaServer(`{"error":"fail"}`, http.StatusInternalServerError)
	defer srv.Close()

	embedder := NewSimpleEmbedder(384)
	ext, err := NewOllamaExtractor(embedder, OllamaExtractorOptions{
		Endpoint:        srv.URL,
		Model:           "test-model",
		Timeout:         5 * time.Second,
		FallbackOnError: true,
		NativeOptions:   NativeExtractorOptions{ModelName: "test-model"},
	})
	if err != nil {
		t.Fatal(err)
	}

	fp, emb, err := ext.ExtractPipeline(context.Background(), makeVerbatim("fallback test"), nil)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got: %v", err)
	}
	if fp == nil || emb == nil {
		t.Fatal("expected non-nil result from fallback")
	}
}

func TestOllamaExtractor_OllamaErrorField(t *testing.T) {
	// Ollama returns 200 with an error field set
	ollamaResp := ollamaResponse{Error: "something went wrong"}
	body, _ := json.Marshal(ollamaResp)

	srv := fakeOllamaServer(string(body), http.StatusOK)
	defer srv.Close()

	ext := newOllamaExtractor(t, srv.URL)
	_, _, err := ext.ExtractPipeline(context.Background(), makeVerbatim("test"), nil)
	if err == nil {
		t.Error("expected error when Ollama response contains error field")
	}
}

func TestOllamaExtractor_BadJSON(t *testing.T) {
	ollamaResp := ollamaResponse{Response: `not valid json {{{ `, Done: true}
	body, _ := json.Marshal(ollamaResp)

	srv := fakeOllamaServer(string(body), http.StatusOK)
	defer srv.Close()

	ext := newOllamaExtractor(t, srv.URL)
	_, _, err := ext.ExtractPipeline(context.Background(), makeVerbatim("test"), nil)
	if err == nil {
		t.Error("expected error on bad JSON in response")
	}
}

func TestOllamaExtractor_Encode(t *testing.T) {
	ext := newOllamaExtractor(t, "http://localhost:9")
	vec, err := ext.Encode(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(vec) != 384 {
		t.Errorf("Encode dim = %d, want 384", len(vec))
	}
}

func TestOllamaExtractor_ModelHash(t *testing.T) {
	ext := newOllamaExtractor(t, "http://localhost:9")
	h := ext.ModelHash()
	if len(h) != 16 {
		t.Errorf("ModelHash len = %d, want 16", len(h))
	}
}

func TestOllamaExtractor_DetectCausalRelations(t *testing.T) {
	ext := newOllamaExtractor(t, "http://localhost:9")
	edges, err := ext.DetectCausalRelations(context.Background(), &entities.Fingerprint{}, nil, "test content")
	if err != nil {
		t.Fatalf("DetectCausalRelations: %v", err)
	}
	_ = edges // may be nil or empty
}

// ── Unit tests for pure helpers ───────────────────────────────────────────────

func TestDeduplicateStrings(t *testing.T) {
	in := []string{"a", "b", "a", "  c  ", "", "b", "c"}
	got := deduplicateStrings(in)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("deduplicateStrings len = %d, want %d; got %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestNormalizeL2Vector(t *testing.T) {
	vec := []float32{3, 4} // norm = 5
	out := normalizeL2Vector(vec)
	// after normalisation: [0.6, 0.8]
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	// Verify norm is ~1.0
	var sum float64
	for _, v := range out {
		sum += float64(v * v)
	}
	norm := sum
	if norm < 0.99 || norm > 1.01 {
		t.Errorf("normalised vector norm = %f, want ~1.0", norm)
	}
}

func TestNormalizeL2Vector_ZeroVector(t *testing.T) {
	vec := []float32{0, 0, 0}
	out := normalizeL2Vector(vec)
	if len(out) != 3 {
		t.Fatal("length changed")
	}
	for _, v := range out {
		if v != 0 {
			t.Error("zero vector should remain zero")
		}
	}
}

func TestExtractionPrompt(t *testing.T) {
	p := extractionPrompt("hello world")
	if len(p) < 100 {
		t.Error("prompt seems too short")
	}
	// Must contain the input text
	if !containsStr(p, "hello world") {
		t.Error("prompt does not contain the input text")
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || sub == "" || findSub(s, sub))
}

func findSub(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
