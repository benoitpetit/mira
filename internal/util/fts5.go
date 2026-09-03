package util

import (
	"strings"
	"unicode"
)

// NormalizeFTS5Query converts natural-language input into a safe FTS5 query.
// Each Unicode word is quoted so punctuation and FTS5 operators are treated as
// user text, while OR keeps lexical recall useful for conversational questions.
func NormalizeFTS5Query(query string) string {
	terms := strings.FieldsFunc(query, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	if len(terms) == 0 {
		return ""
	}

	quoted := make([]string, len(terms))
	for i, term := range terms {
		quoted[i] = `"` + term + `"`
	}
	return strings.Join(quoted, " OR ")
}
