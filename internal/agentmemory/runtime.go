// Package agentmemory contains MIRA's built-in identity and continuity engine.
//
// It is deliberately part of MIRA's internal domain instead of being an
// extension wrapper. Identity snapshots share MIRA's database, encryption
// boundary, token estimator and memory-recall pipeline.
package agentmemory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/benoitpetit/mira/internal/usecases/interactors"
	"github.com/google/uuid"
)

// Config controls identity extraction, continuity recall and drift detection.
type Config struct {
	MinTraitConfidence      float64
	MinObservationsForTrait int
	DefaultBudgetTokens     int
	MaxBudgetTokens         int
	DriftThreshold          float64
	DriftWindowSize         int
	AutoCheckAfterCapture   bool
	AutoReinforce           bool
	EvolutionEnabled        bool
	MaxHistoryVersions      int
	EnrichWithMiraMemories  bool
	MaxMiraMemories         int
}

// DefaultConfig returns values aligned with MIRA's local/offline defaults.
func DefaultConfig() Config {
	return Config{
		MinTraitConfidence:      0.3,
		MinObservationsForTrait: 5,
		DefaultBudgetTokens:     1000,
		MaxBudgetTokens:         4000,
		DriftThreshold:          0.3,
		DriftWindowSize:         10,
		AutoCheckAfterCapture:   true,
		AutoReinforce:           true,
		EvolutionEnabled:        true,
		MaxHistoryVersions:      100,
		MaxMiraMemories:         5,
	}
}

// MemoryReference is the common boundary between identity memory and MIRA.
type MemoryReference struct {
	MemoryID   uuid.UUID
	Content    string
	MemoryType string
	Wing       string
	Room       string
	Relevance  float64
	Timestamp  time.Time
}

// MemoryProvider lets the identity engine use MIRA's normal memory pipeline.
type MemoryProvider interface {
	GetMiraMemories(ctx context.Context, agentID, query string, budget, limit int) ([]MemoryReference, error)
	LinkIdentityToMemory(ctx context.Context, identityID, memoryID uuid.UUID) error
	NotifyMiraOfIdentityChange(ctx context.Context, agentID, changeType string) error
}

// Snapshot is an immutable version of an agent's identity.
type Snapshot struct {
	ID                  uuid.UUID              `json:"id"`
	AgentID             string                 `json:"agent_id"`
	Version             int                    `json:"version"`
	CreatedAt           time.Time              `json:"created_at"`
	SessionID           string                 `json:"session_id,omitempty"`
	DerivedFromID       *uuid.UUID             `json:"derived_from_id,omitempty"`
	PersonalityTraits   []Trait                `json:"personality_traits"`
	VoiceProfile        VoiceProfile           `json:"voice_profile"`
	CommunicationStyle  CommunicationStyle     `json:"communication_style"`
	BehavioralSignature BehavioralSignature    `json:"behavioral_signature"`
	ValueSystem         ValueSystem            `json:"value_system"`
	EmotionalTone       EmotionalTone          `json:"emotional_tone"`
	SourceMemoriesCount int                    `json:"source_memories_count"`
	ConfidenceScore     float64                `json:"confidence_score"`
	ModelIdentifier     string                 `json:"model_identifier"`
	BehavioralMetrics   map[string]interface{} `json:"behavioral_metrics,omitempty"`
	LinkedMiraMemories  []uuid.UUID            `json:"linked_mira_memories"`
}

type Trait struct {
	Name          string    `json:"name"`
	Category      string    `json:"category"`
	Intensity     float64   `json:"intensity"`
	Confidence    float64   `json:"confidence"`
	EvidenceCount int       `json:"evidence_count"`
	FirstObserved time.Time `json:"first_observed"`
	LastObserved  time.Time `json:"last_observed"`
	LastEvidence  string    `json:"last_evidence,omitempty"`
	Contexts      []string  `json:"contexts,omitempty"`
	Consistency   float64   `json:"consistency"`
}

type VoiceProfile struct {
	FormalityLevel     float64  `json:"formality_level"`
	HumorLevel         float64  `json:"humor_level"`
	EmpathyLevel       float64  `json:"empathy_level"`
	TechnicalDepth     float64  `json:"technical_depth"`
	EnthusiasmLevel    float64  `json:"enthusiasm_level"`
	DirectnessLevel    float64  `json:"directness_level"`
	VocabularyRichness float64  `json:"vocabulary_richness"`
	MetaphorUsage      float64  `json:"metaphor_usage"`
	SentenceStructure  string   `json:"sentence_structure"`
	ExplanationStyle   string   `json:"explanation_style"`
	PreferredOpenings  []string `json:"preferred_openings,omitempty"`
	PreferredClosings  []string `json:"preferred_closings,omitempty"`
	CatchPhrases       []string `json:"catch_phrases,omitempty"`
	UsesEmojis         bool     `json:"uses_emojis"`
	UsesMarkdown       bool     `json:"uses_markdown"`
	AvgSentenceLength  int      `json:"avg_sentence_length"`
}

type CommunicationStyle struct {
	QuestionRate       float64 `json:"question_rate"`
	AcknowledgmentRate float64 `json:"acknowledgment_rate"`
	AlternativeRate    float64 `json:"alternative_rate"`
	Structure          string  `json:"structure"`
}

type BehavioralSignature struct {
	ResponseLength string  `json:"response_length"`
	Initiative     float64 `json:"initiative"`
	Consistency    float64 `json:"consistency"`
}

type ValueSystem struct {
	Values map[string]float64 `json:"values"`
}

type EmotionalTone struct {
	Warmth     float64 `json:"warmth"`
	Positivity float64 `json:"positivity"`
	Energy     float64 `json:"energy"`
	Stability  float64 `json:"stability"`
}

type Prompt struct {
	Content         string    `json:"content"`
	TokenEstimate   int       `json:"token_estimate"`
	BudgetTokens    int       `json:"budget_tokens"`
	Priority        int       `json:"priority"`
	GeneratedAt     time.Time `json:"generated_at"`
	SnapshotVersion int       `json:"snapshot_version"`
}

type DriftDimension struct {
	Dimension     string  `json:"dimension"`
	Change        float64 `json:"change"`
	IsSignificant bool    `json:"is_significant"`
}

type DriftReport struct {
	Timestamp       time.Time        `json:"timestamp"`
	PreviousVersion int              `json:"previous_version"`
	CurrentVersion  int              `json:"current_version"`
	DriftScore      float64          `json:"drift_score"`
	DriftDimensions []DriftDimension `json:"drift_dimensions"`
	IsSignificant   bool             `json:"is_significant"`
	Recommendations []string         `json:"recommendations"`
}

