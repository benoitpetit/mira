package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := Default()

	if cfg.Storage.Path == "" {
		t.Error("Storage.Path should not be empty")
	}

	if cfg.Embeddings.Dimension <= 0 {
		t.Error("Embeddings.Dimension should be positive")
	}

	if cfg.Allocator.DefaultBudget <= 0 {
		t.Error("Allocator.DefaultBudget should be positive")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg:  Default(),
			wantErr: false,
		},
		{
			name: "zero values get defaults",
			cfg: &Config{
				Storage:    StorageConfig{},
				Embeddings: EmbeddingsConfig{},
				Allocator:  AllocatorConfig{},
			},
			wantErr: false,
		},
		{
			name: "negative values get fixed",
			cfg: &Config{
				Embeddings: EmbeddingsConfig{
					Dimension: -1,
					BatchSize: -10,
				},
				Allocator: AllocatorConfig{
					DefaultBudget: -100,
					MaxCandidates: -5,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Check that defaults were applied
			if tt.cfg.Embeddings.Dimension <= 0 {
				t.Error("Dimension should have been set to default")
			}
			if tt.cfg.Allocator.DefaultBudget <= 0 {
				t.Error("DefaultBudget should have been set to default")
			}
		})
	}
}

func TestValidateAppliesDefaults(t *testing.T) {
	cfg := &Config{
		Storage: StorageConfig{Path: ""},
		Embeddings: EmbeddingsConfig{
			Dimension: 0,
			BatchSize: 0,
		},
		Allocator: AllocatorConfig{
			DefaultBudget: 0,
			MaxCandidates: 0,
		},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Validate() should not return error: %v", err)
	}

	// Check defaults were applied
	if cfg.Storage.Path != ".mira" {
		t.Errorf("Storage.Path = %s, want .mira", cfg.Storage.Path)
	}
	if cfg.Embeddings.Dimension != 384 {
		t.Errorf("Embeddings.Dimension = %d, want 384", cfg.Embeddings.Dimension)
	}
	if cfg.Embeddings.BatchSize != 32 {
		t.Errorf("Embeddings.BatchSize = %d, want 32", cfg.Embeddings.BatchSize)
	}
	if cfg.Allocator.DefaultBudget != 4000 {
		t.Errorf("Allocator.DefaultBudget = %d, want 4000", cfg.Allocator.DefaultBudget)
	}
	if cfg.Allocator.MaxCandidates != 100 {
		t.Errorf("Allocator.MaxCandidates = %d, want 100", cfg.Allocator.MaxCandidates)
	}
}

// ── Load / Save ───────────────────────────────────────────────────────────────

func TestSaveAndLoad(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")

	cfg := Default()
	cfg.System.Version = "test-version"

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.System.Version != "test-version" {
		t.Errorf("loaded.System.Version = %q, want %q", loaded.System.Version, "test-version")
	}
}

func TestLoad_NotExist(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("Load() on missing file should return error")
	}
}

func TestLoad_BadYAML(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "bad.yaml")
	if err := os.WriteFile(path, []byte(": : : invalid yaml :::"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Error("Load() on bad YAML should return error")
	}
}

// ── LoadOrDefault ─────────────────────────────────────────────────────────────

func TestLoadOrDefault_MissingFile(t *testing.T) {
	cfg, err := LoadOrDefault("/definitely/not/here/config.yaml")
	if err != nil {
		t.Fatalf("LoadOrDefault() unexpected error = %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadOrDefault() returned nil config")
	}
	if cfg.Storage.Path == "" {
		t.Error("default config should have a storage path")
	}
}

func TestLoadOrDefault_ExistingFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")
	cfg := Default()
	cfg.System.Version = "from-file"
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadOrDefault(path)
	if err != nil {
		t.Fatalf("LoadOrDefault() error = %v", err)
	}
	if loaded.System.Version != "from-file" {
		t.Errorf("version = %q, want %q", loaded.System.Version, "from-file")
	}
}

func TestLoadOrDefault_EmptyPath(t *testing.T) {
	// Should resolve to a default path — likely won't exist, so returns Default().
	cfg, err := LoadOrDefault("")
	if err != nil {
		t.Fatalf("LoadOrDefault(\"\") error = %v", err)
	}
	if cfg == nil {
		t.Fatal("returned nil")
	}
}

func TestLoadOrDefault_MIRA_DATA_PATH(t *testing.T) {
	t.Setenv("MIRA_DATA_PATH", "/custom/data")
	cfg, err := LoadOrDefault("/nonexistent/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Storage.Path != "/custom/data" {
		t.Errorf("Storage.Path = %q, want %q", cfg.Storage.Path, "/custom/data")
	}
}

// ── ResolveConfigPath ─────────────────────────────────────────────────────────

func TestResolveConfigPath_Preferred(t *testing.T) {
	got := ResolveConfigPath("/my/config.yaml")
	if got != "/my/config.yaml" {
		t.Errorf("ResolveConfigPath(preferred) = %q, want %q", got, "/my/config.yaml")
	}
}

func TestResolveConfigPath_EnvVar(t *testing.T) {
	t.Setenv("MIRA_CONFIG", "/env/config.yaml")
	got := ResolveConfigPath("")
	if got != "/env/config.yaml" {
		t.Errorf("ResolveConfigPath(env) = %q, want %q", got, "/env/config.yaml")
	}
}

func TestResolveConfigPath_Default(t *testing.T) {
	t.Setenv("MIRA_CONFIG", "")
	got := ResolveConfigPath("")
	if got == "" {
		t.Error("ResolveConfigPath should return a non-empty path")
	}
}

// ── fallbackConfigDir ─────────────────────────────────────────────────────────

func TestFallbackConfigDir_HomeSet(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")
	got := fallbackConfigDir()
	want := filepath.Join("/home/testuser", ".config")
	if got != want {
		t.Errorf("fallbackConfigDir() = %q, want %q", got, want)
	}
}

func TestFallbackConfigDir_HomeEmpty(t *testing.T) {
	t.Setenv("HOME", "")
	// Also clear XDG-style env vars that UserConfigDir might use
	t.Setenv("XDG_CONFIG_HOME", "")
	got := fallbackConfigDir()
	if got != "." {
		// On Linux with no HOME, should return "."
		// (windows/darwin branches don't apply on this OS)
		want := "."
		t.Errorf("fallbackConfigDir() = %q, want %q", got, want)
	}
}

func TestFallbackConfigDir_UsedByResolveConfigPath(t *testing.T) {
	// When os.UserConfigDir succeeds, fallbackConfigDir is not called.
	// Force UserConfigDir to fail by clearing HOME and XDG_CONFIG_HOME.
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("MIRA_CONFIG", "")
	got := ResolveConfigPath("")
	// Should not be empty — fallbackConfigDir returns "." so path is "./mira/config.yaml"
	if got == "" {
		t.Error("ResolveConfigPath should return non-empty even when UserConfigDir fails")
	}
}
