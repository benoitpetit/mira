package agentinstall

import (
	"strings"
	"unicode/utf8"
)

// IsSubstantivePrompt keeps transient control chatter out of automatic
// capture while allowing decisions, questions and constraints through.
func IsSubstantivePrompt(input string, minChars int) bool {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return false
	}
	if minChars > 0 && utf8.RuneCountInString(trimmed) < minChars {
		return false
	}
	if strings.HasPrefix(trimmed, "/") {
		return false
	}
	first := strings.ToLower(strings.Fields(trimmed)[0])
	switch first {
	case "ls", "pwd", "cd", "clear", "reset", "history", "whoami":
		return false
	default:
		return true
	}
}