type ModelSwap struct {
	AgentID              string    `json:"agent_id"`
	PreviousModel        string    `json:"previous_model"`
	NewModel             string    `json:"new_model"`
	Timestamp            time.Time `json:"timestamp"`
	IdentityPreserved    bool      `json:"identity_preserved"`
	IdentityDrift        float64   `json:"identity_drift"`
	ReinforcementApplied bool      `json:"reinforcement_applied"`
}

type StatusSummary struct {
	Enabled    bool           `json:"enabled"`
	AgentCount int            `json:"agent_count"`
	Agents     []AgentSummary `json:"agents"`
}

type AgentSummary struct {
	AgentID         string  `json:"agent_id"`
	Version         int     `json:"version"`
	ConfidenceScore float64 `json:"confidence_score"`
	TraitCount      int     `json:"trait_count"`
	LastCapture     string  `json:"last_capture"`
	DriftScore      float64 `json:"drift_score"`
}

type UpdateResult struct {
	AgentID        string    `json:"agent_id"`
	NewVersion     int       `json:"new_version"`
	ChangesApplied []string  `json:"changes_applied"`
	Timestamp      time.Time `json:"timestamp"`
}

type CaptureRequest struct {
	AgentID           string
	Conversation      string
	AgentResponses    []string
	ModelID           string
	SessionID         string
	BehavioralMetrics map[string]interface{}
}

type Runtime struct {
	db       *sql.DB
	cfg      Config
	provider MemoryProvider
	dialect  SQLDialect
	mu       sync.Mutex
}

func NewRuntime(db *sql.DB, cfg Config, provider MemoryProvider) (*Runtime, error) {
	return NewRuntimeWithDialect(db, cfg, provider, string(SQLiteDialect))
}

// NewRuntimeWithDialect creates the built-in identity engine on the same
// database connection as MIRA, using the configured storage dialect.
func NewRuntimeWithDialect(db *sql.DB, cfg Config, provider MemoryProvider, dialect string) (*Runtime, error) {
	if db == nil {
		return nil, errors.New("agent memory: database is required")
	}
	defaults := DefaultConfig()
	if cfg.MinTraitConfidence <= 0 {
		cfg.MinTraitConfidence = defaults.MinTraitConfidence
	}
	if cfg.MinObservationsForTrait <= 0 {
		cfg.MinObservationsForTrait = defaults.MinObservationsForTrait
	}
	if cfg.DefaultBudgetTokens <= 0 {
		cfg.DefaultBudgetTokens = defaults.DefaultBudgetTokens
	}
	if cfg.MaxBudgetTokens <= 0 {
		cfg.MaxBudgetTokens = defaults.MaxBudgetTokens
	}
	if cfg.MaxBudgetTokens < 32 {
		cfg.MaxBudgetTokens = 32
	}
	if cfg.DefaultBudgetTokens < 32 {
		cfg.DefaultBudgetTokens = 32
	}
	if cfg.DefaultBudgetTokens > cfg.MaxBudgetTokens {
		cfg.DefaultBudgetTokens = cfg.MaxBudgetTokens
	}
	if cfg.DriftThreshold <= 0 || cfg.DriftThreshold > 1 {
		cfg.DriftThreshold = defaults.DriftThreshold
	}
	if cfg.DriftWindowSize <= 0 {
		cfg.DriftWindowSize = defaults.DriftWindowSize
	}
	if cfg.MaxHistoryVersions <= 0 {
		cfg.MaxHistoryVersions = defaults.MaxHistoryVersions
	}
	if cfg.MaxMiraMemories <= 0 {
		cfg.MaxMiraMemories = defaults.MaxMiraMemories
	}
	r := &Runtime{db: db, cfg: cfg, provider: provider, dialect: normalizeDialect(dialect)}
	if err := r.initSchema(); err != nil {
		return nil, fmt.Errorf("agent memory schema: %w", err)
	}
	return r, nil
}

func (r *Runtime) Config() Config { return r.cfg }

func (r *Runtime) initSchema() error {
	schema := `
CREATE TABLE IF NOT EXISTS agent_memory_identities (
 id TEXT PRIMARY KEY, agent_id TEXT NOT NULL, version INTEGER NOT NULL,
 created_at TEXT NOT NULL, derived_from_id TEXT, snapshot_json TEXT NOT NULL,
 confidence_score REAL NOT NULL DEFAULT 0, model_identifier TEXT NOT NULL DEFAULT 'unknown',
 UNIQUE(agent_id, version)
);
CREATE INDEX IF NOT EXISTS idx_agent_memory_identities_agent ON agent_memory_identities(agent_id, version);
CREATE TABLE IF NOT EXISTS agent_memory_model_swaps (
 id INTEGER PRIMARY KEY AUTOINCREMENT, agent_id TEXT NOT NULL,
 previous_model TEXT NOT NULL, new_model TEXT NOT NULL, timestamp TEXT NOT NULL,
 identity_preserved INTEGER NOT NULL DEFAULT 0, identity_drift REAL NOT NULL DEFAULT 0,
 reinforcement_applied INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_agent_memory_swaps_agent ON agent_memory_model_swaps(agent_id, timestamp);
CREATE TABLE IF NOT EXISTS agent_memory_links (
 identity_id TEXT NOT NULL, memory_id BLOB NOT NULL, linked_at TEXT NOT NULL,
 PRIMARY KEY(identity_id, memory_id)
);
CREATE INDEX IF NOT EXISTS idx_agent_memory_links_identity ON agent_memory_links(identity_id);
`
	if r.dialect == PostgreSQLDialect {
		schema = `
CREATE TABLE IF NOT EXISTS agent_memory_identities (
 id TEXT PRIMARY KEY, agent_id TEXT NOT NULL, version INTEGER NOT NULL,
 created_at TEXT NOT NULL, derived_from_id TEXT, snapshot_json TEXT NOT NULL,
 confidence_score DOUBLE PRECISION NOT NULL DEFAULT 0, model_identifier TEXT NOT NULL DEFAULT 'unknown',
 UNIQUE(agent_id, version)
);
CREATE INDEX IF NOT EXISTS idx_agent_memory_identities_agent ON agent_memory_identities(agent_id, version);
CREATE TABLE IF NOT EXISTS agent_memory_model_swaps (
 id BIGSERIAL PRIMARY KEY, agent_id TEXT NOT NULL,
 previous_model TEXT NOT NULL, new_model TEXT NOT NULL, timestamp TEXT NOT NULL,
 identity_preserved BOOLEAN NOT NULL DEFAULT FALSE, identity_drift DOUBLE PRECISION NOT NULL DEFAULT 0,
 reinforcement_applied BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX IF NOT EXISTS idx_agent_memory_swaps_agent ON agent_memory_model_swaps(agent_id, timestamp);
CREATE TABLE IF NOT EXISTS agent_memory_links (
 identity_id TEXT NOT NULL, memory_id UUID NOT NULL, linked_at TEXT NOT NULL,
 PRIMARY KEY(identity_id, memory_id)
);
CREATE INDEX IF NOT EXISTS idx_agent_memory_links_identity ON agent_memory_links(identity_id);
`
	}
	if _, err := r.db.Exec(schema); err != nil {
		return err
	}
	return r.migrateLegacyTables()
}

