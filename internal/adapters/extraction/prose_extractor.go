// Prose-based extractor adapter
package extraction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/jdkato/prose/v2"
	"github.com/pkoukk/tiktoken-go"
)

// ProseExtractor implements extraction using prose NLP
type ProseExtractor struct {
	tokenizer       *tiktoken.Tiktoken
	embedder        ports.Embedder
	modelHash       string
	minEntityLength int

	// Patterns
	decisionPatterns   []*regexp.Regexp
	rejectionPatterns  []*regexp.Regexp
	reasonPatterns     []*regexp.Regexp
	assigneePatterns   []*regexp.Regexp
	validationPatterns []*regexp.Regexp
	deadlinePatterns   []*regexp.Regexp
	causalPatterns     map[valueobjects.RelationType]*regexp.Regexp
	preferencePattern  *regexp.Regexp
	factStatePattern   *regexp.Regexp
	factDataPattern    *regexp.Regexp
	subjectPatterns    []*regexp.Regexp
}

// ProseExtractorOptions configures the extractor
type ProseExtractorOptions struct {
	ModelName       string
	MinEntityLength int
}

// NewProseExtractor creates a new prose-based extractor
func NewProseExtractor(embedder ports.Embedder, opts ProseExtractorOptions) (*ProseExtractor, error) {
	tok, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return nil, err
	}

	minEntityLen := opts.MinEntityLength
	if minEntityLen <= 0 {
		minEntityLen = 2
	}

	// Compute model hash
	hash := sha256.Sum256([]byte(opts.ModelName))
	modelHash := hex.EncodeToString(hash[:8])

	e := &ProseExtractor{
		tokenizer:       tok,
		embedder:        embedder,
		modelHash:       modelHash,
		minEntityLength: minEntityLen,
	}

	e.compilePatterns()
	return e, nil
}

