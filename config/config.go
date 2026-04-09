package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents complete configuration
type Config struct {
	System            SystemConfig       `yaml:"system"`
	Storage           StorageConfig      `yaml:"storage"`
	Embeddings        EmbeddingsConfig   `yaml:"embeddings"`
	Allocator         AllocatorConfig    `yaml:"allocator"`
	ArchiveThresholds map[string]float64 `yaml:"archive_thresholds"`
	Extraction        ExtractionConfig   `yaml:"extraction"`
	MCP               MCPConfig          `yaml:"mcp"`
}

type SystemConfig struct {
	Version string `yaml:"version"`
}

type StorageConfig struct {
	Path string `yaml:"path"`
}

type EmbeddingsConfig struct {
	CurrentModel string `yaml:"current_model"`
	Dimension    int    `yaml:"dimension"`
	BatchSize    int    `yaml:"batch_size"`
	CacheSize    int    `yaml:"cache_size"`
}

type AllocatorConfig struct {
	DefaultBudget         int                  `yaml:"default_budget"`
	MaxCandidates         int                  `yaml:"max_candidates"`
	EarlyPruningThreshold float64              `yaml:"early_pruning_threshold"`
	SessionWindowSeconds  int                  `yaml:"session_window_seconds"`
	SessionBoostBeta      float64              `yaml:"session_boost_beta"`
	CausalPenaltyAlpha    float64              `yaml:"causal_penalty_alpha"`
	DensitySigmoid        DensitySigmoidConfig `yaml:"density_sigmoid"`
}

type DensitySigmoidConfig struct {
	K  float64 `yaml:"k"`
	Mu float64 `yaml:"mu"`
}

type ExtractionConfig struct {
	MinEntityLength int `yaml:"min_entity_length"`
}

type MCPConfig struct {
	Name      string `yaml:"name"`
	Version   string `yaml:"version"`
	Transport string `yaml:"transport"`
}

// Default returns default configuration
func Default() *Config {
	return &Config{
		System: SystemConfig{
			Version:        "0.1.2",
		},
		Storage: StorageConfig{
			Path: "./mira_data",
		},
		Embeddings: EmbeddingsConfig{
			CurrentModel: "sentence-transformers/all-MiniLM-L6-v2",
			Dimension:    384,
			BatchSize:    32,
			CacheSize:    1000,
		},
		Allocator: AllocatorConfig{
			DefaultBudget:         4000,
			MaxCandidates:         100,
			EarlyPruningThreshold: 0.6,
			SessionWindowSeconds:  7200,
			SessionBoostBeta:      0.2,
			CausalPenaltyAlpha:    0.15,
			DensitySigmoid: DensitySigmoidConfig{
				K:  2.0,
				Mu: 0.3,
			},
		},
		ArchiveThresholds: map[string]float64{
			"session_note": 30,
			"debug_log":    7,
		},
		Extraction: ExtractionConfig{
			MinEntityLength: 2,
		},
		MCP: MCPConfig{
			Name:      "mira",
			Version:        "0.1.2",
			Transport: "stdio",
		},
	}
}

// Load loads configuration from file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Save saves configuration to file
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
