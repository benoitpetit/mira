package interactors

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/usecases/ports"
)

// reindexMemoryDerivedData rebuilds tags and causal data after the
// authoritative T0/T1/T2 records have been committed. Store, update, and
// consolidation all use this path so their derived indexes stay consistent.
func reindexMemoryDerivedData(ctx context.Context, repository ports.Repository, fp *entities.Fingerprint, verbatim *entities.Verbatim, content string, causalDetector ports.CausalRelationDetector, logger ports.Logger) {
	if err := storeMemoryTags(ctx, repository, verbatim.ID, fp, content); err != nil {
		warnDerivedIndex(logger, "failed to store memory tags", "error", err, "verbatim_id", verbatim.ID.String())
	}

	summary := fp.Data.Decision
	if summary == "" && len(fp.Data.Subject) > 0 {
		summary = fp.Data.Subject[0]
	}
	if summary == "" {
		summary = fmt.Sprintf("Memory %s", verbatim.ID.String()[:8])
	}
	node := entities.NewCausalNode(fp.ID, string(fp.Type), summary, verbatim.Wing, verbatim.Room)
	if err := repository.AddNode(ctx, node); err != nil {
		warnDerivedIndex(logger, "failed to create causal node", "error", err, "fingerprint_id", fp.ID.String())
		return
	}

	if causalDetector == nil {
		return
	}
	lookback := 50
	if configured, ok := causalDetector.(interface{ CausalLookback() int }); ok && configured.CausalLookback() > 0 {
		lookback = configured.CausalLookback()
	}
	recentFps, err := repository.GetRecentFingerprintsByWing(ctx, verbatim.Wing, fp.ID, lookback)
	if err != nil {
		warnDerivedIndex(logger, "failed to get recent fingerprints for causal detection", "error", err, "wing", verbatim.Wing)
		return
	}
	if len(recentFps) == 0 {
		return
	}
	edges, err := causalDetector.DetectCausalRelations(ctx, fp, recentFps, content)
	if err != nil {
		warnDerivedIndex(logger, "failed to detect causal relations", "error", err, "fingerprint_id", fp.ID.String())
		return
	}
	for _, edge := range edges {
		if edge == nil {
			continue
		}
		if err := repository.AddEdge(ctx, edge); err != nil {
			warnDerivedIndex(logger, "failed to add causal edge", "error", err, "from_id", edge.FromID.String(), "to_id", edge.ToID.String(), "relation", string(edge.Relation))
		}
	}
}

func warnDerivedIndex(logger ports.Logger, msg string, keyValues ...interface{}) {
	if logger != nil {
		logger.Warn(msg, keyValues...)
		return
	}
	args := make([]any, len(keyValues))
	copy(args, keyValues)
	slog.Warn(msg, args...)
}

func normalizeMemoryTag(tag string) string {
	return strings.ToLower(strings.TrimSpace(tag))
}
