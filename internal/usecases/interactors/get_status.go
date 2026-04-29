// GetStatus use case
package interactors

import (
	"context"
	"fmt"
	"time"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
)

// GetStatusOutput contains the output of getting status
type GetStatusOutput struct {
	Stats   *valueobjects.Stats
	Models  []string
	Version string
	Uptime  string
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
