package agentmemory

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/benoitpetit/mira/internal/adapters/storage"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	repo, err := storage.NewSQLiteRepository(filepath.Join(t.TempDir(), "mira.db"), storage.DefaultSQLiteOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo.DB()
}

func TestCaptureUsesConversationWhenAgentResponsesAreAbsent(t *testing.T) {
	runtime, err := NewRuntime(testDB(t), DefaultConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := runtime.Capture(context.Background(), CaptureRequest{AgentID: "agent", Conversation: "I prefer concise and direct answers. Please be transparent."})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != 1 || len(snapshot.PersonalityTraits) == 0 {
		t.Fatalf("capture did not extract identity: version=%d traits=%d", snapshot.Version, len(snapshot.PersonalityTraits))
	}
}

func TestCapturePersistsSessionIDAndRecallWaitsForStableTraits(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MinObservationsForTrait = 2
	runtime, err := NewRuntime(testDB(t), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := runtime.Capture(context.Background(), CaptureRequest{AgentID: "agent", SessionID: "session-a", Conversation: "Please be transparent and direct."})
	if err != nil {
		t.Fatal(err)
	}
	if first.SessionID != "session-a" {
		t.Fatalf("session id was not persisted: %q", first.SessionID)
	}
	prompt, err := runtime.Recall(context.Background(), "agent", "", 200)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt.Content, "transparent (confidence") {
		t.Fatal("a single observation was exposed as a stable trait")
	}
	if _, err := runtime.Capture(context.Background(), CaptureRequest{AgentID: "agent", SessionID: "session-b", Conversation: "Please be transparent and direct."}); err != nil {
		t.Fatal(err)
	}
	second, err := runtime.Latest(context.Background(), "agent")
	if err != nil {
		t.Fatal(err)
	}
	if second.SessionID != "session-b" {
		t.Fatalf("latest session id was not updated: %q", second.SessionID)
	}
	prompt, err = runtime.Recall(context.Background(), "agent", "", 200)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt.Content, "transparent (confidence") {
		t.Fatal("repeated observation was not exposed as stable trait")
	}
}

