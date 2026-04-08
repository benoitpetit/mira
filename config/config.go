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
	VectorIndex       VectorIndexConfig  `yaml:"vector_index"`
	Allocator         AllocatorConfig    `yaml:"allocator"`
	DecayRates        map[string]float64 `yaml:"decay_rates"`
	ArchiveThresholds map[string]float64 `yaml:"archive_thresholds"`
	OverlapCache      OverlapCacheConfig `yaml:"overlap_cache"`
	Extraction        ExtractionConfig   `yaml:"extraction"`
	MCP               MCPConfig          `yaml:"mcp"`
}

type SystemConfig struct {
	Version              string `yaml:"version"`
	MaxConcurrentQueries int    `yaml:"max_concurrent_queries"`
}

type StorageConfig struct {
	Path   string       `yaml:"path"`
	SQLite SQLiteConfig `yaml:"sqlite"`
}

type SQLiteConfig struct {
	JournalMode string `yaml:"journal_mode"`
	Synchronous string `yaml:"synchronous"`
	CacheSize   int    `yaml:"cache_size"`
	MmapSize    int    `yaml:"mmap_size"`
	TempStore   string `yaml:"temp_store"`
}

type EmbeddingsConfig struct {
	CurrentModel string `yaml:"current_model"`
	ModelHash    string `yaml:"model_hash"`
	Dimension    int    `yaml:"dimension"`
	BatchSize    int    `yaml:"batch_size"`
	CacheSize    int    `yaml:"cache_size"`
}

type VectorIndexConfig struct {
	Type           string `yaml:"type"`
	Path           string `yaml:"path"`
	EfConstruction int    `yaml:"ef_construction"`
	EfSearch       int    `yaml:"ef_search"`
	M              int    `yaml:"M"`
	MaxElements    int    `yaml:"max_elements"`
}

type AllocatorConfig struct {
	DefaultBudget         int                `yaml:"default_budget"`
	MaxCandidates         int                `yaml:"max_candidates"`
	EarlyPruningThreshold float64            `yaml:"early_pruning_threshold"`
	SessionWindowSeconds  int                `yaml:"session_window_seconds"`
	SessionBoostBeta      float64            `yaml:"session_boost_beta"`
	CausalPenaltyAlpha    float64            `yaml:"causal_penalty_alpha"`
	DensitySigmoid        DensitySigmoidConfig `yaml:"density_sigmoid"`
}

type DensitySigmoidConfig struct {
	K  float64 `yaml:"k"`
	Mu float64 `yaml:"mu"`
}

type OverlapCacheConfig struct {
	TTLDays    int `yaml:"ttl_days"`
	MaxEntries int `yaml:"max_entries"`
}

type ExtractionConfig struct {
	MaxVerbatimSize   int `yaml:"max_verbatim_size"`
	MaxSentenceLength int `yaml:"max_sentence_length"`
	MinEntityLength   int `yaml:"min_entity_length"`
	CausalLookback    int `yaml:"causal_lookback"`
	CausalMaxDays     int `yaml:"causal_max_days"`
}

type MCPConfig struct {
	Name           string `yaml:"name"`
	Version        string `yaml:"version"`
	Transport      string `yaml:"transport"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

// Default returns default configuration
func Default() *Config {
	return &Config{
		System: SystemConfig{
			Version:        "0.1.1",
			MaxConcurrentQueries: 10,
		},
		Storage: StorageConfig{
			Path: "./mira_data",
			SQLite: SQLiteConfig{
				JournalMode: "WAL",
				Synchronous: "NORMAL",
				CacheSize:   -64000,
				MmapSize:    268435456,
				TempStore:   "MEMORY",
			},
		},
		Embeddings: EmbeddingsConfig{
			CurrentModel: "sentence-transformers/all-MiniLM-L6-v2",
			ModelHash:    "a2d8f3e9",
			Dimension:    384,
			BatchSize:    32,
			CacheSize:    1000,
		},
		VectorIndex: VectorIndexConfig{
			Type:           "hnswlib",
			Path:           "./mira_data/vectors.bin",
			EfConstruction: 200,
			EfSearch:       50,
			M:              16,
			MaxElements:    100000,
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
		DecayRates: map[string]float64{
			"decision":     0.001,
			"fact":         0.005,
			"preference":   0.01,
			"session_note": 0.1,
			"debug_log":    0.5,
		},
		ArchiveThresholds: map[string]float64{
			"session_note": 30,
			"debug_log":    7,
		},
		OverlapCache: OverlapCacheConfig{
			TTLDays:    30,
			MaxEntries: 1000000,
		},
		Extraction: ExtractionConfig{
			MaxVerbatimSize:   65536,
			MaxSentenceLength: 500,
			MinEntityLength:   2,
			CausalLookback:    50,
			CausalMaxDays:     30,
		},
		MCP: MCPConfig{
			Name:           "mira",
			Version:        "0.1.1",
			Transport:      "stdio",
			TimeoutSeconds: 30,
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
