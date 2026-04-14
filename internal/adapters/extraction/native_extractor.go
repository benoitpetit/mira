// Native extractor - Drop-in replacement for prose library
// Uses rule-based NER and tokenization without external dependencies
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
	"unicode"
	"unicode/utf8"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
	"github.com/pkoukk/tiktoken-go"
)

// NativeExtractor implements extraction using pure Go (no prose dependency)
type NativeExtractor struct {
	tokenizer       *tiktoken.Tiktoken
	embedder        ports.Embedder
	modelHash       string
	minEntityLength int

	// Patterns for structured extraction
	decisionPatterns   []*regexp.Regexp
	rejectionPatterns  []*regexp.Regexp
	reasonPatterns     []*regexp.Regexp
	assigneePatterns   []*regexp.Regexp
	validationPatterns []*regexp.Regexp
	deadlinePatterns   []*regexp.Regexp
	causalPatterns     []causalPattern
	preferencePattern  *regexp.Regexp
	factStatePattern   *regexp.Regexp
	factDataPattern    *regexp.Regexp
	subjectPatterns    []*regexp.Regexp

	// Gazetteers for NER
	commonFirstNames map[string]bool
	commonLastNames  map[string]bool
	organizations    map[string]bool
	locations        map[string]bool
}

// NativeExtractorOptions configures the extractor
type NativeExtractorOptions struct {
	ModelName       string
	MinEntityLength int
}

// NewNativeExtractor creates a new native extractor (prose replacement)
func NewNativeExtractor(embedder ports.Embedder, opts NativeExtractorOptions) (*NativeExtractor, error) {
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

	e := &NativeExtractor{
		tokenizer:       tok,
		embedder:        embedder,
		modelHash:       modelHash,
		minEntityLength: minEntityLen,
	}

	e.compilePatterns()
	e.loadGazetteers()
	return e, nil
}

