package extract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jdkato/prose/v2"
	"github.com/pkoukk/tiktoken-go"
	"github.com/benoitpetit/mira/types"
)

const (
	MaxVerbatimSize   = 64 * 1024
	MaxSentenceLength = 500
	MinEntityLength   = 2
)

// UTF-8 aware regex patterns
var (
	decisionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)([\p{L}\p{N}]+)\s+(?:decided|chose|selected|opted|recommended)\s+(?:to\s+)?(?:use|adopt|migrate to|take)\s+([\p{L}\p{N}\s-]+)`),
		regexp.MustCompile(`(?i)(?:decision|choice)\s*:\s*(.+?)(?:\.|\n|$)`),
		regexp.MustCompile(`(?i)(?:we|I|team)\s+(?:will|shall|are going to)\s+(?:use|take|adopt)\s+([\p{L}\p{N}\s-]+)`),
	}

	rejectionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:rather than|instead of)\s+([\p{L}\p{N}\s,]+?)(?:\.|,|\s+and\s+|\s+or\s|$)`),
		regexp.MustCompile(`(?i)(?:rejected|discarded|excluded)\s*:\s*([\p{L}\p{N}\s,]+?)(?:\.|\n|$)`),
	}

	reasonPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:because|since|as)\s+(.+?)(?:\.|\n|$)`),
		regexp.MustCompile(`(?i)(?:reason|rationale|justification)\s*:\s*(.+?)(?:\.|\n|$)`),
	}

	assigneePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)([\p{L}\p{N}]+)\s+(?:will take care of|will (?:implement|do)|is\s+(?:assigned|responsible))`),
		regexp.MustCompile(`(?i)(?:assigned to)\s*[:\s]+([\p{L}\p{N}]+)`),
	}

	validationPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)([\p{L}\p{N}]+)\s+(?:validated|approved|signed off)`),
	}

	deadlinePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:deadline|due date)\s*[:\s]+([\p{L}\p{N}\s-]+?)(?:\.|\n|$)`),
		regexp.MustCompile(`(?i)(?:sprint|S)\s*(\d+)`),
		regexp.MustCompile(`(?i)(?:before|by)\s+(\d{1,2}[/-]\d{1,2}[/-]\d{2,4})`),
	}

	causalPatterns = map[types.RelationType]*regexp.Regexp{
		types.RelTriggered:   regexp.MustCompile(`(?i)(?:following|after|in response to)`),
		types.RelBecause:     regexp.MustCompile(`(?i)(?:because|since|due to|in reason of)`),
		types.RelContradicts: regexp.MustCompile(`(?i)(?:contradicts|in contradiction|however)`),
		types.RelUpdates:     regexp.MustCompile(`(?i)(?:updates|replaces)`),
		types.RelResolves:    regexp.MustCompile(`(?i)(?:resolves|solves|fixes)`),
	}
)

// Embedder interface for embeddings
type Embedder interface {
	Encode(text string) ([]float32, error)
}

// Extractor manages T0 → T1, T2 extraction
type Extractor struct {
	tokenizer *tiktoken.Tiktoken
	embedder  Embedder
	modelHash string
	modelName string
}

// NewExtractor creates a new extractor
func NewExtractor(modelName string, embedder Embedder) (*Extractor, error) {
	tok, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return nil, err
	}

	// Compute model hash for versioning
	hash := sha256.Sum256([]byte(modelName))
	modelHash := hex.EncodeToString(hash[:8]) // 16 chars

	return &Extractor{
		tokenizer: tok,
		embedder:  embedder,
		modelHash: modelHash,
		modelName: modelName,
	}, nil
}

