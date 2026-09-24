// StoreMemory use case
package interactors

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// StoreMemoryInput contains the input for storing a memory
type StoreMemoryInput struct {
	Content    string
	Wing       string
	Room       *string
	Type       *valueobjects.MemoryType
	Kind       *valueobjects.MemoryKind
	Metrics    map[string]any
	ValidFrom  *time.Time
	ValidUntil *time.Time
}

// WingRoomRe matches valid wing and room identifiers.
var WingRoomRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

const defaultDecisionRoom = "decisions"

// Validate checks that the input meets business constraints.
func (in StoreMemoryInput) Validate() error {
	if utf8.RuneCountInString(in.Content) == 0 {
		return fmt.Errorf("content is required")
	}
	if utf8.RuneCountInString(in.Content) > 65536 {
		return fmt.Errorf("content exceeds maximum length of 65536 characters")
	}
	if !WingRoomRe.MatchString(in.Wing) {
		return fmt.Errorf("wing must be 1-100 alphanumeric characters, hyphens or underscores")
	}
	if utf8.RuneCountInString(in.Wing) > 100 {
		return fmt.Errorf("wing exceeds maximum length of 100 characters")
	}
	if in.Room != nil {
		if !WingRoomRe.MatchString(*in.Room) {
			return fmt.Errorf("room must be 1-100 alphanumeric characters, hyphens or underscores")
		}
		if utf8.RuneCountInString(*in.Room) > 100 {
			return fmt.Errorf("room exceeds maximum length of 100 characters")
		}
	}
	if in.Type != nil && !in.Type.IsValid() {
		return fmt.Errorf("invalid memory type: %s", *in.Type)
	}
	if in.Kind != nil && !in.Kind.IsValid() {
		return fmt.Errorf("invalid memory kind: %s", *in.Kind)
	}
	if in.ValidFrom != nil && in.ValidUntil != nil && in.ValidUntil.Before(*in.ValidFrom) {
		return fmt.Errorf("valid_until must be greater than or equal to valid_from")
	}
	if len(in.Metrics) > 0 {
		if b, err := json.Marshal(in.Metrics); err != nil {
			return fmt.Errorf("metrics must be valid JSON: %w", err)
		} else if len(b) > 10000 {
			return fmt.Errorf("metrics exceeds maximum serialized size of 10000 bytes")
		}
	}
	return nil
}

// StoreMemoryOutput contains the output of storing a memory
type StoreMemoryOutput struct {
	FingerprintID string `json:"fingerprint_id"`
	Type          string `json:"type"`
	Kind          string `json:"kind"`
	FactCount     int    `json:"fact_count"`
	TokenCount    int    `json:"token_count"`
	ModelHash     string `json:"model_hash"`
}

// StoreMemory implements the store memory use case
type StoreMemory struct {
	repository          ports.Repository
	extractor           ports.FingerprintExtractor
	causalDetector      ports.CausalRelationDetector
	vectorStore         ports.VectorStore
	metricsCollector    ports.MetricsCollector
	logger              ports.Logger
	autoCompressEnabled bool
	autoCompressMinTok  int
}

// NewStoreMemory creates a new store memory interactor
func NewStoreMemory(
	repository ports.Repository,
	extractor ports.FingerprintExtractor,
	causalDetector ports.CausalRelationDetector,
	vectorStore ports.VectorStore,
	metricsCollector ports.MetricsCollector,
	logger ports.Logger,
) *StoreMemory {
	return &StoreMemory{
		repository:       repository,
		extractor:        extractor,
		causalDetector:   causalDetector,
		vectorStore:      vectorStore,
		metricsCollector: metricsCollector,
		logger:           logger,
	}
}

