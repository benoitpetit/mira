package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/benoitpetit/mira/internal/usecases/interactors"
	soul "github.com/benoitpetit/soul"
)

// ──────────────────────────────────────────────────────────────────────────────
// UC accessor methods (all were 0% coverage)
// ──────────────────────────────────────────────────────────────────────────────

func TestUCAccessors_ReturnExpectedValues(t *testing.T) {
	sm := &interactors.StoreMemory{}
	rm := &interactors.RecallMemory{}
	lm := &interactors.LoadMemory{}
	gt := &interactors.GetTimeline{}
	gs := &interactors.GetStatus{}
	gc := &interactors.GetCausalChain{}
	am := &interactors.ArchiveMemories{}
	cm := &interactors.ClearMemory{}
	dm := &interactors.DeleteMemory{}
	ss := &interactors.SearchSemantic{}
	um := &interactors.UpdateMemory{}
	cons := &interactors.ConsolidateMemories{}

	a := &Application{
		storeMemory:         sm,
		recallMemory:        rm,
		loadMemory:          lm,
		getTimeline:         gt,
		getStatus:           gs,
		getCausalChain:      gc,
		archiveMemories:     am,
		clearMemory:         cm,
		deleteMemory:        dm,
		searchSemantic:      ss,
		updateMemory:        um,
		consolidateMemories: cons,
		soulApp:             nil,
	}

	if a.StoreMemoryUC() != sm {
		t.Error("StoreMemoryUC mismatch")
	}
	if a.RecallMemoryUC() != rm {
		t.Error("RecallMemoryUC mismatch")
	}
	if a.LoadMemoryUC() != lm {
		t.Error("LoadMemoryUC mismatch")
	}
	if a.GetTimelineUC() != gt {
		t.Error("GetTimelineUC mismatch")
	}
	if a.GetStatusUC() != gs {
		t.Error("GetStatusUC mismatch")
	}
	if a.GetCausalChainUC() != gc {
		t.Error("GetCausalChainUC mismatch")
	}
	if a.ArchiveMemoriesUC() != am {
		t.Error("ArchiveMemoriesUC mismatch")
	}
	if a.ClearMemoryUC() != cm {
		t.Error("ClearMemoryUC mismatch")
	}
	if a.DeleteMemoryUC() != dm {
		t.Error("DeleteMemoryUC mismatch")
	}
	if a.SearchSemanticUC() != ss {
		t.Error("SearchSemanticUC mismatch")
	}
	if a.UpdateMemoryUC() != um {
		t.Error("UpdateMemoryUC mismatch")
	}
	if a.ConsolidateMemoriesUC() != cons {
		t.Error("ConsolidateMemoriesUC mismatch")
	}
	if a.SoulApplication() != nil {
		t.Error("SoulApplication should be nil")
	}
}

func TestSoulApplication_NonNil(t *testing.T) {
	soulMock := &soul.Application{}
	a := &Application{soulApp: soulMock}
	if a.SoulApplication() != soulMock {
		t.Error("SoulApplication should return the configured soul app")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// ensureGitignore
// ──────────────────────────────────────────────────────────────────────────────

func TestEnsureGitignore_NoGitignore(t *testing.T) {
	dir := t.TempDir()
	dataPath := filepath.Join(dir, ".mira")
	// No .gitignore in dir → should be a no-op
	if err := ensureGitignore(dataPath); err != nil {
		t.Fatalf("ensureGitignore with no .gitignore: %v", err)
	}
}

func TestEnsureGitignore_AlreadyContainsMira(t *testing.T) {
	dir := t.TempDir()
	gitignore := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("node_modules/\n.mira/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(dir, ".mira")
	if err := ensureGitignore(dataPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// File should be unchanged
	content, _ := os.ReadFile(gitignore)
	if string(content) != "node_modules/\n.mira/\n" {
		t.Errorf("file modified unexpectedly: %q", string(content))
	}
}

func TestEnsureGitignore_AppendsMira(t *testing.T) {
	dir := t.TempDir()
	gitignore := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(dir, ".mira")
	if err := ensureGitignore(dataPath); err != nil {
		t.Fatalf("ensureGitignore: %v", err)
	}
	content, _ := os.ReadFile(gitignore)
	if !containsStr(string(content), ".mira") {
		t.Errorf("expected .mira in gitignore after append, got: %q", string(content))
	}
}

func TestEnsureGitignore_NoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	gitignore := filepath.Join(dir, ".gitignore")
	// No trailing newline
	if err := os.WriteFile(gitignore, []byte("dist"), 0o644); err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(dir, ".mira")
	if err := ensureGitignore(dataPath); err != nil {
		t.Fatalf("ensureGitignore: %v", err)
	}
	content, _ := os.ReadFile(gitignore)
	if !containsStr(string(content), ".mira") {
		t.Errorf("expected .mira appended, got: %q", string(content))
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}())
}