func (e *NativeExtractor) compilePatterns() {
	// Same patterns as prose extractor
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

	e.causalPatterns = []causalPattern{
		// English patterns
		{valueobjects.RelTriggered, regexp.MustCompile(`(?i)\b(?:following|after|in response to|triggered by)\b`)},
		{valueobjects.RelBecause, regexp.MustCompile(`(?i)\b(?:because|since|due to|as a result of)\b`)},
		{valueobjects.RelContradicts, regexp.MustCompile(`(?i)\b(?:contradicts|conflicts with|is inconsistent with)\b`)},
		{valueobjects.RelUpdates, regexp.MustCompile(`(?i)\b(?:updates|replaces|supersedes)\b`)},
		{valueobjects.RelResolves, regexp.MustCompile(`(?i)\b(?:resolves|solves|fixes|addresses)\b`)},
		// French patterns (word boundaries \b don't work well with accented chars in RE2)
		{valueobjects.RelTriggered, regexp.MustCompile(`(?i)(?:^|\s)(?:suite à|à la suite de|consécutivement à|après)(?:$|\s|[.,;!?])`)},
		{valueobjects.RelBecause, regexp.MustCompile(`(?i)(?:^|\s)(?:parce que|car|en raison de|grâce à)(?:$|\s|[.,;!?])`)},
		{valueobjects.RelContradicts, regexp.MustCompile(`(?i)(?:^|\s)(?:contredit|est incompatible avec|s'oppose à)(?:$|\s|[.,;!?])`)},
		{valueobjects.RelUpdates, regexp.MustCompile(`(?i)(?:^|\s)(?:met à jour|remplace|actualise)(?:$|\s|[.,;!?])`)},
		{valueobjects.RelResolves, regexp.MustCompile(`(?i)(?:^|\s)(?:résout|règle|corrige|solutionne)(?:$|\s|[.,;!?])`)},
	}

	e.preferencePattern = regexp.MustCompile(`(?i)(?:prefer|preference|like|dislike)`)
	e.factStatePattern = regexp.MustCompile(`(?i)(?:is|are|was|were|has|have|contains|equals|means|requires|supports|runs on|uses|costs|weighs|measures)\s+`)
	e.factDataPattern = regexp.MustCompile(`(?i)(?:\d{2,}|true|false|v\d+\.\d+|version\s+\d|port\s+\d|http[s]?://|[A-Z]{2,}\d+)`)

	e.subjectPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:subject|topic|about|regarding)\s*[:\s]+([\p{L}\p{N}\s.]+?)(?:\.|\n|$)`),
		regexp.MustCompile(`(?i)(?:migration|architecture|auth|api|db|database|frontend|backend|deploy)\s+(?:of\s+)?([\p{L}\p{N}\s]+?)(?:\.|\n|$)`),
	}
}

// loadGazetteers initializes entity recognition word lists
func (e *NativeExtractor) loadGazetteers() {
	// Common first names (English + French + International)
	e.commonFirstNames = map[string]bool{
		"John": true, "Jane": true, "Michael": true, "Sarah": true, "David": true, "Emma": true,
		"Jean": true, "Marie": true, "Pierre": true, "Sophie": true, "Thomas": true, "Julie": true,
		"Michel": true, "André": true, "Philippe": true, "Louis": true, "Alain": true,
		"Jacques": true, "Bernard": true, "Marcel": true, "René": true, "Alexandre": true,
		"François": true, "Nicolas": true, "Christophe": true, "Sébastien": true, "Isabelle": true,
		"Catherine": true, "Nathalie": true, "Anne": true, "Valérie": true, "Sandrine": true,
		"Stéphanie": true, "Véronique": true, "Alice": true, "Bob": true, "Charlie": true,
		"Diana": true, "Edward": true, "Fiona": true, "James": true, "Robert": true, "William": true,
		"Mary": true, "Patricia": true, "Jennifer": true, "Linda": true, "Elizabeth": true,
	}

	// Organizations (tech companies, common orgs)
	e.organizations = map[string]bool{
		"Microsoft": true, "Google": true, "Amazon": true, "Apple": true, "Facebook": true,
		"Meta": true, "Twitter": true, "X": true, "LinkedIn": true, "GitHub": true,
		"GitLab": true, "Bitbucket": true, "Docker": true, "Kubernetes": true, "AWS": true,
		"Azure": true, "GCP": true, "IBM": true, "Oracle": true, "Salesforce": true,
		"SAP": true, "Stripe": true, "Square": true, "PayPal": true, "MongoDB": true,
		"PostgreSQL": true, "MySQL": true, "Redis": true, "Elasticsearch": true,
		"React": true, "Vue": true, "Angular": true, "Node": true, "Python": true,
		"Go": true, "Rust": true, "Java": true, "Kotlin": true, "Swift": true,
		"Team": true, "Squad": true, "Department": true, "Company": true,
	}

	// Locations (countries, cities)
	e.locations = map[string]bool{
		"Paris": true, "London": true, "New": true, "York": true, "Tokyo": true, "Berlin": true,
		"Madrid": true, "Rome": true, "Moscow": true, "Beijing": true, "Sydney": true,
		"Toronto": true, "Chicago": true, "San": true, "Francisco": true, "Seattle": true,
		"Boston": true, "Austin": true, "Denver": true, "Miami": true, "Atlanta": true,
		"Barcelona": true, "Amsterdam": true, "Brussels": true, "Vienna": true, "Prague": true,
		"Warsaw": true, "Stockholm": true, "Oslo": true, "Helsinki": true, "Copenhagen": true,
		"Zurich": true, "Geneva": true, "Milan": true, "Venice": true, "Munich": true,
		"Frankfurt": true, "Hamburg": true, "Lyon": true, "Marseille": true, "Nice": true,
		"France": true, "USA": true, "US": true, "UK": true, "Germany": true, "Japan": true,
		"China": true, "India": true, "Brazil": true, "Canada": true, "Australia": true,
		"Spain": true, "Italy": true, "Netherlands": true, "Switzerland": true, "Belgium": true,
	}
}

// ExtractPipeline implements Extractor interface
func (e *NativeExtractor) ExtractPipeline(ctx context.Context, verbatim *entities.Verbatim, forcedType *valueobjects.MemoryType) (*entities.Fingerprint, *entities.Embedding, error) {
	// 1. Token count
	tokens := e.tokenizer.Encode(verbatim.Content, nil, nil)
	verbatim.TokenCount = len(tokens)

	// 2. Native tokenization + NER (replaces prose.NewDocument)
	tokensList := e.tokenize(verbatim.Content)

	// 3. Entity extraction (replaces prose doc.Tokens with NER labels)
	extractedEntities := e.extractEntities(tokensList, verbatim.Content)

	// 4. Type detection and structured extraction
	var memType valueobjects.MemoryType
	if forcedType != nil && forcedType.IsValid() {
		memType = *forcedType
	} else {
		memType = e.detectType(verbatim.Content)
	}
	data := e.extractStructured(verbatim, tokensList, extractedEntities, memType)

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

// Token represents a token with its properties
type Token struct {
	Text  string
	Label string // PERSON, ORG, GPE, or "" for other
}

// tokenize splits text into tokens (replacement for prose tokenization)
func (e *NativeExtractor) tokenize(text string) []Token {
	var tokens []Token
	words := strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || r == ',' || r == '.' || r == ';' || r == ':' || r == '!' || r == '?'
	})

	for _, word := range words {
		word = strings.TrimSpace(word)
		if word == "" {
			continue
		}
		// Clean punctuation
		word = strings.TrimFunc(word, func(r rune) bool {
			return unicode.IsPunct(r) || r == '"' || r == '\'' || r == '(' || r == ')'
		})
		if len(word) >= e.minEntityLength {
			tokens = append(tokens, Token{Text: word})
		}
	}
	return tokens
}

// extractEntities performs rule-based NER (replacement for prose NER)
func (e *NativeExtractor) extractEntities(tokens []Token, fullText string) []string {
	entitySet := make(map[string]bool)

	// Process tokens
	for i, tok := range tokens {
		word := tok.Text
		upperCount := 0
		for _, r := range word {
			if unicode.IsUpper(r) {
				upperCount++
			}
		}

		// Check if it looks like a proper noun (starts with uppercase, not all uppercase)
		if len(word) > 0 && unicode.IsUpper(rune(word[0])) && upperCount < len(word) {
			// Check against gazetteers
			if e.commonFirstNames[word] || e.organizations[word] || e.locations[word] {
				entitySet[word] = true
				continue
			}

			// Check for multi-word entities (look ahead)
			if i+1 < len(tokens) {
				nextWord := tokens[i+1].Text
				combined := word + " " + nextWord
				if e.organizations[combined] || e.locations[combined] || e.locations[nextWord] {
					entitySet[combined] = true
					continue
				}
			}

			// Heuristic: if previous word is not end of sentence, likely a name
			if i > 0 {
				prev := tokens[i-1].Text
				if len(prev) > 0 && !strings.Contains(".!?", string(prev[len(prev)-1])) {
					// Check if it's not a common word
					if !isCommonWord(word) {
						entitySet[word] = true
					}
				}
			}
		}
	}

	// Also extract emails and URLs
	emailPattern := regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	urlPattern := regexp.MustCompile(`https?://[^\s]+`)

	emails := emailPattern.FindAllString(fullText, -1)
	urls := urlPattern.FindAllString(fullText, -1)

	for _, e := range emails {
		entitySet[e] = true
	}
	for _, u := range urls {
		entitySet[u] = true
	}

	extractedEntities := make([]string, 0, len(entitySet))
	for entity := range entitySet {
		if utf8.RuneCountInString(entity) >= e.minEntityLength {
			extractedEntities = append(extractedEntities, entity)
		}
	}
	return extractedEntities
}

