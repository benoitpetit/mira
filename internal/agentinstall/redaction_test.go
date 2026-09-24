package agentinstall

import "testing"

func TestRedactSecretsCoversCredentialShapesWithoutLeakingValues(t *testing.T) {
	input := "api_key=sk-test-secret bearer token Bearer abc.def.ghi password: hunter2\n" +
		"Cookie: session=secret-cookie\n-----BEGIN PRIVATE KEY-----\nprivate-material\n-----END PRIVATE KEY-----"
	output, changed := RedactSecrets(input)
	if !changed {
		t.Fatal("RedactSecrets reported no change")
	}
	for _, secret := range []string{"sk-test-secret", "abc.def.ghi", "hunter2", "secret-cookie", "private-material"} {
		if containsManagedBody(output, secret) {
			t.Fatalf("secret %q leaked in %q", secret, output)
		}
	}
	for _, marker := range []string{"[REDACTED_API_KEY]", "[REDACTED_BEARER_TOKEN]", "[REDACTED_PASSWORD]", "[REDACTED_COOKIE]", "[REDACTED_PRIVATE_KEY]"} {
		if !containsManagedBody(output, marker) {
			t.Fatalf("redaction marker %q missing from %q", marker, output)
		}
	}
}

func TestRedactSecretsLeavesOrdinaryTextStable(t *testing.T) {
	input := "Remember the API design and the user's preferred tone."
	output, changed := RedactSecrets(input)
	if changed || output != input {
		t.Fatalf("ordinary text changed: %q -> %q", input, output)
	}
}

func TestRedactSecretsCoversRawOpenAIStyleKeys(t *testing.T) {
	input := "Do not persist sk-proj-1234567890abcdef1234567890abcdef in memory."
	output, changed := RedactSecrets(input)
	if !changed || containsManagedBody(output, "sk-proj-1234567890abcdef1234567890abcdef") || !containsManagedBody(output, "[REDACTED_API_KEY]") {
		t.Fatalf("raw key was not redacted: %q", output)
	}
}