func (e *ProseExtractor) compilePatterns() {
	e.decisionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)([\p{L}\p{N}]+)\s+(?:decided|chose|selected|opted|recommended)\s+(?:to\s+)?(?:use|adopt|migrate to|take)\s+([\p{L}\p{N}\s-]+)`),
		regexp.MustCompile(`(?i)(?:decision|choice)\s*:\s*(.+?)(?:\.|\n|$)`),
		regexp.MustCompile(`(?i)(?:we|I|team)\s+(?:will|shall|are going to)\s+(?:use|take|adopt)\s+([\p{L}\p{N}\s-]+)`),
	}

	e.rejectionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:rather than|instead of)\s+([\p{L}\p{N}\s,]+?)(?:\.|,|\s+and\s+|\s+or\s|$)`),
		regexp.MustCompile(`(?i)(?:rejected|discarded|excluded)\s*:\s*([\p{L}\p{N}\s,]+?)(?:\.|\n|$)`),
	}

	e.reasonPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:because|since|as)\s+(.+?)(?:\.|\n|$)`),
		regexp.MustCompile(`(?i)(?:reason|rationale|justification)\s*:\s*(.+?)(?:\.|\n|$)`),
	}

	e.assigneePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)([\p{L}\p{N}]+)\s+(?:will take care of|will (?:implement|do)|is\s+(?:assigned|responsible))`),
		regexp.MustCompile(`(?i)(?:assigned to)\s*[:\s]+([\p{L}\p{N}]+)`),
	}

	e.validationPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)([\p{L}\p{N}]+)\s+(?:validated|approved|signed off)`),
	}

	e.deadlinePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:deadline|due date)\s*[:\s]+([\p{L}\p{N}\s-]+?)(?:\.|\n|$)`),
		regexp.MustCompile(`(?i)(?:sprint|S)\s*(\d+)`),
		regexp.MustCompile(`(?i)(?:before|by)\s+(\d{1,2}[/-]\d{1,2}[/-]\d{2,4})`),
	}

	e.causalPatterns = map[valueobjects.RelationType]*regexp.Regexp{
		valueobjects.RelTriggered:   regexp.MustCompile(`(?i)(?:following|after|in response to)`),
		valueobjects.RelBecause:     regexp.MustCompile(`(?i)(?:because|since|due to|in reason of)`),
		valueobjects.RelContradicts: regexp.MustCompile(`(?i)(?:contradicts|in contradiction|however)`),
		valueobjects.RelUpdates:     regexp.MustCompile(`(?i)(?:updates|replaces)`),
		valueobjects.RelResolves:    regexp.MustCompile(`(?i)(?:resolves|solves|fixes)`),
	}

	e.preferencePattern = regexp.MustCompile(`(?i)(?:prefer|preference|like|dislike)`)
	e.factStatePattern = regexp.MustCompile(`(?i)(?:is|are|was|were|has|have|contains|equals|means|requires|supports|runs on|uses|costs|weighs|measures)\s+`)
	e.factDataPattern = regexp.MustCompile(`(?i)(?:\d{2,}|true|false|v\d+\.\d+|version\s+\d|port\s+\d|http[s]?://|[A-Z]{2,}\d+)`)

	e.subjectPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:subject|topic|about|regarding)\s*[:\s]+([\p{L}\p{N}\s.]+?)(?:\.|\n|$)`),
		regexp.MustCompile(`(?i)(?:migration|architecture|auth|api|db|database|frontend|backend|deploy)\s+(?:of\s+)?([\p{L}\p{N}\s]+?)(?:\.|\n|$)`),
	}
}

// ExtractPipeline implements Extractor
func (e *ProseExtractor) ExtractPipeline(ctx context.Context, verbatim *entities.Verbatim, forcedType *valueobjects.MemoryType) (*entities.Fingerprint, *entities.Embedding, error) {
	// 1. Token count
	tokens := e.tokenizer.Encode(verbatim.Content, nil, nil)
	verbatim.TokenCount = len(tokens)

	// 2. NER + segmentation
	doc, err := prose.NewDocument(verbatim.Content)
	if err != nil {
		return nil, nil, err
	}

	// 3. Entity extraction
	extractedEntities := e.extractEntities(doc)

	// 4. Type detection and structured extraction
	var memType valueobjects.MemoryType
	if forcedType != nil && forcedType.IsValid() {
		memType = *forcedType
	} else {
		memType = e.detectType(verbatim.Content)
	}
	data := e.extractStructured(verbatim, doc, extractedEntities, memType)

	// 5. Embedding generation
	vec, err := e.embedder.Encode(ctx, verbatim.Content)
	if err != nil {
		return nil, nil, err
	}
	vec = e.normalizeL2(vec)

	// 6. T1 token estimate
	fpJSON, _ := json.Marshal(data)
	t1Tokens := len(e.tokenizer.Encode(string(fpJSON), nil, nil))

	fp := entities.NewFingerprint(verbatim.ID, memType, e.modelHash)
	fp.WithData(data).WithTokenEstimate(t1Tokens)
	fp.CalculateFactCount()
	fp.Entities = extractedEntities
	fp.Subjects = data.Subject

	embedding := entities.NewEmbedding(verbatim.ID, e.modelHash, vec)
	embedding.Normalized = true

	return fp, embedding, nil
}

func (e *ProseExtractor) extractEntities(doc *prose.Document) []string {
	entitySet := make(map[string]bool)

	for _, tok := range doc.Tokens() {
		if tok.Label == "PERSON" || tok.Label == "ORG" || tok.Label == "GPE" {
			name := strings.TrimSpace(tok.Text)
			if utf8.RuneCountInString(name) >= e.minEntityLength {
				entitySet[name] = true
			}
		}
	}

	extractedEntities := make([]string, 0, len(entitySet))
	for e := range entitySet {
		extractedEntities = append(extractedEntities, e)
	}
	return extractedEntities
}

func (e *ProseExtractor) detectType(content string) valueobjects.MemoryType {
	contentLower := strings.ToLower(content)

	// Priority: decision > preference > fact > session_note
	for _, pattern := range e.decisionPatterns {
		if pattern.MatchString(contentLower) {
			return valueobjects.TypeDecision
		}
	}

	if e.preferencePattern.MatchString(contentLower) {
		return valueobjects.TypePreference
	}

	if e.factStatePattern.MatchString(contentLower) && e.factDataPattern.MatchString(contentLower) {
		return valueobjects.TypeFact
	}

	return valueobjects.TypeSessionNote
}

func (e *ProseExtractor) extractStructured(v *entities.Verbatim, doc *prose.Document, entities []string, memType valueobjects.MemoryType) valueobjects.FingerprintData {
	content := v.Content

	data := valueobjects.FingerprintData{
		ID:          v.ID.String(),
		Type:        string(memType),
		Date:        v.CreatedAt.Format(time.RFC3339),
		Entities:    entities,
		VerbatimRef: "T0:" + v.ID.String(),
		Custom:      make(map[string]any),
	}

	// Pattern extraction
	for _, pattern := range e.decisionPatterns {
		if matches := pattern.FindStringSubmatch(content); matches != nil {
			if len(matches) > 1 {
				data.Decision = strings.TrimSpace(matches[len(matches)-1])
				break
			}
		}
	}

	for _, pattern := range e.rejectionPatterns {
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

	for _, pattern := range e.reasonPatterns {
		if matches := pattern.FindStringSubmatch(content); matches != nil {
			data.Reason = append(data.Reason, strings.TrimSpace(matches[1]))
		}
	}

	for _, pattern := range e.assigneePatterns {
		if matches := pattern.FindStringSubmatch(content); matches != nil {
			data.Assignee = matches[1]
			break
		}
	}

	for _, pattern := range e.validationPatterns {
		if matches := pattern.FindStringSubmatch(content); matches != nil {
			data.ValidatedBy = matches[1]
			break
		}
	}

	for _, pattern := range e.deadlinePatterns {
		if matches := pattern.FindStringSubmatch(content); matches != nil {
			data.Deadline = strings.TrimSpace(matches[1])
			break
		}
	}

	data.Subject = e.inferSubject(content, entities)

	return data
}

func (e *ProseExtractor) inferSubject(content string, entities []string) []string {
	for _, pattern := range e.subjectPatterns {
		if matches := pattern.FindStringSubmatch(content); matches != nil {
			return []string{strings.TrimSpace(matches[1])}
		}
	}

	if len(entities) > 0 {
		return []string{entities[0]}
	}

	return []string{"general"}
}

// Encode implements Extractor
func (e *ProseExtractor) Encode(ctx context.Context, text string) ([]float32, error) {
	return e.embedder.Encode(ctx, text)
}

// ModelHash implements Extractor
func (e *ProseExtractor) ModelHash() string {
	return e.modelHash
}

// DetectCausalRelations implements Extractor
func (e *ProseExtractor) DetectCausalRelations(ctx context.Context, newFp *entities.Fingerprint, recentFps []*entities.Fingerprint, verbatimContent string) ([]*entities.CausalEdge, error) {
	var edges []*entities.CausalEdge

	for _, existing := range recentFps {
		if existing.ID == newFp.ID {
			continue
		}

		// Temporal constraint: cause must precede effect
		// existing is the potential cause, newFp is the effect
		if existing.ExtractedAt.After(newFp.ExtractedAt) {
			continue
		}

		// Check temporal proximity (within 30 days)
		timeDiff := newFp.ExtractedAt.Sub(existing.ExtractedAt).Hours() / 24
		if timeDiff > 30 {
			continue
		}

		// Check causal patterns
		for relType, pattern := range e.causalPatterns {
			if pattern.MatchString(verbatimContent) {
				if e.hasOverlap(existing.Entities, newFp.Entities) ||
					e.hasOverlap(existing.Subjects, newFp.Subjects) ||
					strings.Contains(verbatimContent, existing.ID.String()[:8]) {

					edge := entities.NewCausalEdge(existing.ID, newFp.ID, relType)
					edges = append(edges, edge)
				}
			}
		}
	}

	return edges, nil
}

func (e *ProseExtractor) hasOverlap(a, b []string) bool {
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

func (e *ProseExtractor) normalizeL2(v []float32) []float32 {
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

// Ensure ProseExtractor implements the interfaces
var _ ports.Extractor = (*ProseExtractor)(nil)
var _ ports.FingerprintExtractor = (*ProseExtractor)(nil)
var _ ports.CausalRelationDetector = (*ProseExtractor)(nil)
