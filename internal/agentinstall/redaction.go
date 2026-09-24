package agentinstall

import "regexp"

var secretRedactors = []struct {
	re *regexp.Regexp
	to string
}{
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`), "[REDACTED_PRIVATE_KEY]"},
	{regexp.MustCompile(`(?im)^\s*cookie\s*:\s*[^\r\n]+`), "Cookie: [REDACTED_COOKIE]"},
	{regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`), "Bearer [REDACTED_BEARER_TOKEN]"},
	{regexp.MustCompile(`(?i)(?:api[_-]?key|secret[_-]?key)\s*[:=]\s*[^\s,;]+`), "api_key=[REDACTED_API_KEY]"},
	{regexp.MustCompile(`(?i)(?:password|passwd|pwd)\s*[:=]\s*[^\s,;]+`), "password=[REDACTED_PASSWORD]"},
	{regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}`), "[REDACTED_API_KEY]"},
}

// RedactSecrets removes common credential-shaped values before content is
// stored or emitted as agent context. It intentionally favors false positives
// over leaking a credential into a durable memory store.
func RedactSecrets(input string) (string, bool) {
	output := input
	for _, redactor := range secretRedactors {
		output = redactor.re.ReplaceAllString(output, redactor.to)
	}
	return output, output != input
}
