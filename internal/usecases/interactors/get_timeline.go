// GetTimeline use case
package interactors

import (
	"context"
	"fmt"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
)

// GetTimelineInput contains the input for getting timeline
type GetTimelineInput struct {
	Wing  string
	Room  *string
	Type  *valueobjects.MemoryType
	Since *string
	Until *string
}

// GetTimelineOutput contains the output of getting timeline
type GetTimelineOutput struct {
	Items []*valueobjects.TimelineItem
}

// GetTimeline implements the get timeline use case
type GetTimeline struct {
	statsRepo ports.StatsRepository
}

// NewGetTimeline creates a new get timeline interactor
func NewGetTimeline(statsRepo ports.StatsRepository) *GetTimeline {
	return &GetTimeline{
		statsRepo: statsRepo,
	}
}

// Execute retrieves the timeline
func (uc *GetTimeline) Execute(ctx context.Context, input GetTimelineInput) (*GetTimelineOutput, error) {
	items, err := uc.statsRepo.GetTimeline(ctx, input.Wing, input.Room, input.Type, input.Since, input.Until)
	if err != nil {
		return nil, fmt.Errorf("failed to get timeline: %w", err)
	}

	return &GetTimelineOutput{
		Items: items,
	}, nil
}