// isCommonWord checks if a word is a common word (not a proper noun)
func isCommonWord(word string) bool {
	common := map[string]bool{
		"The": true, "A": true, "An": true, "This": true, "That": true,
		"In": true, "On": true, "At": true, "For": true, "With": true,
		"To": true, "From": true, "By": true, "Of": true, "And": true,
		"Or": true, "But": true, "If": true, "Then": true, "When": true,
		"Where": true, "Why": true, "How": true, "What": true, "Who": true,
		"We": true, "They": true, "It": true, "He": true, "She": true,
		"I": true, "You": true, "My": true, "Our": true, "His": true,
		"Her": true, "Its": true, "Their": true, "Not": true, "No": true,
		"Yes": true, "So": true, "As": true, "Be": true, "Is": true,
		"Are": true, "Was": true, "Were": true, "Been": true, "Being": true,
		"Have": true, "Has": true, "Had": true, "Do": true, "Does": true,
		"Did": true, "Will": true, "Would": true, "Could": true, "Should": true,
		"May": true, "Might": true, "Can": true, "Must": true, "Shall": true,
		"About": true, "Above": true, "After": true, "Before": true, "During": true,
		"Between": true, "Under": true, "Over": true, "Into": true, "Through": true,
		"Within": true, "Without": true, "Against": true, "Among": true,
	}
	return common[word]
}

