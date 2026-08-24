// GetStatus use case
package interactors

import (
	"context"
	"fmt"
	"time"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
)

// SoulAgentSummary is a brief identity snapshot for one SOUL agent.
type SoulAgentSummary struct {
	AgentID         string  `json:"agent_id"`
	Version         int     `json:"version"`
	ConfidenceScore float64 `json:"confidence_score"`
	TraitCount      int     `json:"trait_count"`
	LastCapture     string  `json:"last_capture"`
	DriftScore      float64 `json:"drift_score"`
}

// SoulStatusSummary is included in GetStatusOutput when SOUL is enabled.
type SoulStatusSummary struct {
	Enabled    bool               `json:"enabled"`
	AgentCount int                `json:"agent_count"`
	Agents     []SoulAgentSummary `json:"agents"`
}

// GetStatusOutput contains the output of getting status
type GetStatusOutput struct {
	Stats   *valueobjects.Stats `json:"stats"`
	Models  []string            `json:"models"`
	Version string              `json:"version"`
	Uptime  string              `json:"uptime"`
	Soul    *SoulStatusSummary  `json:"soul,omitempty"`
}

// GetStatus implements the get status use case
type GetStatus struct {
	statsRepo ports.StatsRepository
	modelRepo ports.ModelRepository
	startTime time.Time
	version   string
}

// NewGetStatus creates a new get status interactor
func NewGetStatus(statsRepo ports.StatsRepository, modelRepo ports.ModelRepository, startTime time.Time, version string) *GetStatus {
	return &GetStatus{
		statsRepo: statsRepo,
		modelRepo: modelRepo,
		startTime: startTime,
		version:   version,
	}
}

// Execute retrieves the system status
func (uc *GetStatus) Execute(ctx context.Context) (*GetStatusOutput, error) {
	stats, err := uc.statsRepo.GetStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	models, err := uc.modelRepo.GetAllModels(ctx)
	if err != nil {
		models = []string{"unknown"}
	}

	uptime := time.Since(uc.startTime).Round(time.Second)

	return &GetStatusOutput{
		Stats:   stats,
		Models:  models,
		Version: uc.version,
		Uptime:  uptime.String(),
	}, nil
}
