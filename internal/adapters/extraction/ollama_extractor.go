// OllamaExtractor — LLM-backed fingerprint extractor using a local Ollama instance.
// Falls back to NativeExtractor on any error when FallbackOnError is true (default).
package extraction

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/benoitpetit/mira/internal/util"
	"github.com/google/uuid"
	tiktoken "github.com/pkoukk/tiktoken-go"
)

// OllamaExtractor calls a local Ollama instance to extract structured fingerprints.
// It satisfies ports.FingerprintExtractor and ports.CausalRelationDetector by delegating
// causal detection to the wrapped NativeExtractor.
type OllamaExtractor struct {
	native    *NativeExtractor // fallback + causal detection + model hash
	embedder  ports.Embedder
	modelHash string
	tokenizer *tiktoken.Tiktoken

	endpoint        string
	model           string
	client          *http.Client
	fallbackOnError bool
}

// OllamaExtractorOptions configures the Ollama extractor.
type OllamaExtractorOptions struct {
	// Ollama server endpoint — default: "http://localhost:11434"
	Endpoint string
	// Model name — default: "llama3.2:3b"
	Model string
	// HTTP timeout for each Ollama call — default: 30s
	Timeout time.Duration
	// FallbackOnError enables silent fallback to NativeExtractor on any Ollama failure.
	// Recommended: true (default).
	FallbackOnError bool
	// NativeOptions are passed through to the wrapped NativeExtractor.
	NativeOptions NativeExtractorOptions
}

// NewOllamaExtractor creates an OllamaExtractor wrapping a NativeExtractor.
// If Endpoint is empty, defaults to "http://localhost:11434".
// If Model is empty, defaults to "llama3.2:3b".
func NewOllamaExtractor(embedder ports.Embedder, opts OllamaExtractorOptions) (*OllamaExtractor, error) {
	if opts.Endpoint == "" {
		opts.Endpoint = "http://localhost:11434"
	}
	if opts.Model == "" {
		opts.Model = "llama3.2:3b"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}

	native, err := NewNativeExtractor(embedder, opts.NativeOptions)
	if err != nil {
		return nil, fmt.Errorf("ollama extractor: init native fallback: %w", err)
	}

	tok, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return nil, fmt.Errorf("ollama extractor: tiktoken: %w", err)
	}

	return &OllamaExtractor{
		native:          native,
		embedder:        embedder,
		modelHash:       util.ComputeModelHash(opts.NativeOptions.ModelName),
		tokenizer:       tok,
		endpoint:        strings.TrimRight(opts.Endpoint, "/"),
		model:           opts.Model,
		client:          &http.Client{Timeout: opts.Timeout},
		fallbackOnError: opts.FallbackOnError,
	}, nil
}

// ModelHash returns the embedding model hash (same as the native extractor).
func (o *OllamaExtractor) ModelHash() string { return o.modelHash }

// Encode delegates to the underlying embedder (satisfies ports.Embedder).
func (o *OllamaExtractor) Encode(ctx context.Context, text string) ([]float32, error) {
	return o.embedder.Encode(ctx, text)
}

// DetectCausalRelations delegates to the native extractor.
func (o *OllamaExtractor) DetectCausalRelations(ctx context.Context, newFp *entities.Fingerprint, recentFps []*entities.Fingerprint, verbatimContent string) ([]*entities.CausalEdge, error) {
	return o.native.DetectCausalRelations(ctx, newFp, recentFps, verbatimContent)
}

// Summarize generates an abstractive synthesis using Ollama.
func (o *OllamaExtractor) Summarize(ctx context.Context, texts []string) (string, error) {
	if len(texts) == 0 {
		return "", nil
	}
	if len(texts) == 1 {
		return texts[0], nil
	}

	reqBody := ollamaRequest{
		Model:  o.model,
		Prompt: summarizationPrompt(texts),
		Stream: false,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.endpoint+"/api/generate", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		if o.fallbackOnError {
			return o.native.Summarize(ctx, texts)
		}
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if o.fallbackOnError {
			return o.native.Summarize(ctx, texts)
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("ollama HTTP %d: %s", resp.StatusCode, string(body))
	}

	var ollamaResp ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", err
	}

	return strings.TrimSpace(ollamaResp.Response), nil
}

// summarizationPrompt builds the prompt for abstractive synthesis.
func summarizationPrompt(texts []string) string {
	content := strings.Join(texts, "\n---\n")
	return `You are a high-level information synthesizer. You will be provided with several related memories (session notes, logs, or fragments). Your goal is to synthesize them into a single, cohesive, and abstractive fact.

Rules:
1. Distill the core information. Remove redundancy and noise.
2. Be concise but maintain all significant details (names, dates, decisions).
3. The output must be a single paragraph of natural language.
4. Do NOT say "Here is a summary" or "The following notes describe". Start directly with the synthesized content.

Memories to synthesize:
` + content + `

Synthesized Fact:`
}

// ExtractPipeline attempts Ollama extraction and falls back to NativeExtractor on failure.
func (o *OllamaExtractor) ExtractPipeline(ctx context.Context, verbatim *entities.Verbatim, forcedType *valueobjects.MemoryType) (*entities.Fingerprint, *entities.Embedding, error) {
	// Always set token count (independent of extraction path)
	tokens := o.tokenizer.Encode(verbatim.Content, nil, nil)
	verbatim.TokenCount = len(tokens)

	fp, emb, err := o.extractViaOllama(ctx, verbatim, forcedType)
	if err != nil {
		if o.fallbackOnError {
			return o.native.ExtractPipeline(ctx, verbatim, forcedType)
		}
		return nil, nil, fmt.Errorf("ollama extraction failed (fallback disabled): %w", err)
	}
	return fp, emb, nil
}

