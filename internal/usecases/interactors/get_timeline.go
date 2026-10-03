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
	Items      []*valueobjects.TimelineItem `json:"items"`
	NextCursor *string                      `json:"next_cursor,omitempty"`
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
	if input.Type != nil && !input.Type.IsValid() {
		return nil, fmt.Errorf("invalid memory type %q", *input.Type)
	}
	if input.Since != nil {
		if _, err := valueobjects.ParseTimelineBound(*input.Since, false); err != nil {
			return nil, fmt.Errorf("invalid since value %q: %w", *input.Since, err)
		}
	}
	if input.Until != nil {
		if _, err := valueobjects.ParseTimelineBound(*input.Until, true); err != nil {
			return nil, fmt.Errorf("invalid until value %q: %w", *input.Until, err)
		}
	}
	if input.Cursor != nil && *input.Cursor != "" {
		if _, _, _, err := valueobjects.ParseTimelineCursor(*input.Cursor); err != nil {
			return nil, fmt.Errorf("invalid timeline cursor: %w", err)
		}
	}

	items, err := uc.statsRepo.GetTimeline(ctx, input.Wing, input.Room, input.Type, input.Since, input.Until, input.Limit, input.Cursor)
	if err != nil {
		return nil, fmt.Errorf("failed to get timeline: %w", err)
	}

	output := &GetTimelineOutput{Items: items}
	if len(items) > 0 {
		lastItem := items[len(items)-1]
		cursorID := lastItem.CursorID
		if cursorID == "" {
			cursorID = lastItem.ID
		}
		timestamp, err := time.Parse(time.RFC3339Nano, lastItem.CursorTimestamp)
		if err != nil {
			timestamp, err = time.Parse(time.RFC3339Nano, lastItem.Timestamp)
		}
		if err != nil {
			timestamp, err = time.Parse("2006-01-02 15:04", lastItem.Timestamp)
		}
		if err == nil {
			cursor := valueobjects.FormatTimelineCursor(timestamp, cursorID)
			output.NextCursor = &cursor
		}
	}

	return output, nil
}