func (r *Runtime) migrateLegacyTables() error {
	if r.dialect != SQLiteDialect {
		return nil
	}
	// Existing installations may have the former table names. Migrate the
	// immutable snapshot payload once, while leaving legacy tables untouched for
	// safe rollback and compatibility with older binaries.
	var exists int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='soul_identities'`).Scan(&exists); err != nil || exists == 0 {
		return nil
	}
	_, err := r.db.Exec(bindPlaceholders(`
INSERT OR IGNORE INTO agent_memory_identities
 (id, agent_id, version, created_at, derived_from_id, snapshot_json, confidence_score, model_identifier)
SELECT id, agent_id, version, CAST(created_at AS TEXT), derived_from_id,
 json_object(
   'id', id, 'agent_id', agent_id, 'version', version, 'created_at', CAST(created_at AS TEXT),
   'derived_from_id', derived_from_id,
   'personality_traits', json(COALESCE(personality_traits, '[]')),
   'voice_profile', json(COALESCE(voice_profile, '{}')),
   'communication_style', json(COALESCE(communication_style, '{}')),
   'behavioral_signature', json(COALESCE(behavioral_signature, '{}')),
   'value_system', json(COALESCE(value_system, '{}')),
   'emotional_tone', json(COALESCE(emotional_tone, '{}')),
   'source_memories_count', COALESCE(source_memories_count, 0),
   'confidence_score', COALESCE(confidence_score, 0),
   'model_identifier', COALESCE(model_identifier, 'unknown'),
   'behavioral_metrics', json(COALESCE(behavioral_metrics, '{}')),
   'linked_mira_memories', json(COALESCE(linked_mira_memories, '[]'))
 ), COALESCE(confidence_score, 0), COALESCE(model_identifier, 'unknown')
FROM soul_identities`, r.dialect))
	if err != nil {
		return fmt.Errorf("migrate legacy identities: %w", err)
	}
	_, _ = r.db.Exec(`INSERT OR IGNORE INTO agent_memory_model_swaps
 (agent_id, previous_model, new_model, timestamp, identity_preserved, identity_drift, reinforcement_applied)
 SELECT agent_id, previous_model, new_model, CAST(timestamp AS TEXT), identity_preserved, identity_drift, reinforcement_applied
 FROM soul_model_swaps`)
	_, _ = r.db.Exec(`INSERT OR IGNORE INTO agent_memory_links(identity_id, memory_id, linked_at)
 SELECT identity_id, memory_id, CAST(linked_at AS TEXT) FROM soul_mira_links`)
	return nil
}

func (r *Runtime) ListAgents(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, bindPlaceholders(`SELECT DISTINCT agent_id FROM agent_memory_identities ORDER BY agent_id`, r.dialect))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

func (r *Runtime) Latest(ctx context.Context, agentID string) (*Snapshot, error) {
	row := r.db.QueryRowContext(ctx, bindPlaceholders(`SELECT snapshot_json FROM agent_memory_identities WHERE agent_id=? ORDER BY version DESC LIMIT 1`, r.dialect), agentID)
	var payload string
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return decodeSnapshot(payload)
}

func (r *Runtime) History(ctx context.Context, agentID string, limit int) ([]*Snapshot, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > r.cfg.MaxHistoryVersions {
		limit = r.cfg.MaxHistoryVersions
	}
	rows, err := r.db.QueryContext(ctx, bindPlaceholders(`SELECT snapshot_json FROM agent_memory_identities WHERE agent_id=? ORDER BY version DESC LIMIT ?`, r.dialect), agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*Snapshot
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		snap, err := decodeSnapshot(payload)
		if err != nil {
			return nil, err
		}
		result = append(result, snap)
	}
	return result, rows.Err()
}

func (r *Runtime) Capture(ctx context.Context, req CaptureRequest) (*Snapshot, error) {
	if strings.TrimSpace(req.AgentID) == "" {
		return nil, errors.New("agent_id is required")
	}
	text := strings.TrimSpace(strings.Join(req.AgentResponses, "\n"))
	if text == "" {
		text = strings.TrimSpace(req.Conversation)
	}
	if text == "" {
		return nil, errors.New("conversation or agent response is required")
	}
	if req.ModelID == "" {
		req.ModelID = "unknown"
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	previous, err := r.Latest(ctx, req.AgentID)
	if err != nil {
		return nil, err
	}
	if previous != nil && !r.cfg.EvolutionEnabled {
		return nil, errors.New("identity evolution is disabled")
	}
	snapshot := extractSnapshot(req, previous, r.cfg)
	if r.provider != nil && r.cfg.EnrichWithMiraMemories {
		memories, _ := r.provider.GetMiraMemories(ctx, req.AgentID, text, r.cfg.DefaultBudgetTokens/4, r.cfg.MaxMiraMemories)
		for _, memory := range memories {
			if memory.MemoryID != uuid.Nil {
				snapshot.LinkedMiraMemories = appendUniqueUUID(snapshot.LinkedMiraMemories, memory.MemoryID)
			}
		}
		snapshot.SourceMemoriesCount = len(snapshot.LinkedMiraMemories)
	}
	if err := r.storeSnapshot(ctx, snapshot); err != nil {
		return nil, err
	}
	if r.provider != nil {
		for _, memoryID := range snapshot.LinkedMiraMemories {
			_ = r.provider.LinkIdentityToMemory(ctx, snapshot.ID, memoryID)
		}
	}
	if r.cfg.AutoCheckAfterCapture && previous != nil {
		if report, driftErr := r.Drift(ctx, req.AgentID, r.cfg.DriftWindowSize); driftErr == nil && report.IsSignificant && r.provider != nil {
			_ = r.provider.NotifyMiraOfIdentityChange(ctx, req.AgentID, "identity_drift")
		}
	}
	return snapshot, nil
}

type sqlExecer interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

func (r *Runtime) storeSnapshot(ctx context.Context, snap *Snapshot) error {
	return r.storeSnapshotWith(ctx, r.db, snap)
}

func (r *Runtime) storeSnapshotWith(ctx context.Context, execer sqlExecer, snap *Snapshot) error {
	payload, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	_, err = execer.ExecContext(ctx, bindPlaceholders(`INSERT INTO agent_memory_identities
 (id, agent_id, version, created_at, derived_from_id, snapshot_json, confidence_score, model_identifier)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, r.dialect), snap.ID.String(), snap.AgentID, snap.Version,
		snap.CreatedAt.UTC().Format(time.RFC3339Nano), nullableUUID(snap.DerivedFromID), string(payload),
		snap.ConfidenceScore, snap.ModelIdentifier)
	return err
}

