// GetTimeline use case
package interactors

import (
	"context"
	"fmt"
	"time"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
)

// GetTimelineInput contains the input for getting timeline
type GetTimelineInput struct {
	Wing   string
	Room   *string
	Type   *valueobjects.MemoryType
	Since  *string
	Until  *string
	Limit  int
	Cursor *string
}

// GetTimelineOutput contains the output of getting timeline
type GetTimelineOutput struct {
	Items      []*valueobjects.TimelineItem
	NextCursor *string
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
	items, err := uc.statsRepo.GetTimeline(ctx, input.Wing, input.Room, input.Type, input.Since, input.Until, input.Limit, input.Cursor)
	if err != nil {
		return nil, fmt.Errorf("failed to get timeline: %w", err)
	}

	output := &GetTimelineOutput{Items: items}
	if len(items) > 0 {
		// Use the timestamp of the last item as the next cursor
		lastItem := items[len(items)-1]
		t, err := time.Parse("2006-01-02 15:04", lastItem.Timestamp)
		if err == nil {
			cursor := t.Format(time.RFC3339)
			output.NextCursor = &cursor
		}
	}

	return output, nil
}
