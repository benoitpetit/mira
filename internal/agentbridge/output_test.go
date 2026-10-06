package agentbridge

import (
	"encoding/json"
	"testing"
)

func TestRenderHookOutputInjectsContextForCodexAndClaudeCode(t *testing.T) {
	for _, client := range []string{"codex", "claude-code"} {
		t.Run(client, func(t *testing.T) {
			output, err := RenderHookOutput(client, EventPromptSubmit, Result{Continue: true, Captured: true, Context: "<MIRA_CONTEXT trust=\"reference-only\">memory</MIRA_CONTEXT>"})
			if err != nil {
				t.Fatalf("RenderHookOutput failed: %v", err)
			}
			var decoded struct {
				HookSpecificOutput struct {
					HookEventName     string `json:"hookEventName"`
					AdditionalContext string `json:"additionalContext"`
				} `json:"hookSpecificOutput"`
			}
			if err := json.Unmarshal(output, &decoded); err != nil {
				t.Fatalf("decode output: %v", err)
			}
			if decoded.HookSpecificOutput.HookEventName != "UserPromptSubmit" || decoded.HookSpecificOutput.AdditionalContext == "" {
				t.Fatalf("unexpected hook output: %s", output)
			}
		})
	}
}

func TestRenderHookOutputDoesNotInjectCaptureOnlyClientContext(t *testing.T) {
	output, err := RenderHookOutput("windsurf", EventPromptSubmit, Result{Continue: true, Context: "must not be injected"})
	if err != nil {
		t.Fatalf("RenderHookOutput failed: %v", err)
	}
	if string(output) != `{"continue":true}` {
		t.Fatalf("unexpected capture-only output: %s", output)
	}
}
