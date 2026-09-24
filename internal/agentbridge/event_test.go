package agentbridge

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/benoitpetit/mira/internal/agentinstall"
)

type fakeBackend struct {
	recallText  string
	recallErr   error
	captureErr  error
	recallCalls []recallCall
	captures    []CaptureInput
}

type recallCall struct {
	query  string
	budget int
	wing   string
}

type soulBackend struct {
	*fakeBackend
	soulRecallCalls  []recallCall
	soulObservations []SoulObservation
}

func (s *soulBackend) SoulRecall(_ context.Context, query string, budget int, wing string) (string, error) {
	s.soulRecallCalls = append(s.soulRecallCalls, recallCall{query: query, budget: budget, wing: wing})
	return "The agent should stay precise and transparent.", nil
}

func (s *soulBackend) SoulObserve(_ context.Context, observation SoulObservation) error {
	s.soulObservations = append(s.soulObservations, observation)
	return nil
}

func (f *fakeBackend) Recall(_ context.Context, query string, budget int, wing string) (string, error) {
	f.recallCalls = append(f.recallCalls, recallCall{query: query, budget: budget, wing: wing})
	return f.recallText, f.recallErr
}

func (f *fakeBackend) Capture(_ context.Context, input CaptureInput) error {
	f.captures = append(f.captures, input)
	return f.captureErr
}

func TestHandleSessionStartInjectsReferenceOnlyContextWithinBudget(t *testing.T) {
	backend := &fakeBackend{recallText: "The user prefers concise architecture notes."}
	manifest := agentinstall.DefaultManifest("codex", t.TempDir())
	manifest.Recall.BudgetTokens = 240
	bridge := New(backend, manifest)

	result, err := bridge.Handle(context.Background(), Event{
		Client:    "codex",
		EventName: EventSessionStart,
		SessionID: "session-1",
		Wing:      manifest.Wing,
		Content:   "Design the memory integration",
	})
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if !result.Continue || !strings.Contains(result.Context, `<MIRA_CONTEXT trust="reference-only">`) || !strings.Contains(result.Context, "concise architecture") || !strings.Contains(result.Context, "</MIRA_CONTEXT>") {
		t.Fatalf("unexpected injected context: %q", result.Context)
	}
	if len(backend.recallCalls) != 1 || backend.recallCalls[0].budget != 240 || backend.recallCalls[0].wing != manifest.Wing {
		t.Fatalf("unexpected recall calls: %+v", backend.recallCalls)
	}
}

func TestHandlePromptCapturesRedactedSubstantiveUserContent(t *testing.T) {
	backend := &fakeBackend{}
	manifest := agentinstall.DefaultManifest("claude-code", t.TempDir())
	bridge := New(backend, manifest)
	content := "Use PostgreSQL for production and api_key=sk-test-secret"

	result, err := bridge.Handle(context.Background(), Event{
		Client:    "claude-code",
		EventName: EventPromptSubmit,
		Role:      RoleUser,
		Content:   content,
		SessionID: "session-2",
		ThreadID:  "thread-2",
		Wing:      manifest.Wing,
	})
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if !result.Captured || len(backend.captures) != 1 {
		t.Fatalf("prompt was not captured: result=%+v captures=%+v", result, backend.captures)
	}
	if strings.Contains(backend.captures[0].Content, "sk-test-secret") || backend.captures[0].Role != RoleUser {
		t.Fatalf("capture leaked or changed role: %+v", backend.captures[0])
	}
}

