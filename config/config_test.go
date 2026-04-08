package config

import (
	"os"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.System.Version != "0.1.0" {
		t.Errorf("Expected version 0.1.0, got %s", cfg.System.Version)
	}
	if cfg.Allocator.DefaultBudget != 4000 {
		t.Errorf("Expected default budget 4000, got %d", cfg.Allocator.DefaultBudget)
	}
	if cfg.Embeddings.Dimension != 384 {
		t.Errorf("Expected dimension 384, got %d", cfg.Embeddings.Dimension)
	}
	if cfg.Storage.SQLite.JournalMode != "WAL" {
		t.Errorf("Expected WAL journal mode, got %s", cfg.Storage.SQLite.JournalMode)
	}
}

func TestLoadAndSave(t *testing.T) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Save default config
	cfg := Default()
	if err := cfg.Save(tmpFile.Name()); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Load it back
	loaded, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify
	if loaded.System.Version != cfg.System.Version {
		t.Error("Version mismatch after load")
	}
	if loaded.Allocator.DefaultBudget != cfg.Allocator.DefaultBudget {
		t.Error("DefaultBudget mismatch")
	}
	if loaded.Embeddings.Dimension != cfg.Embeddings.Dimension {
		t.Error("Dimension mismatch")
	}
}

func TestLoadNonExistent(t *testing.T) {
	_, err := Load("/non/existent/path/config.yaml")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestDecayRates(t *testing.T) {
	cfg := Default()

	tests := []struct {
		key      string
		expected float64
	}{
		{"decision", 0.001},
		{"fact", 0.005},
		{"preference", 0.01},
		{"session_note", 0.1},
		{"debug_log", 0.5},
	}

	for _, tt := range tests {
		if val, ok := cfg.DecayRates[tt.key]; !ok {
			t.Errorf("Missing decay rate for %s", tt.key)
		} else if val != tt.expected {
			t.Errorf("Decay rate for %s: got %v, want %v", tt.key, val, tt.expected)
		}
	}
}

func TestAllocatorConfig(t *testing.T) {
	cfg := Default()

	if cfg.Allocator.MaxCandidates != 100 {
		t.Errorf("Expected MaxCandidates 100, got %d", cfg.Allocator.MaxCandidates)
	}
	if cfg.Allocator.EarlyPruningThreshold != 0.6 {
		t.Errorf("Expected threshold 0.6, got %f", cfg.Allocator.EarlyPruningThreshold)
	}
	if cfg.Allocator.SessionBoostBeta != 0.2 {
		t.Errorf("Expected beta 0.2, got %f", cfg.Allocator.SessionBoostBeta)
	}
	if cfg.Allocator.CausalPenaltyAlpha != 0.15 {
		t.Errorf("Expected alpha 0.15, got %f", cfg.Allocator.CausalPenaltyAlpha)
	}
	if cfg.Allocator.DensitySigmoid.K != 2.0 {
		t.Errorf("Expected K 2.0, got %f", cfg.Allocator.DensitySigmoid.K)
	}
	if cfg.Allocator.DensitySigmoid.Mu != 0.3 {
		t.Errorf("Expected Mu 0.3, got %f", cfg.Allocator.DensitySigmoid.Mu)
	}
}