func nullableUUID(id *uuid.UUID) interface{} {
	if id == nil {
		return nil
	}
	return id.String()
}

func decodeSnapshot(payload string) (*Snapshot, error) {
	var snap Snapshot
	if err := json.Unmarshal([]byte(payload), &snap); err != nil {
		return nil, err
	}
	if snap.ID == uuid.Nil {
		snap.ID = uuid.New()
	}
	if snap.PersonalityTraits == nil {
		snap.PersonalityTraits = []Trait{}
	}
	if snap.LinkedMiraMemories == nil {
		snap.LinkedMiraMemories = []uuid.UUID{}
	}
	return &snap, nil
}

func extractSnapshot(req CaptureRequest, previous *Snapshot, cfg Config) *Snapshot {
	now := time.Now().UTC()
	snap := neutralSnapshot(req.AgentID, req.ModelID, now)
	if previous != nil {
		copySnapshot(snap, previous)
		snap.ID, snap.CreatedAt = uuid.New(), now
		snap.Version = previous.Version + 1
		parent := previous.ID
		snap.DerivedFromID = &parent
		snap.ModelIdentifier = req.ModelID
	}
	snap.SessionID = req.SessionID
	text := strings.TrimSpace(strings.Join(req.AgentResponses, "\n"))
	if text == "" {
		text = req.Conversation
	}
	observed := extractTraits(text, cfg)
	snap.PersonalityTraits = mergeTraits(snap.PersonalityTraits, observed, now)
	snap.VoiceProfile = extractVoice(text, snap.VoiceProfile)
	snap.CommunicationStyle = extractCommunication(text)
	snap.BehavioralSignature = extractBehavior(text)
	snap.ValueSystem = extractValues(text, snap.ValueSystem)
	snap.EmotionalTone = extractEmotion(text)
	snap.BehavioralMetrics = cloneMap(req.BehavioralMetrics)
	if len(snap.PersonalityTraits) > 0 {
		total := 0.0
		for _, trait := range snap.PersonalityTraits {
			total += trait.Confidence
		}
		snap.ConfidenceScore = clamp(total/float64(len(snap.PersonalityTraits)), 0, 1)
	}
	if len(snap.PersonalityTraits) == 0 && len(req.BehavioralMetrics) > 0 {
		snap.PersonalityTraits = traitsFromMetrics(req.BehavioralMetrics, now)
		snap.ConfidenceScore = 0.6
	}
	return snap
}

func neutralSnapshot(agentID, modelID string, now time.Time) *Snapshot {
	return &Snapshot{ID: uuid.New(), AgentID: agentID, Version: 1, CreatedAt: now, ModelIdentifier: modelID,
		PersonalityTraits: []Trait{}, LinkedMiraMemories: []uuid.UUID{}, VoiceProfile: VoiceProfile{
			FormalityLevel: .5, HumorLevel: .3, EmpathyLevel: .6, TechnicalDepth: .5, EnthusiasmLevel: .5,
			DirectnessLevel: .6, VocabularyRichness: .6, MetaphorUsage: .3, SentenceStructure: "balanced",
			ExplanationStyle: "step_by_step", UsesMarkdown: true, AvgSentenceLength: 15,
		}, CommunicationStyle: CommunicationStyle{Structure: "balanced"}, BehavioralSignature: BehavioralSignature{ResponseLength: "moderate", Initiative: .5, Consistency: .5},
		ValueSystem: ValueSystem{Values: map[string]float64{}}, EmotionalTone: EmotionalTone{Warmth: .5, Positivity: .5, Energy: .5, Stability: .5}}
}

func copySnapshot(dst, src *Snapshot) {
	data, _ := json.Marshal(src)
	_ = json.Unmarshal(data, dst)
	if dst.PersonalityTraits == nil {
		dst.PersonalityTraits = []Trait{}
	}
	if dst.LinkedMiraMemories == nil {
		dst.LinkedMiraMemories = []uuid.UUID{}
	}
}

type traitRule struct {
	name, category        string
	keywords              []string
	intensity, confidence float64
}

var traitRules = []traitRule{
	{"analytical", "cognitive", []string{"analyze", "analysis", "analyse", "examine", "break down", "décompose", "analyse"}, .8, .6},
	{"creative", "cognitive", []string{"imagine", "create", "creative", "innovative", "créatif", "imagine"}, .7, .5},
	{"logical", "cognitive", []string{"therefore", "logically", "consequently", "donc", "logique"}, .8, .6},
	{"empathetic", "emotional", []string{"i understand", "that sounds", "i hear", "je comprends", "cela semble"}, .8, .7},
	{"patient", "emotional", []string{"take your time", "no rush", "step by step", "pas à pas", "prends ton temps"}, .7, .6},
	{"enthusiastic", "emotional", []string{"amazing", "fantastic", "excited", "génial", "formidable", "super"}, .8, .6},
	{"collaborative", "social", []string{"together", "let's", "we can", "ensemble", "nous pouvons"}, .7, .5},
	{"direct", "social", []string{"frankly", "honestly", "to be direct", "directement", "franchement"}, .8, .6},
	{"curious", "epistemic", []string{"wonder", "curious", "explore", "discover", "curieux", "explorer"}, .8, .6},
	{"humorous", "expressive", []string{"haha", "funny", "joke", "humour", "drôle", "lol"}, .7, .6},
	{"concise", "expressive", []string{"in short", "briefly", "to sum up", "en bref", "bref"}, .7, .5},
	{"transparent", "ethical", []string{"transparent", "full disclosure", "honestly", "en toute transparence"}, .8, .7},
	{"benevolent", "ethical", []string{"help you", "your best interest", "want to help", "aider", "bienveillance"}, .7, .6},
}

