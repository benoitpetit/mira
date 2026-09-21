package extraction

import (
	"strings"
	"unicode"
)

// estimateTokenCount returns a deterministic local token estimate.
// MIRA must remain usable without network access. The previous implementation
// initialized tiktoken-go during startup, which downloaded the cl100k_base
// vocabulary from Azure when it was not cached.
func estimateTokenCount(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}

	tokens := 0
	inWord := false
	for _, r := range text {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			inWord = true
		case unicode.IsSpace(r):
			if inWord {
				tokens++
				inWord = false
			}
		case inWord:
			tokens++
			inWord = false
			tokens++
		default:
			tokens++
		}
	}
	if inWord {
		tokens++
	}
	return tokens
}