// WithCompression enables rule-based auto-compression for session_notes
// that exceed minTokens at store time. The compression runs in a goroutine
// (non-fatal, fire-and-forget). minTokens <= 0 defaults to 100.
func (uc *StoreMemory) WithCompression(enabled bool, minTokens int) *StoreMemory {
	uc.autoCompressEnabled = enabled
	uc.autoCompressMinTok = minTokens
	if uc.autoCompressMinTok <= 0 {
		uc.autoCompressMinTok = 100
	}
	return uc
}

// defaultRoomForType suggests a standard room when none is provided.
func defaultRoomForType(memType valueobjects.MemoryType) *string {
	var room string
	switch memType {
	case valueobjects.TypeDecision:
		room = defaultDecisionRoom
	case valueobjects.TypeFact:
		room = "facts"
	case valueobjects.TypePreference:
		room = "preferences"
	case valueobjects.TypeSessionNote:
		room = "session"
	case valueobjects.TypeDebugLog:
		room = "debug"
	default:
		return nil
	}
	return &room
}

// Execute stores a memory with full extraction (atomic transaction)
func (uc *StoreMemory) Execute(ctx context.Context, input StoreMemoryInput) (*StoreMemoryOutput, error) {
	start := time.Now()

	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// 0. Exact deduplication — if an identical verbatim already exists, return it
	if exactStore, ok := uc.vectorStore.(interface {
		SearchExact(ctx context.Context, query string, limit int, wing, room *string) ([]*entities.Candidate, error)
	}); ok {
		exact, err := exactStore.SearchExact(ctx, input.Content, 1, &input.Wing, input.Room)
		if err == nil && len(exact) > 0 && exact[0].Memory != nil {
			return &StoreMemoryOutput{
				FingerprintID: exact[0].Memory.ID.String(),
				Type:          string(exact[0].Memory.Type),
				Kind:          string(exact[0].Verbatim.Kind),
				FactCount:     exact[0].Memory.FactCount,
				TokenCount:    exact[0].Verbatim.TokenCount,
				ModelHash:     exact[0].Memory.ModelHash,
			}, nil
		}
	}

	// 1. Create verbatim
	verbatim := entities.NewVerbatim(input.Content, input.Wing, input.Room)
	verbatim.Metrics = input.Metrics
	verbatim.ValidFrom = input.ValidFrom
	verbatim.ValidUntil = input.ValidUntil

	// 2. Extract T1 and T2
	fp, emb, err := uc.extractor.ExtractPipeline(ctx, verbatim, input.Type)
	if err != nil {
		return nil, fmt.Errorf("extraction failed: %w", err)
	}

	// 2b. Apply default room if none provided, based on detected type
	if input.Room == nil {
		if r := defaultRoomForType(fp.Type); r != nil {
			input.Room = r
			verbatim.Room = r
		}
	}
	if input.Kind != nil {
		verbatim.Kind = *input.Kind
	} else {
		verbatim.Kind = valueobjects.DefaultMemoryKindForType(fp.Type)
	}

	// 3. Atomic transaction for T0, T1, T2 storage
	tx, err := uc.repository.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Store T0
	if err := uc.repository.StoreVerbatimTx(ctx, tx, verbatim); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("failed to store verbatim: %w", err)
	}

	// Store T1
	if err := uc.repository.StoreFingerprintTx(ctx, tx, fp); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("failed to store fingerprint: %w", err)
	}

	// Store T2
	if err := uc.repository.StoreEmbeddingTx(ctx, tx, emb); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("failed to store embedding: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Beliefs are a repairable projection of the committed T0/T1 records.
	// Persistence is optional so existing repositories remain compatible.
	if beliefRepo, ok := uc.repository.(ports.BeliefRepository); ok {
		if belief := deriveBeliefFromFingerprint(fp, verbatim); belief != nil {
			if err := beliefRepo.UpsertBelief(ctx, belief); err != nil {
				warnDerivedIndex(uc.logger, "failed to persist belief projection", "error", err, "verbatim_id", verbatim.ID.String())
			}
		}
	}

	// 4. Add to vector store (non-fatal)
	candidate := entities.NewCandidate(fp, verbatim, emb.Vector)
	if err := uc.vectorStore.AddCandidate(ctx, candidate); err != nil {
		// The repository remains authoritative. Try to repair the derived index before
		// continuing so a transient HNSW failure does not leave it stale.
		repairErr := repairVectorStore(ctx, uc.vectorStore)
		if uc.logger != nil {
			uc.logger.Warn("Failed to add candidate to vector store, continuing with SQLite only",
				"error", err,
				"repair_error", repairErr,
				"fingerprint_id", fp.ID.String(),
			)
		}
	}

	// 4b-6. Rebuild every derived index from the committed memory.
	reindexMemoryDerivedData(ctx, uc.repository, fp, verbatim, input.Content, uc.causalDetector, uc.logger)

	// 4c. Auto-compress session notes (non-fatal, async)
	if uc.autoCompressEnabled &&
		fp.Type == valueobjects.TypeSessionNote &&
		verbatim.TokenCount >= uc.autoCompressMinTok {
		go func(id uuid.UUID, content string, tokenCount int) {
			summary := CompressText(content)
			summaryTokens := EstimateSummaryTokens(summary)
			if summaryTokens < tokenCount {
				if err := uc.repository.UpdateVerbatimSummary(context.Background(), id, summary, summaryTokens); err != nil {
					if uc.logger != nil {
						uc.logger.Warn("Auto-compress: failed to store summary",
							"error", err,
							"verbatim_id", id.String(),
						)
					}
				}
			}
		}(verbatim.ID, verbatim.Content, verbatim.TokenCount)
	}

	// Record metrics if collector is available
	if uc.metricsCollector != nil {
		uc.metricsCollector.RecordStore(time.Since(start))
		uc.metricsCollector.RecordStoreResult(fp.FactCount)
	}

	return &StoreMemoryOutput{
		FingerprintID: fp.ID.String(),
		Type:          string(fp.Type),
		Kind:          string(verbatim.Kind),
		FactCount:     fp.FactCount,
		TokenCount:    verbatim.TokenCount,
		ModelHash:     fp.ModelHash,
	}, nil
}

