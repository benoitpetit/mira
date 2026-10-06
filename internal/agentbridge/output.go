package agentbridge

import (
	"encoding/json"
)

// RenderHookOutput serializes a neutral bridge result using the wire contract
// of the client that invoked it. Only clients with documented context-injection
// hooks receive recalled context in their output.
func RenderHookOutput(client, event string, result Result) ([]byte, error) {
	type hookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext,omitempty"`
	}
	type hookOutput struct {
		Continue           bool                `json:"continue"`
		Captured           bool                `json:"captured,omitempty"`
		Diagnostics        []string            `json:"diagnostics,omitempty"`
		HookSpecificOutput *hookSpecificOutput `json:"hookSpecificOutput,omitempty"`
	}

	output := hookOutput{Continue: result.Continue, Captured: result.Captured, Diagnostics: result.Diagnostics}
	if result.Context != "" {
		hookEventName, ok := hookEventName(client, event)
		if ok {
			output.HookSpecificOutput = &hookSpecificOutput{HookEventName: hookEventName, AdditionalContext: result.Context}
		}
	}
	return json.Marshal(output)
}

func hookEventName(client, event string) (string, bool) {
	if client != "codex" && client != "claude-code" {
		return "", false
	}
	switch event {
	case EventSessionStart:
		return "SessionStart", true
	case EventPromptSubmit:
		return "UserPromptSubmit", true
	case EventResponseComplete:
		return "Stop", true
	default:
		return "", false
	}
}
