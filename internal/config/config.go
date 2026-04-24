// Package config provides application configuration loading and validation.
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
	DecayRates        map[string]float64 `yaml:"decay_rates"`
	ArchiveThresholds map[string]float64 `yaml:"archive_thresholds"`
	OverlapCache      OverlapCacheConfig `yaml:"overlap_cache"`
	Extraction        ExtractionConfig   `yaml:"extraction"`
	MCP               MCPConfig          `yaml:"mcp"`
	HNSW              HNSWConfig         `yaml:"hnsw"`
	Metrics           MetricsConfig      `yaml:"metrics"`
	Webhooks          WebhooksConfig     `yaml:"webhooks"`
	Recall            RecallConfig       `yaml:"recall"`
	Soul              SoulConfig         `yaml:"soul"`
}

// SoulConfig configures the optional SOUL identity subsystem.
// When Enabled is false, MIRA runs without identity features (8 tools only).
// All sub-fields are optional and use SOUL defaults when omitted.
type SoulConfig struct {
	Enabled bool `yaml:"enabled"`

	// Extraction controls how identity traits are extracted from conversations.
	Extraction SoulExtractionConfig `yaml:"extraction"`

	// Recall controls the identity prompt generation budget and behaviour.
	Recall SoulRecallConfig `yaml:"recall"`

	// DriftDetection configures identity drift monitoring.
	DriftDetection SoulDriftDetectionConfig `yaml:"drift_detection"`

	// ModelSwap controls automatic reinforcement after a model change.
	ModelSwap SoulModelSwapConfig `yaml:"model_swap"`

	// Evolution controls versioned identity history tracking.
	Evolution SoulEvolutionConfig `yaml:"evolution"`
}

// SoulExtractionConfig mirrors SOUL extraction settings.
type SoulExtractionConfig struct {
	MinTraitConfidence      float64 `yaml:"min_trait_confidence"`
	MinObservationsForTrait int     `yaml:"min_observations_for_trait"`
}

// SoulRecallConfig mirrors SOUL recall settings.
type SoulRecallConfig struct {
	DefaultBudgetTokens int `yaml:"default_budget_tokens"`
}

// SoulDriftDetectionConfig mirrors SOUL drift detection settings.
type SoulDriftDetectionConfig struct {
	Threshold             float64 `yaml:"threshold"`
	WindowSize            int     `yaml:"window_size"`
	AutoCheckAfterCapture bool    `yaml:"auto_check_after_capture"`
}

// SoulModelSwapConfig mirrors SOUL model-swap settings.
type SoulModelSwapConfig struct {
	AutoReinforce bool `yaml:"auto_reinforce"`
}

// SoulEvolutionConfig mirrors SOUL evolution settings.
type SoulEvolutionConfig struct {
	Enabled            bool `yaml:"enabled"`
	MaxHistoryVersions int  `yaml:"max_history_versions"`
}

type QueryExpansionConfig struct {
	Enabled     bool    `yaml:"enabled"`
	NumVariants int     `yaml:"num_variants"`
	Temperature float64 `yaml:"temperature"`
}

type SearchTimeClusteringConfig struct {
	Enabled             bool    `yaml:"enabled"`
	SimilarityThreshold float64 `yaml:"similarity_threshold"`
}

type RerankerConfig struct {
	Enabled bool `yaml:"enabled"`
	TopK    int  `yaml:"top_k"`
}

type RecallConfig struct {
	AdaptiveThresholdMethod  string                     `yaml:"adaptive_threshold_method"`
	AdaptiveThresholdFloor   float64                    `yaml:"adaptive_threshold_floor"`
	AdaptiveThresholdCeiling float64                    `yaml:"adaptive_threshold_ceiling"`
	EnableFTS5               bool                       `yaml:"enable_fts5"`
	FTS5Limit                int                        `yaml:"fts5_limit"`
	RRFK                     int                        `yaml:"rrf_k"`
	QueryExpansion           QueryExpansionConfig       `yaml:"query_expansion"`
	SearchTimeClustering     SearchTimeClusteringConfig `yaml:"search_time_clustering"`
	Reranker                 RerankerConfig             `yaml:"reranker"`
}

type HNSWConfig struct {
	M              int     `yaml:"M"`
	Ml             float64 `yaml:"Ml"`
	EfConstruction int     `yaml:"ef_construction"`
	EfSearch       int     `yaml:"ef_search"`
}

type SystemConfig struct {
	Version string `yaml:"version"`
}