// storeTags extracts and stores tags for a memory. Non-fatal.
func (uc *StoreMemory) storeTags(ctx context.Context, verbatimID uuid.UUID, fp *entities.Fingerprint, content string) {
	if err := storeMemoryTags(ctx, uc.repository, verbatimID, fp, content); err != nil && uc.logger != nil {
		uc.logger.Warn("Failed to store tags",
			"error", err,
			"verbatim_id", verbatimID.String(),
		)
	}
}

// storeMemoryTags persists deterministic tags derived from a fingerprint.
// It is shared by normal storage and consolidation so synthesized facts
// participate in the same tag-assisted recall as every other MIRA memory.
func storeMemoryTags(ctx context.Context, repository ports.TagRepository, verbatimID uuid.UUID, fp *entities.Fingerprint, content string) error {
	tagSet := make(map[string]bool)

	// Entities
	for _, e := range fp.Entities {
		e = normalizeMemoryTag(e)
		if len(e) >= 2 {
			tagSet[e] = true
		}
	}

	// Subjects from fp.Data
	for _, s := range fp.Data.Subject {
		s = normalizeMemoryTag(s)
		if len(s) >= 2 {
			tagSet[s] = true
		}
	}

	// Simple keyword extraction (words > 5 chars)
	clean := punctuationRe.ReplaceAllString(content, " ")
	words := strings.Fields(clean)
	for _, w := range words {
		lw := strings.ToLower(w)
		if len(lw) >= 5 && !stopWords[lw] {
			tagSet[lw] = true
		}
	}

	if len(tagSet) == 0 {
		return nil
	}

	var tags []string
	for t := range tagSet {
		tags = append(tags, t)
	}
	sort.Strings(tags)

	return repository.StoreTags(ctx, verbatimID, tags, "keyword")
}