func extractTraits(text string, cfg Config) []Trait {
	lower := strings.ToLower(text)
	now := time.Now().UTC()
	var result []Trait
	for _, rule := range traitRules {
		count := 0
		evidence := ""
		for _, keyword := range rule.keywords {
			n := strings.Count(lower, strings.ToLower(keyword))
			if n > count {
				evidence = keyword
			}
			count += n
		}
		if count == 0 {
			continue
		}
		intensity := clamp(rule.intensity*math.Min(1, math.Log1p(float64(count))/1.5), 0, 1)
		confidence := clamp(math.Max(cfg.MinTraitConfidence, rule.confidence+math.Min(.25, float64(count-1)*.05)), 0, 1)
		result = append(result, Trait{Name: rule.name, Category: rule.category, Intensity: intensity, Confidence: confidence, EvidenceCount: count, FirstObserved: now, LastObserved: now, LastEvidence: evidence, Consistency: .5})
	}
	return result
}

func mergeTraits(existing, observed []Trait, now time.Time) []Trait {
	result := append([]Trait(nil), existing...)
	for _, incoming := range observed {
		found := false
		for i := range result {
			if result[i].Name != incoming.Name {
				continue
			}
			found = true
			old := result[i]
			total := old.EvidenceCount + incoming.EvidenceCount
			if total <= 0 {
				total = 1
			}
			result[i].Intensity = clamp((old.Intensity*float64(old.EvidenceCount)+incoming.Intensity*float64(incoming.EvidenceCount))/float64(total), 0, 1)
			result[i].EvidenceCount = total
			result[i].Confidence = clamp(1-(1-old.Confidence)*math.Exp(-float64(incoming.EvidenceCount)/5), 0, 1)
			result[i].LastObserved = now
			result[i].LastEvidence = incoming.LastEvidence
			result[i].Consistency = clamp(result[i].Consistency+.05, 0, 1)
			break
		}
		if !found {
			result = append(result, incoming)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Confidence == result[j].Confidence {
			return result[i].Name < result[j].Name
		}
		return result[i].Confidence > result[j].Confidence
	})
	return result
}

func extractVoice(text string, previous VoiceProfile) VoiceProfile {
	lower := strings.ToLower(text)
	sentences := sentenceCount(text)
	words := len(strings.Fields(text))
	if words > 0 {
		previous.AvgSentenceLength = words / maxInt(sentences, 1)
	}
	previous.FormalityLevel = blend(previous.FormalityLevel, ratioAny(lower, "dear", "sincerely", "furthermore", "cependant", "cependant"), .35)
	previous.HumorLevel = blend(previous.HumorLevel, ratioAny(lower, "haha", "lol", "joke", "humour", ":)"), .35)
	previous.EmpathyLevel = blend(previous.EmpathyLevel, ratioAny(lower, "i understand", "i hear", "je comprends", "je vois"), .35)
	previous.TechnicalDepth = blend(previous.TechnicalDepth, ratioAny(lower, "api", "architecture", "implementation", "fonction", "système"), .35)
	previous.EnthusiasmLevel = blend(previous.EnthusiasmLevel, ratioAny(lower, "amazing", "fantastic", "génial", "formidable"), .35)
	previous.DirectnessLevel = blend(previous.DirectnessLevel, ratioAny(lower, "direct", "frankly", "directement", "en bref"), .35)
	previous.UsesEmojis = strings.ContainsAny(text, "😀😁😂🤣😊🙂😉👍❤️")
	previous.UsesMarkdown = strings.Contains(text, "**") || strings.Contains(text, "- ") || strings.Contains(text, "##")
	if strings.Contains(lower, "step by step") || strings.Contains(lower, "pas à pas") {
		previous.ExplanationStyle = "step_by_step"
	}
	if strings.Contains(lower, "in short") || strings.Contains(lower, "en bref") {
		previous.SentenceStructure = "concise"
	}
	return previous
}

func extractCommunication(text string) CommunicationStyle {
	lower := strings.ToLower(text)
	return CommunicationStyle{QuestionRate: ratioAny(lower, "?", "how", "why", "comment", "pourquoi"), AcknowledgmentRate: ratioAny(lower, "i see", "understood", "got it", "je comprends", "compris"), AlternativeRate: ratioAny(lower, "alternatively", "another option", "sinon", "autre option"), Structure: chooseStructure(text)}
}
func extractBehavior(text string) BehavioralSignature {
	words := len(strings.Fields(text))
	response := "moderate"
	if words < 30 {
		response = "concise"
	}
	if words > 180 {
		response = "detailed"
	}
	return BehavioralSignature{ResponseLength: response, Initiative: clamp(.4+ratioAny(strings.ToLower(text), "let's", "je propose", "i recommend", "je suggère")*.5, 0, 1), Consistency: .5}
}
func extractValues(text string, previous ValueSystem) ValueSystem {
	if previous.Values == nil {
		previous.Values = map[string]float64{}
	}
	lower := strings.ToLower(text)
	for key, words := range map[string][]string{"clarity": {"clear", "clarity", "clarté"}, "accuracy": {"accurate", "precise", "exact", "précis"}, "helpfulness": {"help", "useful", "aider", "utile"}, "transparency": {"transparent", "honest", "transparence"}} {
		if ratioAny(lower, words...) > 0 {
			previous.Values[key] = clamp(previous.Values[key]+.1, 0, 1)
		}
	}
	return previous
}
func extractEmotion(text string) EmotionalTone {
	lower := strings.ToLower(text)
	return EmotionalTone{Warmth: clamp(.5+ratioAny(lower, "thank", "please", "merci", "s'il te plaît")*.3, 0, 1), Positivity: clamp(.5+ratioAny(lower, "great", "good", "amazing", "bien", "génial")*.3-ratioAny(lower, "error", "bad", "problème")*.2, 0, 1), Energy: clamp(.4+ratioAny(lower, "!", "excited", "enthousiaste")*.4, 0, 1), Stability: .6}
}

