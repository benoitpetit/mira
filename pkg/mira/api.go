package mira

import (
	"context"

	"github.com/benoitpetit/mira/internal/app"
	"github.com/benoitpetit/mira/internal/config"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/interactors"
	"github.com/benoitpetit/soul"
	"github.com/google/uuid"
)

// Application exposes MIRA capabilities to external modules.
type Application struct {
	inner *app.Application
}

// NewApplication creates a MIRA application from config.
func NewApplication(cfg *config.Config) (*Application, error) {
	a, err := app.NewApplication(cfg)
	if err != nil {
		return nil, err
	}
	return &Application{inner: a}, nil
}

// Config is a public alias.
type Config = config.Config

// MemoryType is a public alias.
type MemoryType = valueobjects.MemoryType

// SearchResult is a public alias for semantic search results.
type SearchResult = interactors.SearchSemanticResult

// StoreMemoryOutput is a public alias.
type StoreMemoryOutput = interactors.StoreMemoryOutput

// RecallMemoryOutput is a public alias.
type RecallMemoryOutput = interactors.RecallMemoryOutput

// LoadMemoryOutput is a public alias.
type LoadMemoryOutput = interactors.LoadMemoryOutput

// GetTimelineOutput is a public alias.
type GetTimelineOutput = interactors.GetTimelineOutput

// GetStatusOutput is a public alias.
type GetStatusOutput = interactors.GetStatusOutput

// GetCausalChainOutput is a public alias.
type GetCausalChainOutput = interactors.GetCausalChainOutput

// ArchiveMemoriesOutput is a public alias.
type ArchiveMemoriesOutput = interactors.ArchiveMemoriesOutput

// ClearMemoryOutput is a public alias.
type ClearMemoryOutput = interactors.ClearMemoryOutput

// UpdateMemoryOutput is a public alias.
type UpdateMemoryOutput = interactors.UpdateMemoryOutput

// LoadConfig loads a configuration from a YAML file.
func LoadConfig(path string) (*Config, error) {
	return config.LoadOrDefault(path)
}

// DefaultConfig returns default MIRA configuration.
func DefaultConfig() *Config {
	return config.Default()
}

// Close cleans up resources.
func (a *Application) Close() error {
	return a.inner.Close()
}

// Store stores a memory.
func (a *Application) Store(ctx context.Context, content, wing string, room *string, forcedType *valueobjects.MemoryType) (*interactors.StoreMemoryOutput, error) {
	return a.inner.StoreMemoryUC().Execute(ctx, interactors.StoreMemoryInput{
		Content: content,
		Wing:    wing,
		Room:    room,
		Type:    forcedType,
	})
}

// Recall retrieves context.
func (a *Application) Recall(ctx context.Context, query string, budget int, wing string, room, fallbackWings *string, sessionID *string) (*interactors.RecallMemoryOutput, error) {
	var fw []string
	if fallbackWings != nil {
		fw = append(fw, *fallbackWings)
	}
	return a.inner.RecallMemoryUC().Execute(ctx, interactors.RecallMemoryInput{
		Query:         query,
		Budget:        budget,
		Wing:          &wing,
		Room:          room,
		FallbackWings: fw,
		SessionID:     sessionID,
	})
}

// Load retrieves a verbatim by ID.
func (a *Application) Load(ctx context.Context, id uuid.UUID) (*interactors.LoadMemoryOutput, error) {
	return a.inner.LoadMemoryUC().Execute(ctx, interactors.LoadMemoryInput{ID: id})
}

// GetTimeline returns a chronological timeline.
func (a *Application) GetTimeline(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string, limit int, cursor *string) (*interactors.GetTimelineOutput, error) {
	return a.inner.GetTimelineUC().Execute(ctx, interactors.GetTimelineInput{
		Wing:   wing,
		Room:   room,
		Type:   memType,
		Since:  since,
		Until:  until,
		Limit:  limit,
		Cursor: cursor,
	})
}

// GetStatus returns system statistics.
func (a *Application) GetStatus(ctx context.Context) (*interactors.GetStatusOutput, error) {
	return a.inner.GetStatusUC().Execute(ctx)
}

// GetCausalChain traces causal relations.
func (a *Application) GetCausalChain(ctx context.Context, id uuid.UUID, maxDepth int, includeConsequences bool) (*interactors.GetCausalChainOutput, error) {
	return a.inner.GetCausalChainUC().Execute(ctx, interactors.GetCausalChainInput{
		ID:                  id,
		MaxDepth:            maxDepth,
		IncludeConsequences: includeConsequences,
	})
}

// Archive archives old memories.
func (a *Application) Archive(ctx context.Context) (*interactors.ArchiveMemoriesOutput, error) {
	return a.inner.ArchiveMemoriesUC().Execute(ctx)
}

// Clear removes memories globally or by room.
func (a *Application) Clear(ctx context.Context, wing string, room *string) (*interactors.ClearMemoryOutput, error) {
	mode := "room"
	if wing == "" && room == nil {
		mode = "global"
	}
	return a.inner.ClearMemoryUC().Execute(ctx, interactors.ClearMemoryInput{
		Mode: mode,
		Wing: wing,
		Room: room,
	})
}

// Delete removes a memory by ID.
func (a *Application) Delete(ctx context.Context, id uuid.UUID) error {
	return a.inner.DeleteMemoryUC().Execute(ctx, interactors.DeleteMemoryInput{ID: id})
}

// Search performs a semantic vector search.
func (a *Application) Search(ctx context.Context, query string, topK int, threshold float64) ([]*SearchResult, error) {
	return a.inner.SearchSemanticUC().Execute(ctx, interactors.SearchSemanticInput{
		Query:     query,
		TopK:      topK,
		Threshold: threshold,
	})
}

// Update updates a memory's content and regenerates its fingerprint and embedding.
func (a *Application) Update(ctx context.Context, id uuid.UUID, content string) error {
	_, err := a.inner.UpdateMemoryUC().Execute(ctx, interactors.UpdateMemoryInput{
		ID:      id,
		Content: content,
	})
	return err
}

// Consolidate merges redundant session notes into synthesized facts.
func (a *Application) Consolidate(ctx context.Context, wing string, threshold float64) error {
	_, err := a.inner.ConsolidateMemoriesUC().Execute(ctx, interactors.ConsolidateMemoriesInput{
		Wing:                wing,
		SimilarityThreshold: threshold,
	})
	return err
}

// SoulApp returns the embedded SOUL application if enabled.
func (a *Application) SoulApp() *soul.Application {
	return a.inner.SoulApplication()
}
