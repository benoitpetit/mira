// OptimizeContext use case: stateless, deterministic pruning of a chat
// message array against a token budget. Unlike RecallMemory, it requires no
// vector store, embeddings, or persistence — it operates purely on the
// messages given by the caller (e.g. a conversation history file), which is
// what powers `mira optimize --file history.json` and the MIRA Proxy product.
package interactors

import (
	"sort"
	"strings"
)

// ContextMessage is a provider-agnostic chat message.
type ContextMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OptimizeContextInput is the input for OptimizeContext.
type OptimizeContextInput struct {
	Messages     []ContextMessage
	BudgetTokens int
	KeepLastN    int
}

// OptimizeContextOutput is the result of OptimizeContext.
type OptimizeContextOutput struct {
	Messages        []ContextMessage `json:"messages"`
	OriginalTokens  int              `json:"original_tokens"`
	OptimizedTokens int              `json:"optimized_tokens"`
	TokensSaved     int              `json:"tokens_saved"`
	Dropped         int              `json:"dropped"`
}

// ContextEfficiency measures how much explicitly required evidence survives a
// context reduction. It is intentionally evidence coverage, not model-answer
// accuracy: callers can reproduce it without an LLM or a subjective judge.
type ContextEfficiency struct {
	RequiredAssertions     int     `json:"required_assertions"`
	RetainedAssertions     int     `json:"retained_assertions"`
	CoveragePercent        float64 `json:"coverage_percent"`
	CoveragePer1KTokens    float64 `json:"coverage_per_1k_tokens"`
	OptimizedContextTokens int     `json:"optimized_context_tokens"`
}

const (
	// DefaultOptimizeBudgetTokens is used when the caller does not request a budget.
	DefaultOptimizeBudgetTokens = 4000
	// DefaultOptimizeKeepLastN is the number of most recent messages always kept verbatim.
	DefaultOptimizeKeepLastN = 4
)

// OptimizeContext runs the deterministic O(n log n) CBA-lite budget
// allocation used by MIRA Proxy: no LLM calls, no persistence, native Go.
type OptimizeContext struct{}

// NewOptimizeContext creates a new OptimizeContext use case.
func NewOptimizeContext() *OptimizeContext { return &OptimizeContext{} }

// estimateContextTokens is a fast, deterministic token estimate (~4 chars/token).
func estimateContextTokens(content string) int {
	const charsPerToken = 4
	n := (len(content) + charsPerToken - 1) / charsPerToken
	if n < 1 {
		n = 1
	}
	return n + 3 // small per-message overhead (role/formatting)
}

// Execute selects the subset of messages that maximizes recency+relevance
// within BudgetTokens. System messages and the last KeepLastN messages are
// always kept verbatim. Remaining messages are scored and greedily packed by
// descending score (sort.Slice => O(n log n)), then reassembled in their
// original chronological order.
func (uc *OptimizeContext) Execute(input OptimizeContextInput) OptimizeContextOutput {
	budgetTokens := input.BudgetTokens
	if budgetTokens <= 0 {
		budgetTokens = DefaultOptimizeBudgetTokens
	}
	keepLastN := input.KeepLastN
	if keepLastN <= 0 {
		keepLastN = DefaultOptimizeKeepLastN
	}

	messages := input.Messages
	n := len(messages)
	originalTokens := 0
	for _, m := range messages {
		originalTokens += estimateContextTokens(m.Content)
	}
	if n == 0 {
		return OptimizeContextOutput{Messages: messages}
	}

	lastUserWords := wordSetOf(lastUserContentOf(messages))

	alwaysKeep := make([]bool, n)
	keepFrom := n - keepLastN
	for i, m := range messages {
		if strings.EqualFold(m.Role, "system") || i >= keepFrom {
			alwaysKeep[i] = true
		}
	}

	type scored struct {
		idx    int
		score  float64
		tokens int
	}

	usedTokens := 0
	candidates := make([]scored, 0, n)
	for i, m := range messages {
		tok := estimateContextTokens(m.Content)
		if alwaysKeep[i] {
			usedTokens += tok
			continue
		}
		recency := float64(i+1) / float64(n)
		relevance := jaccardOf(wordSetOf(m.Content), lastUserWords)
		score := 0.6*relevance + 0.4*recency
		candidates = append(candidates, scored{idx: i, score: score, tokens: tok})
	}

	sort.Slice(candidates, func(a, b int) bool {
		return candidates[a].score > candidates[b].score
	})

	selected := make([]bool, n)
	copy(selected, alwaysKeep)
	remaining := budgetTokens - usedTokens
	for _, c := range candidates {
		if c.tokens > remaining {
			continue
		}
		selected[c.idx] = true
		remaining -= c.tokens
		usedTokens += c.tokens
	}

	out := make([]ContextMessage, 0, n)
	dropped := 0
	for i, m := range messages {
		if selected[i] {
			out = append(out, m)
		} else {
			dropped++
		}
	}

	return OptimizeContextOutput{
		Messages:        out,
		OriginalTokens:  originalTokens,
		OptimizedTokens: usedTokens,
		TokensSaved:     originalTokens - usedTokens,
		Dropped:         dropped,
	}
}

func lastUserContentOf(messages []ContextMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(messages[i].Role, "user") {
			return messages[i].Content
		}
	}
	return ""
}

func wordSetOf(s string) map[string]struct{} {
	fields := strings.Fields(strings.ToLower(s))
	set := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		set[f] = struct{}{}
	}
	return set
}

func jaccardOf(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	intersection := 0
	for k := range a {
		if _, ok := b[k]; ok {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// MeasureContextEfficiency reports how much required evidence remains in a
// pruned context. Assertions are matched case-insensitively after whitespace
// normalization, making a JSON assertion set a small, portable evaluation
// fixture for a real chat history.
func MeasureContextEfficiency(messages []ContextMessage, assertions []string, optimizedTokens int) ContextEfficiency {
	metric := ContextEfficiency{
		RequiredAssertions:     len(assertions),
		OptimizedContextTokens: optimizedTokens,
	}
	if len(assertions) == 0 {
		return metric
	}

	var content strings.Builder
	for _, message := range messages {
		content.WriteString(" ")
		content.WriteString(normalizeEvidenceText(message.Content))
	}
	selectedText := content.String()
	for _, assertion := range assertions {
		normalized := normalizeEvidenceText(assertion)
		if normalized != "" && strings.Contains(selectedText, normalized) {
			metric.RetainedAssertions++
		}
	}
	metric.CoveragePercent = float64(metric.RetainedAssertions) / float64(metric.RequiredAssertions) * 100
	if optimizedTokens > 0 {
		metric.CoveragePer1KTokens = metric.CoveragePercent / float64(optimizedTokens) * 1000
	}
	return metric
}

func normalizeEvidenceText(text string) string {
	return strings.Join(strings.Fields(strings.ToLower(text)), " ")
}