func TestHandleAssistantCaptureRequiresCompletePolicy(t *testing.T) {
	for _, policy := range []agentinstall.Policy{agentinstall.PolicyStandard, agentinstall.PolicyComplete} {
		t.Run(string(policy), func(t *testing.T) {
			backend := &fakeBackend{}
			manifest := agentinstall.DefaultManifest("codex", t.TempDir())
			manifest.Policy = policy
			bridge := New(backend, manifest)
			result, err := bridge.Handle(context.Background(), Event{
				Client:    "codex",
				EventName: EventResponseComplete,
				Role:      RoleAssistant,
				Content:   "The implementation should keep memory local and bounded.",
				SessionID: "session-3",
				Wing:      manifest.Wing,
			})
			if err != nil {
				t.Fatalf("Handle failed: %v", err)
			}
			wantCaptured := policy == agentinstall.PolicyComplete
			if result.Captured != wantCaptured || len(backend.captures) != boolInt(wantCaptured) {
				t.Fatalf("policy %s capture mismatch: result=%+v captures=%+v", policy, result, backend.captures)
			}
		})
	}
}

func TestHandleDeduplicatesSameSessionContent(t *testing.T) {
	backend := &fakeBackend{}
	manifest := agentinstall.DefaultManifest("codex", t.TempDir())
	bridge := New(backend, manifest)
	event := Event{Client: "codex", EventName: EventPromptSubmit, Role: RoleUser, Content: "Keep the memory store local to this project.", SessionID: "same", Wing: manifest.Wing}
	first, err := bridge.Handle(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	second, err := bridge.Handle(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Captured || second.Captured || len(backend.captures) != 1 {
		t.Fatalf("deduplication failed: first=%+v second=%+v captures=%d", first, second, len(backend.captures))
	}
}

func TestHandleFailsOpenOnRecallAndCaptureErrors(t *testing.T) {
	backend := &fakeBackend{recallErr: errors.New("recall unavailable"), captureErr: errors.New("store unavailable")}
	manifest := agentinstall.DefaultManifest("codex", t.TempDir())
	bridge := New(backend, manifest)
	result, err := bridge.Handle(context.Background(), Event{
		Client: "codex", EventName: EventPromptSubmit, Role: RoleUser,
		Content:   "Remember that deployments must be reviewed before production.",
		SessionID: "failure-session", Wing: manifest.Wing,
	})
	if err != nil || !result.Continue || len(result.Diagnostics) != 2 {
		t.Fatalf("bridge did not fail open: err=%v result=%+v", err, result)
	}
}

func TestHandleUsesSeparateSoulAndMemoryBudgetsAndObservesCompleteAssistant(t *testing.T) {
	backend := &soulBackend{fakeBackend: &fakeBackend{recallText: "A durable project decision."}}
	manifest := agentinstall.DefaultManifest("codex", t.TempDir())
	manifest.Policy = agentinstall.PolicyComplete
	manifest.Soul.ObserveAssistant = true
	manifest.Recall.BudgetTokens = 1000
	bridge := New(backend, manifest)

	result, err := bridge.Handle(context.Background(), Event{
		Client: "codex", EventName: EventResponseComplete, Role: RoleAssistant,
		Content: "The implementation remains local, bounded and reviewable.", SessionID: "soul-session", Wing: manifest.Wing,
	})
	if err != nil || !result.Continue || len(backend.soulObservations) != 1 {
		t.Fatalf("assistant soul observation failed: err=%v result=%+v observations=%+v", err, result, backend.soulObservations)
	}
	if backend.soulObservations[0].Role != RoleAssistant || !result.Captured {
		t.Fatalf("unexpected assistant observation/capture: %+v %+v", backend.soulObservations, result)
	}

	_, err = bridge.Handle(context.Background(), Event{Client: "codex", EventName: EventSessionStart, Content: "How should the agent answer?", SessionID: "soul-session-2", Wing: manifest.Wing})
	if err != nil || len(backend.soulRecallCalls) != 1 || len(backend.recallCalls) != 1 {
		t.Fatalf("separate soul/memory recall was not used: err=%v soul=%+v memory=%+v", err, backend.soulRecallCalls, backend.recallCalls)
	}
	if backend.soulRecallCalls[0].budget >= manifest.Recall.BudgetTokens || backend.recallCalls[0].budget >= manifest.Recall.BudgetTokens {
		t.Fatalf("recall budgets were not separated: soul=%d memory=%d", backend.soulRecallCalls[0].budget, backend.recallCalls[0].budget)
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