// ── Ollama HTTP call ──────────────────────────────────────────────────────────

// ollamaRequest is the Ollama /api/generate request body.
type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
	Format string `json:"format"`
}

// ollamaResponse is the Ollama /api/generate response body (non-streaming).
type ollamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
	Error    string `json:"error,omitempty"`
}

// ollamaFingerprintResult is the JSON structure we ask Ollama to produce.
type ollamaFingerprintResult struct {
	Type     string   `json:"type"`
	Entities []string `json:"entities"`
	Subject  []string `json:"subject"`
	Decision string   `json:"decision,omitempty"`
	Rejected []string `json:"rejected,omitempty"`
	Reason   []string `json:"reason,omitempty"`
	Assignee string   `json:"assignee,omitempty"`
	Deadline string   `json:"deadline,omitempty"`
	Negated  bool     `json:"negated,omitempty"`
}

// extractionPrompt builds the few-shot JSON extraction prompt.
func extractionPrompt(content string) string {
	return `You are a structured memory extraction system. Extract information from the text below.
Return ONLY a valid JSON object — no explanation, no markdown, no extra text.

JSON schema:
{
  "type": "decision|fact|preference|session_note|debug_log",
  "entities": ["list of named entities: people, systems, tools"],
  "subject": ["1-3 word topic labels"],
  "decision": "the main decision made (if any, else empty string)",
  "rejected": ["alternatives that were rejected"],
  "reason": ["reasons or rationale"],
  "assignee": "person responsible (if any, else empty)",
  "deadline": "deadline or date (if any, else empty)",
  "negated": false
}

Rules:
- "type" must be one of the five values above
- "entities" and "subject" are required arrays (can be empty [])
- All other fields are optional; omit or leave empty if not applicable
- "negated" is true only when the memory explicitly contradicts or negates something

Text:
` + content + `

JSON:`
}

func (o *OllamaExtractor) extractViaOllama(ctx context.Context, verbatim *entities.Verbatim, forcedType *valueobjects.MemoryType) (*entities.Fingerprint, *entities.Embedding, error) {
	reqBody := ollamaRequest{
		Model:  o.model,
		Prompt: extractionPrompt(verbatim.Content),
		Stream: false,
		Format: "json",
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.endpoint+"/api/generate", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("ollama call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, nil, fmt.Errorf("ollama HTTP %d: %s", resp.StatusCode, string(body))
	}

	var ollamaResp ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, nil, fmt.Errorf("decode ollama response: %w", err)
	}
	if ollamaResp.Error != "" {
		return nil, nil, fmt.Errorf("ollama error: %s", ollamaResp.Error)
	}

	// Parse the extracted JSON from the model response
	var extracted ollamaFingerprintResult
	raw := strings.TrimSpace(ollamaResp.Response)
	// Ollama with format:"json" should return pure JSON, but strip markdown fences defensively
	if idx := strings.Index(raw, "{"); idx > 0 {
		raw = raw[idx:]
	}
	if idx := strings.LastIndex(raw, "}"); idx >= 0 && idx < len(raw)-1 {
		raw = raw[:idx+1]
	}
	if err := json.Unmarshal([]byte(raw), &extracted); err != nil {
		return nil, nil, fmt.Errorf("parse extracted JSON %q: %w", raw, err)
	}

	// Resolve memory type
	memType := valueobjects.MemoryType(extracted.Type)
	if forcedType != nil && forcedType.IsValid() {
		memType = *forcedType
	} else if !memType.IsValid() {
		memType = valueobjects.TypeFact // safe default
	}

	// Build FingerprintData
	data := valueobjects.FingerprintData{
		ID:          uuid.New().String(),
		Type:        string(memType),
		Date:        verbatim.CreatedAt.Format("2006-01-02"),
		Entities:    deduplicateStrings(extracted.Entities),
		Subject:     deduplicateStrings(extracted.Subject),
		Decision:    extracted.Decision,
		Rejected:    extracted.Rejected,
		Reason:      extracted.Reason,
		Assignee:    extracted.Assignee,
		Deadline:    extracted.Deadline,
		VerbatimRef: verbatim.ID.String(),
		Negated:     extracted.Negated,
	}

	// Generate embedding
	vec, err := o.embedder.Encode(ctx, verbatim.Content)
	if err != nil {
		return nil, nil, fmt.Errorf("embedding: %w", err)
	}
	vec = normalizeL2Vector(vec)

	// T1 token estimate
	fpJSON, _ := json.Marshal(data)
	t1Tokens := len(o.tokenizer.Encode(string(fpJSON), nil, nil))

	fp := entities.NewFingerprint(verbatim.ID, memType, o.modelHash)
	fp.WithData(data).WithTokenEstimate(t1Tokens)
	fp.CalculateFactCount()
	fp.Entities = data.Entities
	fp.Subjects = data.Subject

	embedding := entities.NewEmbedding(verbatim.ID, o.modelHash, vec)
	embedding.Normalized = true

	return fp, embedding, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// deduplicateStrings returns a deduplicated copy preserving order.
func deduplicateStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// normalizeL2Vector normalises a float32 vector in-place (identical to NativeExtractor.normalizeL2).
func normalizeL2Vector(vec []float32) []float32 {
	var sum float64
	for _, v := range vec {
		sum += float64(v * v)
	}
	norm := math.Sqrt(sum)
	if norm == 0 {
		return vec
	}
	for i := range vec {
		vec[i] = float32(float64(vec[i]) / norm)
	}
	return vec
}