type SQLiteSettingsConfig struct {
	JournalMode string `yaml:"journal_mode"`
	Synchronous string `yaml:"synchronous"`
	CacheSize   int    `yaml:"cache_size"`
	MmapSize    int    `yaml:"mmap_size"`
	TempStore   string `yaml:"temp_store"`
}

type StorageConfig struct {
	Path   string               `yaml:"path"`
	SQLite SQLiteSettingsConfig `yaml:"sqlite"`
}

type EmbeddingsConfig struct {
	CurrentModel      string `yaml:"current_model"`
	ModelHash         string `yaml:"model_hash"`
	Dimension         int    `yaml:"dimension"`
	BatchSize         int    `yaml:"batch_size"`
	CacheSize         int    `yaml:"cache_size"`
	UseSimpleEmbedder bool   `yaml:"use_simple_embedder,omitempty"` // For testing
}

type AllocatorConfig struct {
	DefaultBudget         int                  `yaml:"default_budget"`
	MaxCandidates         int                  `yaml:"max_candidates"`
	EarlyPruningThreshold float64              `yaml:"early_pruning_threshold"`
	SessionWindowSeconds  int                  `yaml:"session_window_seconds"`
	SessionBoostBeta      float64              `yaml:"session_boost_beta"`
	SessionBoostMax       float64              `yaml:"session_boost_max"`
	CausalPenaltyAlpha    float64              `yaml:"causal_penalty_alpha"`
	DensitySigmoid        DensitySigmoidConfig `yaml:"density_sigmoid"`
}

type DensitySigmoidConfig struct {
	K  float64 `yaml:"k"`
	Mu float64 `yaml:"mu"`
}

type ExtractionConfig struct {
	MinEntityLength int `yaml:"min_entity_length"`
	CausalLookback  int `yaml:"causal_lookback"`
	CausalMaxDays   int `yaml:"causal_max_days"`
}