func traitsFromMetrics(metrics map[string]interface{}, now time.Time) []Trait {
	var result []Trait
	if value, ok := metrics["success_rate"].(float64); ok && value >= .8 {
		result = append(result, Trait{Name: "precise", Category: "cognitive", Intensity: .8, Confidence: .6, EvidenceCount: 1, FirstObserved: now, LastObserved: now, LastEvidence: fmt.Sprintf("success rate %.0f%%", value*100), Consistency: .5})
	}
	if tools, ok := metrics["preferred_tools"].([]interface{}); ok && len(tools) > 0 {
		result = append(result, Trait{Name: "tool-oriented", Category: "cognitive", Intensity: .7, Confidence: .6, EvidenceCount: 1, FirstObserved: now, LastObserved: now, LastEvidence: "preferred tools", Consistency: .5})
	}
	return result
}

func (r *Runtime) Recall(ctx context.Context, agentID, query string, budget int) (*Prompt, error) {
	snap, err := r.Latest(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if snap == nil {
		return nil, fmt.Errorf("no identity found for agent %s", agentID)
	}
	budget = r.normalizeBudget(budget)
	content := composePrompt(snap, r.cfg.MinObservationsForTrait)
	if r.provider != nil && r.cfg.EnrichWithMiraMemories && strings.TrimSpace(query) != "" {
		memories, _ := r.provider.GetMiraMemories(ctx, agentID, query, budget/4, r.cfg.MaxMiraMemories)
		if len(memories) > 0 {
			var b strings.Builder
			b.WriteString(content)
			b.WriteString("\n\n### Relevant MIRA memories\n")
			for _, memory := range memories {
				b.WriteString("- ")
				b.WriteString(memory.Content)
				b.WriteByte('\n')
			}
			content = b.String()
		}
	}
	content = truncateToBudget(content, budget)
	return &Prompt{Content: content, TokenEstimate: estimateTokens(content), BudgetTokens: budget, Priority: 100, GeneratedAt: time.Now().UTC(), SnapshotVersion: snap.Version}, nil
}
func (r *Runtime) normalizeBudget(budget int) int {
	if budget <= 0 {
		budget = r.cfg.DefaultBudgetTokens
	}
	if budget > r.cfg.MaxBudgetTokens {
		budget = r.cfg.MaxBudgetTokens
	}
	if budget < 32 {
		budget = 32
	}
	return budget
}
func composePrompt(s *Snapshot, minObservations int) string {
	var b strings.Builder
	b.WriteString("## Agent identity\n\n### How you communicate\n")
	b.WriteString(fmt.Sprintf("Formality %.2f; empathy %.2f; technical depth %.2f; directness %.2f.\n", s.VoiceProfile.FormalityLevel, s.VoiceProfile.EmpathyLevel, s.VoiceProfile.TechnicalDepth, s.VoiceProfile.DirectnessLevel))
	b.WriteString("\n### Core traits\n")
	stableTraits := 0
	for _, t := range s.PersonalityTraits {
		if t.Confidence >= .6 && t.EvidenceCount >= minObservations {
			b.WriteString("- ")
			b.WriteString(t.Name)
			b.WriteString(fmt.Sprintf(" (confidence %.0f%%)\n", t.Confidence*100))
			stableTraits++
		}
	}
	if stableTraits == 0 {
		b.WriteString("- No stable trait captured yet; continue observing before treating a trait as persistent.\n")
	}
	b.WriteString("\n### Communication style\n")
	b.WriteString(fmt.Sprintf("Structure: %s; response length: %s.\n", s.CommunicationStyle.Structure, s.BehavioralSignature.ResponseLength))
	b.WriteString("\n### Values\n")
	keys := make([]string, 0, len(s.ValueSystem.Values))
	for k := range s.ValueSystem.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(fmt.Sprintf("- %s (%.0f%%)\n", k, s.ValueSystem.Values[k]*100))
	}
	return b.String()
}

func (r *Runtime) Drift(ctx context.Context, agentID string, window int) (*DriftReport, error) {
	history, err := r.History(ctx, agentID, window)
	if err != nil {
		return nil, err
	}
	if len(history) < 2 {
		return &DriftReport{Timestamp: time.Now().UTC(), CurrentVersion: func() int {
			if len(history) == 1 {
				return history[0].Version
			}
			return 0
		}(), DriftDimensions: []DriftDimension{}, Recommendations: []string{"Capture another version before measuring drift."}}, nil
	}
	current, previous := history[0], history[len(history)-1]
	dims := []DriftDimension{{"voice_profile", voiceDistance(previous.VoiceProfile, current.VoiceProfile), false}, {"personality_traits", traitDistance(previous.PersonalityTraits, current.PersonalityTraits), false}, {"value_system", valueDistance(previous.ValueSystem, current.ValueSystem), false}, {"communication_style", styleDistance(previous.CommunicationStyle, current.CommunicationStyle), false}}
	total := 0.0
	for i := range dims {
		dims[i].Change = clamp(dims[i].Change, 0, 1)
		dims[i].IsSignificant = dims[i].Change >= r.cfg.DriftThreshold
		total += dims[i].Change
	}
	score := total / float64(len(dims))
	report := &DriftReport{Timestamp: time.Now().UTC(), PreviousVersion: previous.Version, CurrentVersion: current.Version, DriftScore: score, DriftDimensions: dims, IsSignificant: score >= r.cfg.DriftThreshold}
	if report.IsSignificant {
		report.Recommendations = []string{"Reinforce the current identity before continuing."}
	}
	return report, nil
}

