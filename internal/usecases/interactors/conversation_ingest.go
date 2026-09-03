package interactors

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
)

// ConversationMessage is the portable subset shared by common JSON chat exports.
type ConversationMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ParseConversationMessages accepts either a JSON array or a {"messages":[]}
// envelope, both common formats for conversation exports.
func ParseConversationMessages(data []byte) ([]ConversationMessage, error) {
	var messages []ConversationMessage
	if err := json.Unmarshal(data, &messages); err == nil {
		return validateConversationMessages(messages)
	}
	var envelope struct {
		Messages []ConversationMessage `json:"messages"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("conversation must be a JSON message array or an object with a messages array: %w", err)
	}
	return validateConversationMessages(envelope.Messages)
}

// ParseConversationMessage validates one JSON Lines conversation event. It is
// intended for long-lived streams where each line carries one {role, content}
// object instead of an exported conversation array.
func ParseConversationMessage(data []byte) (ConversationMessage, error) {
	var message ConversationMessage
	if err := json.Unmarshal(data, &message); err != nil {
		return ConversationMessage{}, fmt.Errorf("conversation message must be a JSON object: %w", err)
	}
	if _, err := validateConversationMessages([]ConversationMessage{message}); err != nil {
		return ConversationMessage{}, err
	}
	return message, nil
}

// ParseConversationStreamMessage accepts a portable {role,content} JSONL line
// and Cursor CLI stream-json user/result events. The boolean is false for
// non-conversation events such as tool calls and assistant text deltas.
func ParseConversationStreamMessage(data []byte) (ConversationMessage, bool, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return ConversationMessage{}, false, fmt.Errorf("conversation stream event must be JSON: %w", err)
	}
	if _, hasRole := fields["role"]; hasRole {
		message, err := ParseConversationMessage(data)
		return message, true, err
	}

	var event struct {
		Type    string `json:"type"`
		Result  string `json:"result"`
		Message struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return ConversationMessage{}, false, fmt.Errorf("parse conversation stream event: %w", err)
	}
	switch event.Type {
	case "user":
		content, err := conversationContentText(event.Message.Content)
		if err != nil {
			return ConversationMessage{}, false, err
		}
		message := ConversationMessage{Role: event.Message.Role, Content: content}
		if _, err := validateConversationMessages([]ConversationMessage{message}); err != nil {
			return ConversationMessage{}, false, err
		}
		return message, true, nil
	case "result":
		if strings.TrimSpace(event.Result) == "" {
			return ConversationMessage{}, false, nil
		}
		return ConversationMessage{Role: "assistant", Content: event.Result}, true, nil
	default:
		return ConversationMessage{}, false, nil
	}
}

func conversationContentText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", fmt.Errorf("conversation stream message content must be text or text parts: %w", err)
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n"), nil
}

// ValidateConversationMessages rejects empty conversations and messages without
// a role. It is exported so non-JSON adapters can apply the same validation.
func ValidateConversationMessages(messages []ConversationMessage) ([]ConversationMessage, error) {
	return validateConversationMessages(messages)
}

func validateConversationMessages(messages []ConversationMessage) ([]ConversationMessage, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("conversation contains no messages")
	}
	for index, message := range messages {
		if strings.TrimSpace(message.Role) == "" {
			return nil, fmt.Errorf("conversation message %d has no role", index+1)
		}
	}
	return messages, nil
}

// ConversationMemoryInputs selects substantive conversation messages and
// translates them into normal MIRA store inputs. Captured conversation content
// is always categorised as history while extraction still infers its technical
// memory type.
func ConversationMemoryInputs(messages []ConversationMessage, wing string, room *string, includeAssistant bool, minChars int) ([]StoreMemoryInput, error) {
	if !WingRoomRe.MatchString(wing) {
		return nil, fmt.Errorf("wing must be 1-100 alphanumeric characters, hyphens or underscores")
	}
	if len([]rune(wing)) > 100 {
		return nil, fmt.Errorf("wing exceeds maximum length of 100 characters")
	}
	if room != nil && (!WingRoomRe.MatchString(*room) || len([]rune(*room)) > 100) {
		return nil, fmt.Errorf("room must be 1-100 alphanumeric characters, hyphens or underscores")
	}
	if minChars < 1 {
		minChars = 1
	}

	kind := valueobjects.KindHistory
	inputs := make([]StoreMemoryInput, 0, len(messages))
	for index, message := range messages {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		if role != "user" && (!includeAssistant || role != "assistant") {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if len([]rune(content)) < minChars {
			continue
		}
		inputs = append(inputs, StoreMemoryInput{
			Content: content,
			Wing:    wing,
			Room:    room,
			Kind:    &kind,
			Metrics: map[string]any{
				"source":        "conversation_ingest",
				"role":          role,
				"message_index": index + 1,
			},
		})
	}
	return inputs, nil
}