// ExtractPipeline transforms T0 into T1 + T2
func (e *Extractor) ExtractPipeline(verbatim *types.Verbatim) (*types.Fingerprint, *types.Embedding, error) {
	// 1. Token count
	tokens := e.tokenizer.Encode(verbatim.Content, nil, nil)
	verbatim.TokenCount = len(tokens)

	// 2. NER + segmentation
	doc, err := prose.NewDocument(verbatim.Content)
	if err != nil {
		return nil, nil, err
	}

	// 3. Entity extraction (UTF-8 aware)
	entities := e.extractEntities(doc)

	// 4. Type detection and structured extraction
	ftype := e.detectType(verbatim.Content)
	data := e.extractStructured(verbatim, doc, entities, ftype)

	// 5. Embedding generation
	vec, err := e.embedder.Encode(verbatim.Content)
	if err != nil {
		return nil, nil, err
	}
	vec = normalizeL2(vec)

	// 6. T1 token estimate
	fpJSON, _ := json.Marshal(data)
	t1Tokens := len(e.tokenizer.Encode(string(fpJSON), nil, nil))

	fp := &types.Fingerprint{
		ID:            verbatim.ID,
		VerbatimID:    verbatim.ID,
		Type:          ftype,
		ExtractedAt:   time.Now(),
		Entities:      entities,
		Subjects:      data.Subject,
		Decision:      nil,
		Data:          data,
		FactCount:     e.countFacts(data),
		TokenEstimate: t1Tokens,
		ModelHash:     e.modelHash,
	}

	if data.Decision != "" {
		fp.Decision = &data.Decision
	}

	embedding := &types.Embedding{
		ID:         verbatim.ID,
		ModelHash:  e.modelHash,
		Dim:        len(vec),
		Vector:     vec,
		Normalized: true,
		CreatedAt:  time.Now(),
	}

	return fp, embedding, nil
}

// extractEntities with UTF-8 handling
func (e *Extractor) extractEntities(doc *prose.Document) []string {
	entitySet := make(map[string]bool)

	for _, tok := range doc.Tokens() {
		if tok.Label == "PERSON" || tok.Label == "ORG" || tok.Label == "GPE" {
			name := strings.TrimSpace(tok.Text)
			if utf8.RuneCountInString(name) >= MinEntityLength {
				entitySet[name] = true
			}
		}
	}

	entities := make([]string, 0, len(entitySet))
	for e := range entitySet {
		entities = append(entities, e)
	}
	return entities
}

// detectType with strict priority
func (e *Extractor) detectType(content string) types.MemoryType {
	contentLower := strings.ToLower(content)

	// Priority: decision > fact > preference > session_note
	for _, pattern := range decisionPatterns {
		if pattern.MatchString(contentLower) {
			return types.TypeDecision
		}
	}

	if regexp.MustCompile(`(?i)(?:prefer|preference|like|dislike)`).MatchString(contentLower) {
		return types.TypePreference
	}

	if regexp.MustCompile(`(?i)(?:is|are|was|were|has|have)\s+`).MatchString(contentLower) &&
		regexp.MustCompile(`\d{4}`).MatchString(contentLower) {
		return types.TypeFact
	}

	return types.TypeSessionNote
}

// extractStructured extracts structured data
func (e *Extractor) extractStructured(v *types.Verbatim, doc *prose.Document, entities []string, ftype types.MemoryType) types.FingerprintData {
	content := v.Content

	data := types.FingerprintData{
		ID:          v.ID.String(),
		Type:        string(ftype),
		Date:        v.CreatedAt.Format(time.RFC3339),
		Entities:    entities,
		VerbatimRef: "T0:" + v.ID.String(),
		Custom:      make(map[string]any),
	}

	// Pattern extraction...
	for _, pattern := range decisionPatterns {
		if matches := pattern.FindStringSubmatch(content); matches != nil {
			if len(matches) > 1 {
				data.Decision = strings.TrimSpace(matches[len(matches)-1])
				break
			}
		}
	}

	for _, pattern := range rejectionPatterns {
		if matches := pattern.FindStringSubmatch(content); matches != nil {
			rejected := strings.Split(matches[1], ",")
			for _, r := range rejected {
				r = strings.TrimSpace(r)
				if r != "" {
					data.Rejected = append(data.Rejected, r)
				}
			}
		}
	}

	for _, pattern := range reasonPatterns {
		if matches := pattern.FindStringSubmatch(content); matches != nil {
			data.Reason = append(data.Reason, strings.TrimSpace(matches[1]))
		}
	}

	for _, pattern := range assigneePatterns {
		if matches := pattern.FindStringSubmatch(content); matches != nil {
			data.Assignee = matches[1]
			break
		}
	}

	for _, pattern := range validationPatterns {
		if matches := pattern.FindStringSubmatch(content); matches != nil {
			data.ValidatedBy = matches[1]
			break
		}
	}

	for _, pattern := range deadlinePatterns {
		if matches := pattern.FindStringSubmatch(content); matches != nil {
			data.Deadline = strings.TrimSpace(matches[1])
			break
		}
	}

	data.Subject = e.inferSubject(content, entities)

	return data
}