func TestRecallNeverExceedsConfiguredBudget(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxBudgetTokens = 120
	cfg.DefaultBudgetTokens = 80
	runtime, err := NewRuntime(testDB(t), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.Capture(context.Background(), CaptureRequest{AgentID: "agent", Conversation: "I prefer clear, direct, technical answers with a structured explanation and careful uncertainty."})
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := runtime.Recall(context.Background(), "agent", "", 10000)
	if err != nil {
		t.Fatal(err)
	}
	if prompt.BudgetTokens != 120 || prompt.TokenEstimate > 120 {
		t.Fatalf("budget mismatch: %+v", prompt)
	}
}

func TestPatchKeepsSnapshotsImmutable(t *testing.T) {
	runtime, err := NewRuntime(testDB(t), DefaultConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := runtime.Capture(context.Background(), CaptureRequest{AgentID: "agent", Conversation: "I prefer clear answers."})
	if err != nil {
		t.Fatal(err)
	}
	second, result, err := runtime.Patch(context.Background(), "agent", map[string]interface{}{"enthusiasm_level": 0.9}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.NewVersion != 2 || second.DerivedFromID == nil || *second.DerivedFromID != first.ID {
		t.Fatalf("invalid immutable lineage: %+v", result)
	}
	history, err := runtime.History(context.Background(), "agent", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[1].ID != first.ID || history[1].VoiceProfile.EnthusiasmLevel == second.VoiceProfile.EnthusiasmLevel {
		t.Fatal("previous snapshot was modified")
	}
}

func TestUpdatesAndPatchesStartAtVersionOne(t *testing.T) {
	runtime, err := NewRuntime(testDB(t), DefaultConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, result, err := runtime.Update(context.Background(), "new-agent", "be more concise", "test")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 1 || result.NewVersion != 1 || updated.DerivedFromID != nil {
		t.Fatalf("new identity update has invalid lineage: snapshot=%+v result=%+v", updated, result)
	}
	history, err := runtime.History(context.Background(), "new-agent", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("expected one initial snapshot, got %d", len(history))
	}
}

func TestModelSwapCreatesInternalizedIdentitySnapshot(t *testing.T) {
	runtime, err := NewRuntime(testDB(t), DefaultConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := runtime.Capture(context.Background(), CaptureRequest{AgentID: "agent", ModelID: "model-a", Conversation: "I prefer clear answers."})
	if err != nil {
		t.Fatal(err)
	}
	swap, prompt, err := runtime.HandleSwap(context.Background(), "agent", "model-a", "model-b")
	if err != nil {
		t.Fatal(err)
	}
	if !swap.ReinforcementApplied || prompt.SnapshotVersion != 2 {
		t.Fatalf("model swap did not reinforce identity: swap=%+v prompt=%+v", swap, prompt)
	}
	latest, err := runtime.Latest(context.Background(), "agent")
	if err != nil {
		t.Fatal(err)
	}
	if latest.ModelIdentifier != "model-b" || latest.DerivedFromID == nil || *latest.DerivedFromID != first.ID {
		t.Fatalf("model swap lost snapshot lineage: %+v", latest)
	}
}

func TestModelSwapHonorsAutoReinforceAndModelLineage(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoReinforce = false
	runtime, err := NewRuntime(testDB(t), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Capture(context.Background(), CaptureRequest{AgentID: "agent", ModelID: "model-a", Conversation: "I prefer clear answers."}); err != nil {
		t.Fatal(err)
	}
	swap, prompt, err := runtime.HandleSwap(context.Background(), "agent", "model-a", "model-b")
	if err != nil {
		t.Fatal(err)
	}
	if swap.ReinforcementApplied || prompt != nil {
		t.Fatalf("reinforcement was applied while disabled: swap=%+v prompt=%+v", swap, prompt)
	}
	latest, err := runtime.Latest(context.Background(), "agent")
	if err != nil {
		t.Fatal(err)
	}
	if latest.ModelIdentifier != "model-a" || latest.Version != 1 {
		t.Fatalf("disabled reinforcement changed identity: %+v", latest)
	}
	if _, _, err := runtime.HandleSwap(context.Background(), "agent", "wrong-model", "model-c"); err == nil {
		t.Fatal("model transition accepted a stale from_model")
	}
}

func TestEvolutionDisabledRejectsNewVersionsButRecordsSwap(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EvolutionEnabled = false
	runtime, err := NewRuntime(testDB(t), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Capture(context.Background(), CaptureRequest{AgentID: "agent", ModelID: "model-a", Conversation: "I prefer clear answers."}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Capture(context.Background(), CaptureRequest{AgentID: "agent", ModelID: "model-a", Conversation: "I prefer direct answers."}); err == nil {
		t.Fatal("capture created a version while evolution was disabled")
	}
	if _, _, err := runtime.Update(context.Background(), "agent", "be concise", "test"); err == nil {
		t.Fatal("update created a version while evolution was disabled")
	}
	swap, prompt, err := runtime.HandleSwap(context.Background(), "agent", "model-a", "model-b")
	if err != nil {
		t.Fatal(err)
	}
	if !swap.IdentityPreserved || swap.ReinforcementApplied || prompt != nil {
		t.Fatalf("disabled evolution produced reinforcement: swap=%+v prompt=%+v", swap, prompt)
	}
}

func TestAgentMemoryUsesSharedEncryptedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "encrypted-mira.db")
	key := "mira-agent-memory-test-key"
	repo, err := storage.NewSQLiteRepository(path, storage.SQLiteOptions{EncryptionKey: key})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := NewRuntime(repo.DB(), DefaultConfig(), nil)
	if err != nil {
		_ = repo.Close()
		t.Fatal(err)
	}
	if _, err := runtime.Capture(context.Background(), CaptureRequest{AgentID: "encrypted-agent", Conversation: "I prefer precise answers."}); err != nil {
		_ = repo.Close()
		t.Fatal(err)
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := storage.NewSQLiteRepository(path, storage.SQLiteOptions{EncryptionKey: key})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedRuntime, err := NewRuntime(reopened.DB(), DefaultConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := reopenedRuntime.Latest(context.Background(), "encrypted-agent")
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.AgentID != "encrypted-agent" {
		t.Fatalf("identity was not recovered from encrypted shared database: %+v", latest)
	}
}

func TestControllerUsesSoulNamesAndRejectsBareNames(t *testing.T) {
	runtime, err := NewRuntime(testDB(t), DefaultConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range NewController(runtime).ToolDefinitions() {
		if len(tool.Name) < len(ToolPrefix) || tool.Name[:len(ToolPrefix)] != ToolPrefix {
			t.Fatalf("unexpected public tool %q", tool.Name)
		}
	}
	if _, err := NewController(runtime).Call(context.Background(), "capture", map[string]interface{}{}); err == nil || !strings.Contains(err.Error(), "unknown soul tool") {
		t.Fatalf("bare tool name was accepted: %v", err)
	}
}
