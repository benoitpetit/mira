package interactors

import (
	"testing"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
)

func TestParseConversationMessagesAcceptsArrayAndEnvelope(t *testing.T) {
	for _, data := range [][]byte{
		[]byte(`[{"role":"user","content":"Remember this"}]`),
		[]byte(`{"messages":[{"role":"user","content":"Remember this"}]}`),
	} {
		messages, err := ParseConversationMessages(data)
		if err != nil {
			t.Fatalf("ParseConversationMessages failed: %v", err)
		}
		if len(messages) != 1 || messages[0].Role != "user" {
			t.Fatalf("messages = %#v", messages)
		}
	}
}

func TestParseConversationMessageAcceptsJSONLine(t *testing.T) {
	message, err := ParseConversationMessage([]byte(`{"role":"assistant","content":"A streamed response."}`))
	if err != nil {
		t.Fatalf("ParseConversationMessage failed: %v", err)
	}
	if message.Role != "assistant" || message.Content != "A streamed response." {
		t.Errorf("message = %#v", message)
	}
	if _, err := ParseConversationMessage([]byte(`{"content":"missing role"}`)); err == nil {
		t.Error("expected a JSONL message without a role to be rejected")
	}
}

func TestParseConversationStreamMessageAcceptsCursorCLIEvents(t *testing.T) {
	user, recognized, err := ParseConversationStreamMessage([]byte(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Use PostgreSQL for this service."}]},"session_id":"abc"}`))
	if err != nil || !recognized {
		t.Fatalf("user event: recognized=%v err=%v", recognized, err)
	}
	if user.Role != "user" || user.Content != "Use PostgreSQL for this service." {
		t.Errorf("user = %#v", user)
	}
	assistant, recognized, err := ParseConversationStreamMessage([]byte(`{"type":"result","result":"I configured PostgreSQL."}`))
	if err != nil || !recognized {
		t.Fatalf("result event: recognized=%v err=%v", recognized, err)
	}
	if assistant.Role != "assistant" || assistant.Content != "I configured PostgreSQL." {
		t.Errorf("assistant = %#v", assistant)
	}
	if _, recognized, err := ParseConversationStreamMessage([]byte(`{"type":"tool_call","subtype":"started"}`)); err != nil || recognized {
		t.Errorf("tool event: recognized=%v err=%v", recognized, err)
	}
}

func TestConversationMemoryInputsSelectsAndAnnotatesMessages(t *testing.T) {
	messages := []ConversationMessage{
		{Role: "user", Content: "  Keep this substantive user message.  "},
		{Role: "assistant", Content: "Keep this assistant reply too."},
		{Role: "tool", Content: "Ignore this tool payload."},
	}
	inputs, err := ConversationMemoryInputs(messages, "api", nil, true, 20)
	if err != nil {
		t.Fatalf("ConversationMemoryInputs failed: %v", err)
	}
	if len(inputs) != 2 {
		t.Fatalf("selected %d inputs, want 2", len(inputs))
	}
	if inputs[0].Content != "Keep this substantive user message." {
		t.Errorf("content = %q", inputs[0].Content)
	}
	if inputs[0].Kind == nil || *inputs[0].Kind != valueobjects.KindHistory {
		t.Errorf("kind = %v, want history", inputs[0].Kind)
	}
	if inputs[1].Metrics["role"] != "assistant" || inputs[1].Metrics["message_index"] != 2 {
		t.Errorf("metrics = %#v", inputs[1].Metrics)
	}
}
