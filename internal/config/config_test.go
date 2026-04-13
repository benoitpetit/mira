package config

import (
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
