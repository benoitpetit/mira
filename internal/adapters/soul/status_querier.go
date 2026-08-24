// Package soul provides adapters that bridge MIRA to SOUL.
package soul

import (
	"context"
	"time"

	"github.com/benoitpetit/mira/internal/usecases/interactors"
	soul "github.com/benoitpetit/soul"
)

// StatusQuerier wraps soul.Application to implement rest.SoulStatusQuerier.
// It is injected into the REST handler only when SOUL is enabled.
type StatusQuerier struct {
	app *soul.Application
}

// NewStatusQuerier creates a StatusQuerier backed by the given SOUL application.
func NewStatusQuerier(app *soul.Application) *StatusQuerier {
	return &StatusQuerier{app: app}
}

// QueryStatus lists all known agents and builds a lightweight summary of each.
func (q *StatusQuerier) QueryStatus(ctx context.Context) (*interactors.SoulStatusSummary, error) {
	agents, err := q.app.ListAgents(ctx)
	if err != nil {
		return nil, err
	}

	summaries := make([]interactors.SoulAgentSummary, 0, len(agents))
	for _, agentID := range agents {
		// Fetch the most recent snapshot (limit=1).
		history, err := q.app.GetIdentityHistory(ctx, agentID, 1)
		if err != nil || len(history) == 0 {
			summaries = append(summaries, interactors.SoulAgentSummary{AgentID: agentID})
			continue
		}
		snap := history[0]

		lastCapture := ""
		if !snap.CreatedAt.IsZero() {
			lastCapture = snap.CreatedAt.UTC().Format(time.RFC3339)
		}

		summaries = append(summaries, interactors.SoulAgentSummary{
			AgentID:         agentID,
			Version:         snap.Version,
			ConfidenceScore: snap.ConfidenceScore,
			TraitCount:      len(snap.PersonalityTraits),
			LastCapture:     lastCapture,
		})
	}

	return &interactors.SoulStatusSummary{
		Enabled:    true,
		AgentCount: len(agents),
		Agents:     summaries,
	}, nil
}