func (r *Runtime) HandleSwap(ctx context.Context, agentID, fromModel, toModel string) (*ModelSwap, *Prompt, error) {
	if strings.TrimSpace(agentID) == "" || strings.TrimSpace(fromModel) == "" || strings.TrimSpace(toModel) == "" {
		return nil, nil, errors.New("agent_id, from_model and to_model are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, err := r.Latest(ctx, agentID)
	if err != nil {
		return nil, nil, err
	}
	swap := &ModelSwap{AgentID: agentID, PreviousModel: fromModel, NewModel: toModel, Timestamp: time.Now().UTC(), IdentityPreserved: true}
	if current != nil {
		if current.ModelIdentifier != "" && current.ModelIdentifier != "unknown" && current.ModelIdentifier != fromModel {
			return nil, nil, fmt.Errorf("from_model %q does not match active model %q", fromModel, current.ModelIdentifier)
		}
		report, err := r.Drift(ctx, agentID, r.cfg.DriftWindowSize)
		if err != nil {
			return nil, nil, err
		}
		if report != nil {
			swap.IdentityDrift = report.DriftScore
			swap.IdentityPreserved = !report.IsSignificant
		}
		if !r.cfg.EvolutionEnabled || !r.cfg.AutoReinforce {
			if err := r.storeSwap(ctx, swap); err != nil {
				return nil, nil, err
			}
			return swap, nil, nil
		}
		// A model transition is an immutable identity event. Keep the full
		// snapshot lineage while moving the active model marker forward so the
		// next model receives exactly the identity that was preserved.
		reinforced := &Snapshot{}
		copySnapshot(reinforced, current)
		reinforced.ID = uuid.New()
		reinforced.CreatedAt = swap.Timestamp
		reinforced.Version = current.Version + 1
		parent := current.ID
		reinforced.DerivedFromID = &parent
		reinforced.ModelIdentifier = toModel
		swap.ReinforcementApplied = true
		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, nil, err
		}
		if err := r.storeSnapshotWith(ctx, tx, reinforced); err != nil {
			_ = tx.Rollback()
			return nil, nil, err
		}
		if err := r.storeSwapWith(ctx, tx, swap); err != nil {
			_ = tx.Rollback()
			return nil, nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, nil, err
		}
		prompt, err := r.Recall(ctx, agentID, "", r.cfg.DefaultBudgetTokens)
		if err != nil {
			return nil, nil, err
		}
		if r.provider != nil {
			_ = r.provider.NotifyMiraOfIdentityChange(ctx, agentID, "model_transition")
		}
		return swap, prompt, nil
	}
	if err := r.storeSwap(ctx, swap); err != nil {
		return nil, nil, err
	}
	return swap, nil, nil
}
func (r *Runtime) storeSwap(ctx context.Context, swap *ModelSwap) error {
	return r.storeSwapWith(ctx, r.db, swap)
}

func (r *Runtime) storeSwapWith(ctx context.Context, execer sqlExecer, swap *ModelSwap) error {
	_, err := execer.ExecContext(ctx, bindPlaceholders(`INSERT INTO agent_memory_model_swaps(agent_id,previous_model,new_model,timestamp,identity_preserved,identity_drift,reinforcement_applied) VALUES(?,?,?,?,?,?,?)`, r.dialect), swap.AgentID, swap.PreviousModel, swap.NewModel, swap.Timestamp.Format(time.RFC3339Nano), swap.IdentityPreserved, swap.IdentityDrift, swap.ReinforcementApplied)
	return err
}

func (r *Runtime) Status(ctx context.Context) (*StatusSummary, error) {
	agents, err := r.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	result := &StatusSummary{Enabled: true, AgentCount: len(agents), Agents: make([]AgentSummary, 0, len(agents))}
	for _, id := range agents {
		snap, e := r.Latest(ctx, id)
		if e != nil {
			continue
		}
		sum := AgentSummary{AgentID: id, Version: snap.Version, ConfidenceScore: snap.ConfidenceScore, TraitCount: len(snap.PersonalityTraits), LastCapture: snap.CreatedAt.Format(time.RFC3339)}
		if drift, e := r.Drift(ctx, id, r.cfg.DriftWindowSize); e == nil && drift != nil {
			sum.DriftScore = drift.DriftScore
		}
		result.Agents = append(result.Agents, sum)
	}
	return result, nil
}

// QueryStatus implements MIRA's REST status boundary.
func (r *Runtime) QueryStatus(ctx context.Context) (*interactors.AgentMemoryStatusSummary, error) {
	status, err := r.Status(ctx)
	if err != nil {
		return nil, err
	}
	agents := make([]interactors.AgentMemoryAgentSummary, 0, len(status.Agents))
	for _, agent := range status.Agents {
		agents = append(agents, interactors.AgentMemoryAgentSummary{AgentID: agent.AgentID, Version: agent.Version, ConfidenceScore: agent.ConfidenceScore, TraitCount: agent.TraitCount, LastCapture: agent.LastCapture, DriftScore: agent.DriftScore})
	}
	return &interactors.AgentMemoryStatusSummary{Enabled: status.Enabled, AgentCount: status.AgentCount, Agents: agents}, nil
}

func (r *Runtime) Update(ctx context.Context, agentID, directive, reason string) (*Snapshot, *UpdateResult, error) {
	if !r.cfg.EvolutionEnabled {
		return nil, nil, errors.New("identity evolution is disabled")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	snap, err := r.Latest(ctx, agentID)
	if err != nil {
		return nil, nil, err
	}
	lower := strings.ToLower(directive)
	changes := []string{}
	next := &Snapshot{}
	if snap == nil {
		next = neutralSnapshot(agentID, "unknown", time.Now().UTC())
		next.ConfidenceScore = .5
	} else {
		copySnapshot(next, snap)
		next.ID = uuid.New()
		next.CreatedAt = time.Now().UTC()
		next.Version = snap.Version + 1
		parent := snap.ID
		next.DerivedFromID = &parent
	}
	if containsAny(lower, "enthusiasm", "enthousiasme") {
		next.VoiceProfile.EnthusiasmLevel = clamp(next.VoiceProfile.EnthusiasmLevel+.15, 0, 1)
		changes = append(changes, "enthusiasm increased")
	}
	if containsAny(lower, "formal", "formel") {
		next.VoiceProfile.FormalityLevel = clamp(next.VoiceProfile.FormalityLevel+.15, 0, 1)
		changes = append(changes, "formality increased")
	}
	if containsAny(lower, "humor", "humour") {
		next.VoiceProfile.HumorLevel = clamp(next.VoiceProfile.HumorLevel+.15, 0, 1)
		changes = append(changes, "humor increased")
	}
	if containsAny(lower, "concise", "concis") {
		next.VoiceProfile.SentenceStructure = "concise"
		next.BehavioralSignature.ResponseLength = "concise"
		changes = append(changes, "conciseness enabled")
	}
	if containsAny(lower, "technical", "technique") {
		next.VoiceProfile.TechnicalDepth = clamp(next.VoiceProfile.TechnicalDepth+.15, 0, 1)
		changes = append(changes, "technical depth increased")
	}
	if len(changes) == 0 {
		return nil, nil, fmt.Errorf("directive not recognized")
	}
	if err := r.storeSnapshot(ctx, next); err != nil {
		return nil, nil, err
	}
	return next, &UpdateResult{AgentID: agentID, NewVersion: next.Version, ChangesApplied: changes, Timestamp: next.CreatedAt}, nil
}

func (r *Runtime) Patch(ctx context.Context, agentID string, patch map[string]interface{}, reason string) (*Snapshot, *UpdateResult, error) {
	if !r.cfg.EvolutionEnabled {
		return nil, nil, errors.New("identity evolution is disabled")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	snap, err := r.Latest(ctx, agentID)
	if err != nil {
		return nil, nil, err
	}
	next := &Snapshot{}
	if snap == nil {
		next = neutralSnapshot(agentID, "unknown", time.Now().UTC())
		next.ConfidenceScore = .5
	} else {
		copySnapshot(next, snap)
		next.ID = uuid.New()
		next.CreatedAt = time.Now().UTC()
		next.Version = snap.Version + 1
		parent := snap.ID
		next.DerivedFromID = &parent
	}
	changes := []string{}
	for key, value := range patch {
		f, ok := value.(float64)
		switch key {
		case "enthusiasm_level", "formality_level", "humor_level", "empathy_level", "technical_depth", "directness_level", "vocabulary_richness", "metaphor_usage":
			if !ok || f < 0 || f > 1 {
				return nil, nil, fmt.Errorf("%s must be between 0 and 1", key)
			}
			switch key {
			case "enthusiasm_level":
				next.VoiceProfile.EnthusiasmLevel = f
			case "formality_level":
				next.VoiceProfile.FormalityLevel = f
			case "humor_level":
				next.VoiceProfile.HumorLevel = f
			case "empathy_level":
				next.VoiceProfile.EmpathyLevel = f
			case "technical_depth":
				next.VoiceProfile.TechnicalDepth = f
			case "directness_level":
				next.VoiceProfile.DirectnessLevel = f
			case "vocabulary_richness":
				next.VoiceProfile.VocabularyRichness = f
			case "metaphor_usage":
				next.VoiceProfile.MetaphorUsage = f
			}
			changes = append(changes, key)
		case "uses_emojis":
			if b, ok := value.(bool); ok {
				next.VoiceProfile.UsesEmojis = b
				changes = append(changes, key)
			}
		case "uses_markdown":
			if b, ok := value.(bool); ok {
				next.VoiceProfile.UsesMarkdown = b
				changes = append(changes, key)
			}
		case "sentence_structure":
			if s, ok := value.(string); ok {
				next.VoiceProfile.SentenceStructure = s
				changes = append(changes, key)
			}
		}
	}
	if len(changes) == 0 {
		return nil, nil, errors.New("patch is empty")
	}
	if err := r.storeSnapshot(ctx, next); err != nil {
		return nil, nil, err
	}
	return next, &UpdateResult{AgentID: agentID, NewVersion: next.Version, ChangesApplied: changes, Timestamp: next.CreatedAt}, nil
}

func estimateTokens(text string) int {
	tokens := 0
	word := false
	for _, r := range text {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			word = true
		case unicode.IsSpace(r):
			if word {
				tokens++
				word = false
			}
		case word:
			tokens++
			word = false
			tokens++
		default:
			tokens++
		}
	}
	if word {
		tokens++
	}
	return tokens
}
func truncateToBudget(text string, budget int) string {
	if estimateTokens(text) <= budget {
		return strings.TrimSpace(text)
	}
	words := strings.Fields(text)
	var b strings.Builder
	for _, word := range words {
		candidate := strings.TrimSpace(b.String() + " " + word)
		if estimateTokens(candidate) > budget-1 {
			break
		}
		b.WriteString(word)
		b.WriteByte(' ')
	}
	return strings.TrimSpace(b.String())
}
func ratioAny(text string, terms ...string) float64 {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	hits := 0
	for _, term := range terms {
		if strings.Contains(text, strings.ToLower(term)) {
			hits++
		}
	}
	return math.Min(1, float64(hits)/3)
}
func containsAny(text string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}
func blend(old, observed, weight float64) float64 { return clamp(old*(1-weight)+observed*weight, 0, 1) }
func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func sentenceCount(text string) int {
	n := strings.Count(text, ".") + strings.Count(text, "!") + strings.Count(text, "?")
	if n == 0 && strings.TrimSpace(text) != "" {
		return 1
	}
	return n
}
func chooseStructure(text string) string {
	if strings.Contains(text, "\n-") || strings.Contains(text, "\n*") {
		return "bulleted"
	}
	if strings.Contains(text, "\n1.") {
		return "numbered"
	}
	return "freeform"
}
func cloneMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := map[string]interface{}{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
func appendUniqueUUID(items []uuid.UUID, id uuid.UUID) []uuid.UUID {
	for _, item := range items {
		if item == id {
			return items
		}
	}
	return append(items, id)
}
func voiceDistance(a, b VoiceProfile) float64 {
	vals := []float64{math.Abs(a.FormalityLevel - b.FormalityLevel), math.Abs(a.HumorLevel - b.HumorLevel), math.Abs(a.EmpathyLevel - b.EmpathyLevel), math.Abs(a.TechnicalDepth - b.TechnicalDepth), math.Abs(a.EnthusiasmLevel - b.EnthusiasmLevel), math.Abs(a.DirectnessLevel - b.DirectnessLevel), math.Abs(a.VocabularyRichness - b.VocabularyRichness), math.Abs(a.MetaphorUsage - b.MetaphorUsage)}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}
func traitDistance(a, b []Trait) float64 {
	am := map[string]Trait{}
	bm := map[string]Trait{}
	for _, t := range a {
		am[t.Name] = t
	}
	for _, t := range b {
		bm[t.Name] = t
	}
	names := map[string]bool{}
	for n := range am {
		names[n] = true
	}
	for n := range bm {
		names[n] = true
	}
	if len(names) == 0 {
		return 0
	}
	sum := 0.0
	for n := range names {
		x, y := am[n], bm[n]
		sum += math.Abs(x.Intensity-y.Intensity) + math.Abs(x.Confidence-y.Confidence)
	}
	return clamp(sum/(2*float64(len(names))), 0, 1)
}
func valueDistance(a, b ValueSystem) float64 {
	keys := map[string]bool{}
	for k := range a.Values {
		keys[k] = true
	}
	for k := range b.Values {
		keys[k] = true
	}
	if len(keys) == 0 {
		return 0
	}
	sum := 0.0
	for k := range keys {
		sum += math.Abs(a.Values[k] - b.Values[k])
	}
	return clamp(sum/float64(len(keys)), 0, 1)
}
func styleDistance(a, b CommunicationStyle) float64 {
	return clamp((math.Abs(a.QuestionRate-b.QuestionRate)+math.Abs(a.AcknowledgmentRate-b.AcknowledgmentRate)+math.Abs(a.AlternativeRate-b.AlternativeRate))/3, 0, 1)
}
