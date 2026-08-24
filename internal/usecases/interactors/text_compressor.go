// Package interactors provides the application use cases.
package interactors

import (
	"regexp"
	"strings"
)

// fillerReplacements maps verbose phrases to terse equivalents.
// Applied case-insensitively with word-boundary anchors.
// Order matters: more specific phrases before subsets.
var fillerReplacements = []struct{ pattern, replacement string }{
	// Multi-word fillers — longer/more specific first
	{`(?i)\bIt is important to note that\b`, ""},
	{`(?i)\bplease note that\b`, ""},
	{`(?i)\bDue to the fact that\b`, "because"},
	{`(?i)\bIn the event that\b`, "if"},
	{`(?i)\bAt this point in time\b`, "now"},
	{`(?i)\bFor the purpose of\b`, "for"},
	{`(?i)\bWith regards to\b`, "about"},
	{`(?i)\bWith regard to\b`, "about"},
	{`(?i)\bIn terms of\b`, "for"},
	{`(?i)\bIn order to\b`, "to"},
	{`(?i)\bIn conclusion\b`, ""},
	{`(?i)\bAs a result\b`, "so"},
	{`(?i)\bThe reason why\b`, "why"},
	{`(?i)\bfor the reason that\b`, "because"},
	// Verb simplifications
	{`(?i)\bhas been\b`, "is"},
	{`(?i)\bhave been\b`, "are"},
	{`(?i)\bwas able to\b`, "could"},
	{`(?i)\bwere able to\b`, "could"},
	{`(?i)\bwith the exception of\b`, "except"},
	{`(?i)\buntil such time as\b`, "until"},
	{`(?i)\bprior to\b`, "before"},
	{`(?i)\bsubsequent to\b`, "after"},
}

// compiled holds pre-compiled regexps for performance.
var compiled []struct {
	re          *regexp.Regexp
	replacement string
}

// multiSpaceRe collapses runs of spaces.
var multiSpaceRe = regexp.MustCompile(` {2,}`)

// multiNewlineRe collapses 3+ consecutive newlines.
var multiNewlineRe = regexp.MustCompile(`\n{3,}`)

func init() {
	for _, fr := range fillerReplacements {
		compiled = append(compiled, struct {
			re          *regexp.Regexp
			replacement string
		}{
			re:          regexp.MustCompile(fr.pattern),
			replacement: fr.replacement,
		})
	}
}

// CompressText applies rule-based filler-phrase removal and whitespace
// normalization to produce a terse version of the input text.
// The original technical content is preserved; only verbose prose is removed.
// Safe to call on empty strings.
func CompressText(text string) string {
	if text == "" {
		return ""
	}

	result := text

	// Apply phrase substitutions
	for _, c := range compiled {
		result = c.re.ReplaceAllString(result, c.replacement)
	}

	// Normalize spaces (multiple spaces → one)
	result = multiSpaceRe.ReplaceAllString(result, " ")

	// Normalize blank lines (3+ → 2)
	result = multiNewlineRe.ReplaceAllString(result, "\n\n")

	return strings.TrimSpace(result)
}

// EstimateSummaryTokens returns an approximate token count for a compressed
// summary using whitespace-split word counting (consistent with MIRA's
// verbatim token_count estimation).
func EstimateSummaryTokens(summary string) int {
	if summary == "" {
		return 0
	}
	return len(strings.Fields(summary))
}