func (e *Extractor) inferSubject(content string, entities []string) []string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:subject|topic|about|regarding)\s*[:\s]+([\p{L}\p{N}\s.]+?)(?:\.|\n|$)`),
		regexp.MustCompile(`(?i)(?:migration|architecture|auth|api|db|database|frontend|backend|deploy)\s+(?:of\s+)?([\p{L}\p{N}\s]+?)(?:\.|\n|$)`),
	}

	for _, pattern := range patterns {
		if matches := pattern.FindStringSubmatch(content); matches != nil {
			return []string{strings.TrimSpace(matches[1])}
		}
	}

	if len(entities) > 0 {
		return []string{entities[0]}
	}

	return []string{"general"}
}

func (e *Extractor) countFacts(data types.FingerprintData) int {
	count := 0
	if data.Decision != "" {
		count++
	}
	count += len(data.Rejected)
	count += len(data.Reason)
	if data.ValidatedBy != "" {
		count++
	}
	if data.Assignee != "" {
		count++
	}
	if data.Deadline != "" {
		count++
	}
	if len(data.Subject) > 0 && data.Subject[0] != "" {
		count++
	}
	return count
}

func normalizeL2(v []float32) []float32 {
	var norm float32
	for _, x := range v {
		norm += x * x
	}
	norm = float32(math.Sqrt(float64(norm)))

	if norm == 0 {
		return v
	}

	for i := range v {
		v[i] /= norm
	}
	return v
}

// DetectCausalRelations compares against N recent items from same wing
func (e *Extractor) DetectCausalRelations(newFp *types.Fingerprint, recentFps []*types.Fingerprint, verbatimContent string) ([]*types.CausalEdge, error) {
	var edges []*types.CausalEdge

	for _, existing := range recentFps {
		if existing.ID == newFp.ID {
			continue
		}

		// Check reasonable temporal overlap (not > 30 days apart for causality)
		timeDiff := math.Abs(float64(newFp.ExtractedAt.Sub(existing.ExtractedAt).Hours()) / 24)
		if timeDiff > 30 {
			continue
		}

		// Check causal patterns in new text referencing old one
		for relType, pattern := range causalPatterns {
			if pattern.MatchString(verbatimContent) {
				// Check if existing entities in newFp (implicit reference)
				if hasOverlap(existing.Entities, newFp.Entities) ||
					hasOverlap(existing.Subjects, newFp.Subjects) ||
					strings.Contains(verbatimContent, existing.ID.String()[:8]) {

					// Determine direction: if newFp mentions "following", then existing -> newFp
					edge := &types.CausalEdge{
						FromID:     existing.ID,
						ToID:       newFp.ID,
						Relation:   relType,
						Weight:     0.7,
						DetectedAt: time.Now(),
					}
					edges = append(edges, edge)
				}
			}
		}
	}

	return edges, nil
}

func hasOverlap(a, b []string) bool {
	set := make(map[string]bool)
	for _, x := range a {
		set[x] = true
	}
	for _, x := range b {
		if set[x] {
			return true
		}
	}
	return false
}

// ModelHash returns model hash
func (e *Extractor) ModelHash() string {
	return e.modelHash
}

// ModelName returns model name
func (e *Extractor) ModelName() string {
	return e.modelName
}

// Encode encodes text into embedding vector
func (e *Extractor) Encode(text string) ([]float32, error) {
	return e.embedder.Encode(text)
}

// SimpleEmbedder simple Embedder implementation for tests
type SimpleEmbedder struct {
	dim int
}

// NewSimpleEmbedder creates a new simple embedder
func NewSimpleEmbedder(dim int) *SimpleEmbedder {
	return &SimpleEmbedder{dim: dim}
}

// Encode generates pseudo-random embedding based on text
func (s *SimpleEmbedder) Encode(text string) ([]float32, error) {
	vec := make([]float32, s.dim)
	seed := hashString(text)
	for i := 0; i < s.dim; i++ {
		vec[i] = float32((seed+uint64(i)*6364136223846793005)&0xFFFFFFFF) / float32(0xFFFFFFFF)
	}
	return normalizeL2(vec), nil
}

func hashString(s string) uint64 {
	var h uint64 = 14695981039346656037
	for _, c := range s {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return h
}
