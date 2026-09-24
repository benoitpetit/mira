package agentbridge

import (
	"context"
	"strings"

	"github.com/benoitpetit/mira/internal/agentmemory"
	"github.com/benoitpetit/mira/internal/app"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/interactors"
)

// MiraBackend adapts the existing use cases to the short-lived bridge. It
// deliberately uses the same recall and store paths as MCP and REST clients.
type MiraBackend struct {
	application *app.Application
}

func NewMiraBackend(application *app.Application) *MiraBackend {
	return &MiraBackend{application: application}
}

func (b *MiraBackend) Recall(ctx context.Context, query string, budget int, wing string) (string, error) {
	wingRef := strings.TrimSpace(wing)
	out, err := b.application.RecallMemoryUC().Execute(ctx, interactors.RecallMemoryInput{
		Query: query, Budget: budget, Wing: &wingRef,
	})
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(out.Memories))
	for _, memory := range out.Memories {
		if memory != nil && strings.TrimSpace(memory.Rendered) != "" {
			parts = append(parts, memory.Rendered)
		}
	}
	return strings.Join(parts, "\n"), nil
}

func (b *MiraBackend) Capture(ctx context.Context, input CaptureInput) error {
	room := strings.TrimSpace(input.Room)
	var roomRef *string
	if room != "" {
		roomRef = &room
	}
	kind := valueobjects.KindHistory
	_, err := b.application.StoreMemoryUC().Execute(ctx, interactors.StoreMemoryInput{
		Content: input.Content,
		Wing:    input.Wing,
		Room:    roomRef,
		Kind:    &kind,
		Metrics: map[string]any{
			"source":     "agent_bridge",
			"role":       input.Role,
			"session_id": input.SessionID,
			"thread_id":  input.ThreadID,
		},
	})
	return err
}

func (b *MiraBackend) SoulRecall(ctx context.Context, query string, budget int, wing string) (string, error) {
	runtime := b.application.AgentMemoryApplication()
	if runtime == nil {
		return "", nil
	}
	prompt, err := runtime.Recall(ctx, wing, query, budget)
	if err != nil {
		return "", err
	}
	return prompt.Content, nil
}

func (b *MiraBackend) SoulObserve(ctx context.Context, observation SoulObservation) error {
	runtime := b.application.AgentMemoryApplication()
	if runtime == nil {
		return nil
	}
	_, err := runtime.Capture(ctx, agentmemory.CaptureRequest{
		AgentID:   observation.AgentID,
		ModelID:   observation.ModelID,
		SessionID: observation.SessionID,
		Messages:  []agentmemory.ConversationObservation{{Role: observation.Role, Content: observation.Content, SessionID: observation.SessionID}},
	})
	return err
}
