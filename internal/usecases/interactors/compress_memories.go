// Package interactors provides the application use cases.
package interactors

import (
	"context"
	"fmt"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

// compressVerbatimReader is the read side needed by CompressMemories.
type compressVerbatimReader interface {
	GetTimeline(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string, limit int, cursor *string) ([]*valueobjects.TimelineItem, error)
	GetVerbatimByID(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error)
}

// compressVerbatimWriter is the write side needed by CompressMemories.
type compressVerbatimWriter interface {
	UpdateVerbatimSummary(ctx context.Context, id uuid.UUID, summary string, summaryTokens int) error
}

// CompressMemoriesInput contains the input for the compress use case.
type CompressMemoriesInput struct {
	// Wing optionally filters to a single wing. Empty string = all wings.
	Wing string
	// MinTokens is the minimum token count required to compress a verbatim.
	// Verbatims with fewer tokens are skipped. Default 100 when <= 0.
	MinTokens int
	// DryRun counts candidates without persisting summaries.
	DryRun bool
}

// CompressMemoriesOutput contains compression statistics.
type CompressMemoriesOutput struct {
	CompressedCount int `json:"compressed_count"`
	TokensSaved     int `json:"tokens_saved"`
}

// CompressMemories generates rule-based compressed summaries for session_note
// verbatims that exceed MinTokens and do not yet have a summary.
type CompressMemories struct {
	reader compressVerbatimReader
	writer compressVerbatimWriter
}

// NewCompressMemories creates a new CompressMemories interactor.
// reader and writer are typically the same *SQLiteRepository.
func NewCompressMemories(reader compressVerbatimReader, writer compressVerbatimWriter) *CompressMemories {
	return &CompressMemories{reader: reader, writer: writer}
}

// Execute scans session_notes in the given wing (or all wings when Wing == ""),
// compresses those above MinTokens that have no existing summary, and persists
// the results unless DryRun is set.
func (uc *CompressMemories) Execute(ctx context.Context, input CompressMemoriesInput) (*CompressMemoriesOutput, error) {
	minTokens := input.MinTokens
	if minTokens <= 0 {
		minTokens = 100
	}

	memType := valueobjects.TypeSessionNote
	items, err := uc.reader.GetTimeline(ctx, input.Wing, nil, &memType, nil, nil, 1000, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch session notes: %w", err)
	}

	out := &CompressMemoriesOutput{}

	for _, item := range items {
		uid, err := uuid.Parse(item.ID)
		if err != nil {
			continue
		}

		v, err := uc.reader.GetVerbatimByID(ctx, uid)
		if err != nil {
			continue
		}

		// Skip: already compressed
		if v.HasSummary() {
			continue
		}

		// Skip: too short to bother
		if v.TokenCount < minTokens {
			continue
		}

		summary := CompressText(v.Content)
		summaryTokens := EstimateSummaryTokens(summary)

		// Only worthwhile if it's actually shorter
		if summaryTokens >= v.TokenCount {
			continue
		}

		out.CompressedCount++
		out.TokensSaved += v.TokenCount - summaryTokens

		if !input.DryRun {
			_ = uc.writer.UpdateVerbatimSummary(ctx, uid, summary, summaryTokens)
		}
	}

	return out, nil
}