func (e *NativeExtractor) detectType(content string) valueobjects.MemoryType {
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

func (e *NativeExtractor) extractStructured(v *entities.Verbatim, tokens []Token, entities []string, memType valueobjects.MemoryType) valueobjects.FingerprintData {
	content := v.Content

	data := valueobjects.FingerprintData{
		ID:          v.ID.String(),
		Type:        string(memType),
		Date:        v.CreatedAt.Format(time.RFC3339),
		Entities:    entities,
		VerbatimRef: "T0:" + v.ID.String(),
		Custom:      make(map[string]any),
	}

	// Pattern extraction (same as prose extractor)
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
			data.Assignee = strings.TrimSpace(matches[1])
			break
		}
	}

	for _, pattern := range e.validationPatterns {
		if matches := pattern.FindStringSubmatch(content); matches != nil {
			data.ValidatedBy = strings.TrimSpace(matches[1])
			break
		}
	}

	for _, pattern := range e.deadlinePatterns {
		if matches := pattern.FindStringSubmatch(content); matches != nil {
			data.Deadline = strings.TrimSpace(matches[1])
			break
		}
	}

	// Subject inference
	data.Subject = e.inferSubject(content, entities, memType)

	return data
}

func (e *NativeExtractor) inferSubject(content string, entities []string, memType valueobjects.MemoryType) []string {
	var subjects []string

	// Try pattern-based extraction first
	for _, pattern := range e.subjectPatterns {
		if matches := pattern.FindStringSubmatch(content); matches != nil {
			subjects = append(subjects, strings.TrimSpace(matches[1]))
			if len(subjects) >= 3 {
				return subjects
			}
		}
	}

	// Use entities as fallback
	if len(entities) > 0 {
		for _, entity := range entities {
			if len(subjects) < 3 {
				subjects = append(subjects, entity)
			}
		}
	}

	// Default to type-based subject
	if len(subjects) == 0 {
		switch memType {
		case valueobjects.TypeDecision:
			subjects = []string{"Decision"}
		case valueobjects.TypeFact:
			subjects = []string{"Fact"}
		case valueobjects.TypePreference:
			subjects = []string{"Preference"}
		default:
			subjects = []string{"Note"}
		}
	}

	return subjects
}

func (e *NativeExtractor) normalizeL2(vec []float32) []float32 {
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

// ModelHash returns the model hash
func (e *NativeExtractor) ModelHash() string {
	return e.modelHash
}

// Encode implements Embedder interface (delegates to internal embedder)
func (e *NativeExtractor) Encode(ctx context.Context, text string) ([]float32, error) {
	return e.embedder.Encode(ctx, text)
}

// DetectCausalRelations implements CausalRelationDetector
// causalPattern associates a regex with its relation type
type causalPattern struct {
	relType valueobjects.RelationType
	pattern *regexp.Regexp
}

func (e *NativeExtractor) DetectCausalRelations(ctx context.Context, newFp *entities.Fingerprint, recentFps []*entities.Fingerprint, verbatimContent string) ([]*entities.CausalEdge, error) {
	var edges []*entities.CausalEdge
	contentLower := strings.ToLower(verbatimContent)
	seen := make(map[uuid.UUID]bool)

	for _, cp := range e.causalPatterns {
		if cp.pattern.MatchString(contentLower) {
			// Find first recent fingerprint not already linked
			for _, recentFp := range recentFps {
				if recentFp != nil && !seen[recentFp.ID] {
					edge := entities.NewCausalEdge(recentFp.ID, newFp.ID, cp.relType)
					edges = append(edges, edge)
					seen[recentFp.ID] = true
					break
				}
			}
		}
	}

	return edges, nil
}

// Ensure NativeExtractor implements the Extractor interface
var _ ports.Extractor = (*NativeExtractor)(nil)
