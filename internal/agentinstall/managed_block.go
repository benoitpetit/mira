package agentinstall

import (
	"fmt"
	"strings"
)

const (
	managedBegin = "<!-- MIRA:BEGIN managed instructions -->"
	managedEnd   = "<!-- MIRA:END managed instructions -->"
)

func MergeManagedBlock(existing, body string) (string, error) {
	if strings.Contains(body, managedBegin) || strings.Contains(body, managedEnd) {
		return "", fmt.Errorf("managed instruction body must not contain MIRA markers")
	}
	start, end, sep, err := managedRange(existing)
	if err != nil {
		return "", err
	}
	if start < 0 {
		sep = newlineFor(existing)
		prefix := existing
		if prefix != "" && !strings.HasSuffix(prefix, "\n") && !strings.HasSuffix(prefix, "\r") {
			prefix += sep
		}
		return prefix + formatManagedBlock(body, sep), nil
	}
	blockStart, blockEnd := managedLineRange(existing, start, end)
	return existing[:blockStart] + formatManagedBlock(body, sep) + existing[blockEnd:], nil
}

func RemoveManagedBlock(existing string) (string, error) {
	start, end, _, err := managedRange(existing)
	if err != nil {
		return "", err
	}
	if start < 0 {
		return existing, nil
	}
	blockStart, blockEnd := managedLineRange(existing, start, end)
	return existing[:blockStart] + existing[blockEnd:], nil
}

func managedRange(value string) (start, end int, sep string, err error) {
	beginCount := strings.Count(value, managedBegin)
	endCount := strings.Count(value, managedEnd)
	if beginCount == 0 && endCount == 0 {
		return -1, -1, newlineFor(value), nil
	}
	if beginCount != 1 || endCount != 1 {
		return 0, 0, "", fmt.Errorf("MIRA managed instructions contain duplicate markers")
	}
	start = strings.Index(value, managedBegin)
	end = strings.Index(value, managedEnd)
	if end < start {
		return 0, 0, "", fmt.Errorf("MIRA managed instruction markers are out of order")
	}
	return start, end, newlineFor(value), nil
}

func managedLineRange(value string, start, end int) (int, int) {
	lineStart := strings.LastIndex(value[:start], "\n") + 1
	lineEnd := end + len(managedEnd)
	if strings.HasPrefix(value[lineEnd:], "\r\n") {
		lineEnd += 2
	} else if strings.HasPrefix(value[lineEnd:], "\n") {
		lineEnd++
	}
	return lineStart, lineEnd
}

func formatManagedBlock(body, sep string) string {
	body = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\r", "\n"))
	body = strings.ReplaceAll(body, "\n", sep)
	return managedBegin + sep + body + sep + managedEnd + sep
}

func newlineFor(value string) string {
	if strings.Contains(value, "\r\n") {
		return "\r\n"
	}
	return "\n"
}