// OverlapCacheConfig configures the overlap cache for CBA
type OverlapCacheConfig struct {
	TTLDays    int `yaml:"ttl_days"`
	MaxEntries int `yaml:"max_entries"`
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
			Version:        "0.4.5",
		},
		Storage: StorageConfig{
			Path: ".mira",
			SQLite: SQLiteSettingsConfig{
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
		Allocator: AllocatorConfig{
			DefaultBudget:         4000,
			MaxCandidates:         100,
			EarlyPruningThreshold: 0.6,
			SessionWindowSeconds:  7200,
			SessionBoostBeta:      0.2,
			SessionBoostMax:       1.2,
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
		Extraction: ExtractionConfig{
			MinEntityLength: 2,
			CausalLookback:  50,
			CausalMaxDays:   30,
		},
		OverlapCache: OverlapCacheConfig{
			TTLDays:    30,
			MaxEntries: 1000000,
		},
		MCP: MCPConfig{
			Name:           "mira",
			Version:        "0.4.5",
			Transport:      "stdio",
			TimeoutSeconds: 30,
		},
		// HNSW configuration - vector search index
		// Note: EfConstruction is not supported by the underlying hnsw library (coder/hnsw v0.4.0).
		// Only M, Ml and EfSearch are effective.
		HNSW: HNSWConfig{
			M:              32,
			Ml:             0.25,
			EfConstruction: 0,
			EfSearch:       100,
		},
		// Metrics configuration - monitoring
		Metrics: MetricsConfig{
			Enabled:        true,
			PrometheusAddr: ":9090",
			ReportInterval: 60,
		},
		// Webhooks configuration - external notifications
		Webhooks: WebhooksConfig{
			Enabled:   false,
			Workers:   3,
			QueueSize: 1000,
			Timeout:   30,
		},
		Recall: RecallConfig{
			AdaptiveThresholdMethod:  "iqr",
			AdaptiveThresholdFloor:   0.15,
			AdaptiveThresholdCeiling: 0.75,
			EnableFTS5:               true,
			FTS5Limit:                100,
			RRFK:                     60,
			QueryExpansion: QueryExpansionConfig{
				Enabled:     true,
				NumVariants: 3,
				Temperature: 0.3,
			},
			SearchTimeClustering: SearchTimeClusteringConfig{
				Enabled:             true,
				SimilarityThreshold: 0.88,
			},
			Reranker: RerankerConfig{
				Enabled: false,
				TopK:    30,
			},
		},
		Soul: SoulConfig{
			Enabled: false,
			Extraction: SoulExtractionConfig{
				MinTraitConfidence:      0.3,
				MinObservationsForTrait: 5,
			},
			Recall: SoulRecallConfig{
				DefaultBudgetTokens: 1000,
			},
			DriftDetection: SoulDriftDetectionConfig{
				Threshold:             0.3,
				WindowSize:            10,
				AutoCheckAfterCapture: true,
			},
			ModelSwap: SoulModelSwapConfig{
				AutoReinforce: true,
			},
			Evolution: SoulEvolutionConfig{
				Enabled:            true,
				MaxHistoryVersions: 100,
			},
		},
	}
}

// Load loads configuration from file and validates it
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Validate and apply defaults
	if err := cfg.Validate(); err != nil {
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

// MetricsConfig configures metrics export
type MetricsConfig struct {
	Enabled        bool   `yaml:"enabled"`
	PrometheusAddr string `yaml:"prometheus_addr"`
	ReportInterval int    `yaml:"report_interval_seconds"`
}

// WebhooksConfig configures webhook notifications
type WebhooksConfig struct {
	Enabled   bool     `yaml:"enabled"`
	Workers   int      `yaml:"workers"`
	QueueSize int      `yaml:"queue_size"`
	Timeout   int      `yaml:"timeout_seconds"`
	Endpoints []string `yaml:"endpoints,omitempty"`
}

// Validate checks if the configuration is valid and applies defaults for invalid values
func (c *Config) Validate() error {
	// Storage validation
	if dataPath := os.Getenv("MIRA_DATA_PATH"); dataPath != "" {
		c.Storage.Path = dataPath
	}
	if c.Storage.Path == "" {
		c.Storage.Path = ".mira"
	}

	// Embeddings validation
	if c.Embeddings.Dimension <= 0 {
		c.Embeddings.Dimension = 384
	}
	if c.Embeddings.BatchSize <= 0 {
		c.Embeddings.BatchSize = 32
	}
	if c.Embeddings.CacheSize <= 0 {
		c.Embeddings.CacheSize = 1000
	}
	if c.Embeddings.CurrentModel == "" {
		c.Embeddings.CurrentModel = "sentence-transformers/all-MiniLM-L6-v2"
	}

	// Allocator validation
	if c.Allocator.DefaultBudget <= 0 {
		c.Allocator.DefaultBudget = 4000
	}
	if c.Allocator.MaxCandidates <= 0 {
		c.Allocator.MaxCandidates = 100
	}
	if c.Allocator.EarlyPruningThreshold < 0 || c.Allocator.EarlyPruningThreshold > 1 {
		c.Allocator.EarlyPruningThreshold = 0.6
	}
	if c.Allocator.SessionWindowSeconds <= 0 {
		c.Allocator.SessionWindowSeconds = 7200
	}
	if c.Allocator.SessionBoostBeta < 0 {
		c.Allocator.SessionBoostBeta = 0.2
	}
	if c.Allocator.SessionBoostMax <= 1.0 {
		c.Allocator.SessionBoostMax = 1.2
	}
	if c.Allocator.SessionBoostMax > 1.5 {
		c.Allocator.SessionBoostMax = 1.5
	}
	if c.Allocator.CausalPenaltyAlpha < 0 {
		c.Allocator.CausalPenaltyAlpha = 0.15
	}
	// Decay rates defaults
	if c.DecayRates == nil {
		c.DecayRates = map[string]float64{
			"decision":     0.001,
			"fact":         0.005,
			"preference":   0.01,
			"session_note": 0.1,
			"debug_log":    0.5,
		}
	}
	if c.Allocator.DensitySigmoid.K <= 0 {
		c.Allocator.DensitySigmoid.K = 2.0
	}
	if c.Allocator.DensitySigmoid.Mu < 0 {
		c.Allocator.DensitySigmoid.Mu = 0.3
	}

	// HNSW validation
	if c.HNSW.M <= 0 {
		c.HNSW.M = 16
	}
	if c.HNSW.Ml <= 0 {
		c.HNSW.Ml = 0.25
	}
	// EfConstruction is ignored by the hnsw library, kept for backward compatibility only.
	if c.HNSW.EfConstruction < 0 {
		c.HNSW.EfConstruction = 0
	}
	if c.HNSW.EfSearch <= 0 {
		c.HNSW.EfSearch = 50
	}

	// Metrics validation
	if c.Metrics.Enabled {
		if c.Metrics.PrometheusAddr == "" {
			c.Metrics.PrometheusAddr = ":9090"
		}
		if c.Metrics.ReportInterval <= 0 {
			c.Metrics.ReportInterval = 60
		}
	}

	// Webhooks validation
	if c.Webhooks.Enabled {
		if c.Webhooks.Workers <= 0 {
			c.Webhooks.Workers = 3
		}
		if c.Webhooks.QueueSize <= 0 {
			c.Webhooks.QueueSize = 1000
		}
		if c.Webhooks.Timeout <= 0 {
			c.Webhooks.Timeout = 30
		}
	}

	// Extraction validation
	if c.Extraction.MinEntityLength <= 0 {
		c.Extraction.MinEntityLength = 2
	}
	if c.Extraction.CausalLookback <= 0 {
		c.Extraction.CausalLookback = 50
	}
	if c.Extraction.CausalMaxDays <= 0 {
		c.Extraction.CausalMaxDays = 30
	}

	// Recall validation
	if c.Recall.AdaptiveThresholdMethod == "" {
		c.Recall.AdaptiveThresholdMethod = "iqr"
	}
	if c.Recall.AdaptiveThresholdFloor < 0 {
		c.Recall.AdaptiveThresholdFloor = 0.15
	}
	if c.Recall.AdaptiveThresholdCeiling <= 0 {
		c.Recall.AdaptiveThresholdCeiling = 0.75
	}
	if c.Recall.FTS5Limit <= 0 {
		c.Recall.FTS5Limit = 100
	}
	if c.Recall.RRFK <= 0 {
		c.Recall.RRFK = 60
	}
	if c.Recall.QueryExpansion.NumVariants <= 0 {
		c.Recall.QueryExpansion.NumVariants = 3
	}
	if c.Recall.SearchTimeClustering.SimilarityThreshold <= 0 {
		c.Recall.SearchTimeClustering.SimilarityThreshold = 0.88
	}
	if c.Recall.Reranker.TopK <= 0 {
		c.Recall.Reranker.TopK = 30
	}

	// Overlap cache validation
	if c.OverlapCache.TTLDays <= 0 {
		c.OverlapCache.TTLDays = 30
	}
	if c.OverlapCache.MaxEntries <= 0 {
		c.OverlapCache.MaxEntries = 1000000
	}

	// Archive thresholds validation
	if c.ArchiveThresholds == nil {
		c.ArchiveThresholds = map[string]float64{
			"session_note": 30,
			"debug_log":    7,
		}
	}

	// MCP validation
	if c.MCP.Name == "" {
		c.MCP.Name = "mira"
	}
	if c.MCP.Version == "" {
		c.MCP.Version = "0.4.4"
	}
	if c.MCP.Transport == "" {
		c.MCP.Transport = "stdio"
	}
	if c.MCP.TimeoutSeconds <= 0 {
		c.MCP.TimeoutSeconds = 30
	}

	// Embeddings validation - ModelHash
	if c.Embeddings.ModelHash == "" {
		c.Embeddings.ModelHash = "a2d8f3e9"
	}

	// Storage SQLite validation
	if c.Storage.SQLite.JournalMode == "" {
		c.Storage.SQLite.JournalMode = "WAL"
	}
	if c.Storage.SQLite.Synchronous == "" {
		c.Storage.SQLite.Synchronous = "NORMAL"
	}
	if c.Storage.SQLite.CacheSize == 0 {
		c.Storage.SQLite.CacheSize = -64000
	}
	if c.Storage.SQLite.MmapSize <= 0 {
		c.Storage.SQLite.MmapSize = 268435456
	}
	if c.Storage.SQLite.TempStore == "" {
		c.Storage.SQLite.TempStore = "MEMORY"
	}

	// Soul validation — apply defaults for zero values so embedded SOUL
	// behaves identically to standalone SOUL when settings are omitted.
	if c.Soul.Extraction.MinTraitConfidence <= 0 {
		c.Soul.Extraction.MinTraitConfidence = 0.3
	}
	if c.Soul.Extraction.MinObservationsForTrait <= 0 {
		c.Soul.Extraction.MinObservationsForTrait = 5
	}
	if c.Soul.Recall.DefaultBudgetTokens <= 0 {
		c.Soul.Recall.DefaultBudgetTokens = 1000
	}
	if c.Soul.DriftDetection.Threshold <= 0 {
		c.Soul.DriftDetection.Threshold = 0.3
	}
	if c.Soul.DriftDetection.WindowSize <= 0 {
		c.Soul.DriftDetection.WindowSize = 10
	}
	if c.Soul.Evolution.MaxHistoryVersions <= 0 {
		c.Soul.Evolution.MaxHistoryVersions = 100
	}

	return nil
}
