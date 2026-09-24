// Package agentbridge translates client hook events into bounded MIRA memory
// operations. It is intentionally short-lived and contains no server loop.
package agentbridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/benoitpetit/mira/internal/agentinstall"
)

const (
	EventSessionStart     = "session-start"
	EventPromptSubmit     = "prompt-submit"
	EventResponseComplete = "response-complete"

	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Event is the normalized input shared by all client adapters.
type Event struct {
	Client       string `json:"client"`
	EventName    string `json:"event"`
	Role         string `json:"role"`
	Content      string `json:"content"`
	SessionID    string `json:"session_id"`
	ThreadID     string `json:"thread_id"`
	Wing         string `json:"wing"`
	Room         string `json:"room"`
	ManifestPath string `json:"manifest_path"`
}

type CaptureInput struct {
	Role      string
	Content   string
	Wing      string
	Room      string
	SessionID string
	ThreadID  string
}

type SoulObservation struct {
	AgentID   string
	Role      string
	Content   string
	SessionID string
	ModelID   string
}

// Backend is the small capability surface needed by the bridge. The concrete
// command adapter supplies an app-backed implementation; tests and embedding
// clients can use a local implementation without starting MIRA servers.
type Backend interface {
	Recall(ctx context.Context, query string, budget int, wing string) (string, error)
	Capture(ctx context.Context, input CaptureInput) error
}

type SoulBackend interface {
	SoulRecall(ctx context.Context, query string, budget int, wing string) (string, error)
	SoulObserve(ctx context.Context, observation SoulObservation) error
}

type Result struct {
	Continue    bool     `json:"continue"`
	Context     string   `json:"context,omitempty"`
	Captured    bool     `json:"captured"`
	Diagnostics []string `json:"diagnostics,omitempty"`
}

type Bridge struct {
	backend  Backend
	manifest agentinstall.Manifest

	mu   sync.Mutex
	seen map[string]struct{}
}

func New(backend Backend, manifest agentinstall.Manifest) *Bridge {
	return &Bridge{backend: backend, manifest: manifest, seen: make(map[string]struct{})}
}

func (b *Bridge) Handle(ctx context.Context, event Event) (Result, error) {
	result := Result{Continue: true}
	if b == nil || b.backend == nil {
		return Result{}, errors.New("agent bridge backend is required")
	}
	if err := validateEvent(event); err != nil {
		return Result{}, err
	}

	wing := strings.TrimSpace(event.Wing)
	if wing == "" {
		wing = b.manifest.Wing
	}
	if b.manifest.Recall.Enabled && (event.EventName == EventSessionStart || event.EventName == EventPromptSubmit) && b.manifest.Policy.InjectionEnabled() {
		query := strings.TrimSpace(event.Content)
		if query == "" {
			query = "session start"
		}
		memoryBudget := b.manifest.Recall.BudgetTokens
		identity := ""
		if soul, ok := b.backend.(SoulBackend); ok && b.manifest.Soul.Enabled {
			identityBudget := memoryBudget * 2 / 5
			if identityBudget < 32 && memoryBudget >= 64 {
				identityBudget = 32
			}
			memoryBudget -= identityBudget
			var soulErr error
			identity, soulErr = soul.SoulRecall(ctx, query, identityBudget, wing)
			if soulErr != nil {
				result.Diagnostics = append(result.Diagnostics, "soul recall unavailable")
			}
		}
		memory, err := b.backend.Recall(ctx, query, memoryBudget, wing)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, "recall unavailable")
		} else if contextText := renderReferenceContextParts(identity, memory, b.manifest.Recall.BudgetTokens); contextText != "" {
			result.Context = contextText
		}
	}

	if b.shouldCapture(event) {
		content, _ := agentinstall.RedactSecrets(event.Content)
		if agentinstall.IsSubstantivePrompt(content, b.manifest.Capture.MinChars) {
			key := captureKey(event, content, wing)
			if b.markSeen(key) {
				if err := b.backend.Capture(ctx, CaptureInput{
					Role: event.Role, Content: content, Wing: wing, Room: event.Room,
					SessionID: event.SessionID, ThreadID: event.ThreadID,
				}); err != nil {
					result.Diagnostics = append(result.Diagnostics, "capture unavailable")
				} else {
					result.Captured = true
				}
			}
		}
	}
	if event.EventName == EventResponseComplete && event.Role == RoleAssistant && b.manifest.Policy.CaptureAssistantResponses() && b.manifest.Soul.Enabled && b.manifest.Soul.ObserveAssistant {
		if soul, ok := b.backend.(SoulBackend); ok {
			content, _ := agentinstall.RedactSecrets(event.Content)
			if agentinstall.IsSubstantivePrompt(content, b.manifest.Capture.MinChars) {
				if err := soul.SoulObserve(ctx, SoulObservation{AgentID: wing, Role: event.Role, Content: content, SessionID: event.SessionID, ModelID: event.Client}); err != nil {
					result.Diagnostics = append(result.Diagnostics, "soul observation unavailable")
				}
			}
		}
	}
	return result, nil
}

func (b *Bridge) shouldCapture(event Event) bool {
	switch {
	case event.EventName == EventPromptSubmit && event.Role == RoleUser:
		return b.manifest.Policy.CaptureUserPrompts() && b.manifest.Capture.UserPrompts
	case event.EventName == EventResponseComplete && event.Role == RoleAssistant:
		return b.manifest.Policy.CaptureAssistantResponses()
	default:
		return false
	}
}

func (b *Bridge) markSeen(key string) bool {
	if !b.manifest.Capture.DeduplicateSessions {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.seen[key]; exists {
		return false
	}
	b.seen[key] = struct{}{}
	return true
}

func validateEvent(event Event) error {
	switch event.EventName {
	case EventSessionStart, EventPromptSubmit, EventResponseComplete:
	default:
		return fmt.Errorf("unsupported agent event %q", event.EventName)
	}
	if event.EventName != EventSessionStart && strings.TrimSpace(event.Content) == "" {
		return errors.New("agent event content is required")
	}
	if event.EventName == EventPromptSubmit && event.Role != RoleUser {
		return fmt.Errorf("prompt-submit role %q is invalid", event.Role)
	}
	if event.EventName == EventResponseComplete && event.Role != RoleAssistant {
		return fmt.Errorf("response-complete role %q is invalid", event.Role)
	}
	return nil
}

func renderReferenceContext(memory string, budget int) string {
	return renderReferenceContextParts("", memory, budget)
}

func renderReferenceContextParts(identity, memory string, budget int) string {
	identity, _ = agentinstall.RedactSecrets(strings.TrimSpace(identity))
	cleaned, _ := agentinstall.RedactSecrets(strings.TrimSpace(memory))
	if identity == "" && cleaned == "" {
		return ""
	}
	var sections []string
	if identity != "" {
		sections = append(sections, "### Identity continuity (reference only)\n"+identity)
	}
	if cleaned != "" {
		sections = append(sections, "### Relevant MIRA memory evidence (reference only)\n"+cleaned)
	}
	cleaned = strings.Join(sections, "\n\n")
	maxChars := budget * 4
	if maxChars > 0 && len([]rune(cleaned)) > maxChars {
		cleaned = string([]rune(cleaned)[:maxChars]) + "…"
	}
	return "<MIRA_CONTEXT trust=\"reference-only\">\n" + cleaned + "\n</MIRA_CONTEXT>"
}

func captureKey(event Event, content, wing string) string {
	value := event.SessionID + "\x00" + event.ThreadID + "\x00" + event.Role + "\x00" + wing + "\x00" + content
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
