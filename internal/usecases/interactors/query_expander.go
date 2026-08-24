package interactors

import (
	"regexp"
	"sort"
	"strings"
)

var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true, "was": true, "were": true,
	"be": true, "been": true, "being": true, "have": true, "has": true, "had": true, "do": true,
	"does": true, "did": true, "will": true, "would": true, "shall": true, "should": true,
	"can": true, "could": true, "may": true, "might": true, "must": true,
	"to": true, "of": true, "in": true, "for": true, "on": true, "with": true, "at": true,
	"by": true, "from": true, "as": true, "into": true, "through": true, "during": true,
	"before": true, "after": true, "above": true, "below": true, "between": true, "under": true,
	"and": true, "but": true, "or": true, "yet": true, "so": true, "if": true, "because": true,
	"although": true, "though": true, "while": true, "where": true, "when": true, "that": true,
	"this": true, "these": true, "those": true, "i": true, "you": true, "he": true, "she": true,
	"it": true, "we": true, "they": true, "me": true, "him": true, "her": true, "us": true,
	"them": true, "my": true, "your": true, "his": true, "its": true, "our": true, "their": true,
	"le": true, "la": true, "les": true, "un": true, "une": true, "des": true, "du": true, "de": true,
	"et": true, "ou": true, "mais": true, "donc": true, "si": true, "que": true, "qui": true,
	"quoi": true, "dont": true, "où": true, "comment": true, "pourquoi": true, "quand": true,
	"à": true, "au": true, "aux": true, "en": true, "dans": true, "sur": true, "sous": true,
	"par": true, "pour": true, "avec": true, "sans": true, "contre": true, "vers": true,
	"je": true, "tu": true, "il": true, "elle": true, "nous": true, "vous": true, "ils": true,
	"elles": true, "mon": true, "ton": true, "son": true, "notre": true, "votre": true, "leur": true,
	"ce": true, "cet": true, "cette": true, "ces": true, "est": true, "sont": true, "été": true,
}

var punctuationRe = regexp.MustCompile(`[^\w\s]`)
var whitespaceRe = regexp.MustCompile(`\s+`)

// expandQuery generates semantically-close variants of the input query.
// It returns at most numVariants + 1 strings (original + variants).
func expandQuery(query string, numVariants int) []string {
	if numVariants <= 0 {
		return []string{query}
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return []string{query}
	}

	variants := []string{query}
	lower := strings.ToLower(query)
	clean := punctuationRe.ReplaceAllString(lower, " ")
	clean = whitespaceRe.ReplaceAllString(clean, " ")
	clean = strings.TrimSpace(clean)

	if clean != query && clean != "" {
		variants = append(variants, clean)
	}

	words := strings.Split(clean, " ")
	if len(words) > 1 {
		// Variant without stop words
		var filtered []string
		for _, w := range words {
			if !stopWords[w] && w != "" {
				filtered = append(filtered, w)
			}
		}
		if len(filtered) > 0 && len(filtered) < len(words) {
			noStop := strings.Join(filtered, " ")
			if noStop != clean && noStop != query {
				variants = append(variants, noStop)
			}
		}
	}

	if len(words) > 3 {
		// Variant with longest words (most informative)
		sorted := make([]string, len(words))
		copy(sorted, words)
		sort.Slice(sorted, func(i, j int) bool {
			return len(sorted[i]) > len(sorted[j])
		})
		keywords := sorted[:3]
		keywordQuery := strings.Join(keywords, " ")
		if keywordQuery != clean && keywordQuery != query {
			variants = append(variants, keywordQuery)
		}
	}

	if len(variants) > numVariants+1 {
		variants = variants[:numVariants+1]
	}
	return variants
}
