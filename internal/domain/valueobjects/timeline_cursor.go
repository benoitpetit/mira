package valueobjects

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const timelineCursorVersion = "v1:"

// ParseTimelineBound accepts RFC3339 timestamps and ISO calendar dates.
// Date-only lower bounds start at UTC midnight; upper bounds include the full day.
func ParseTimelineBound(value string, upperBound bool) (time.Time, error) {
	timestamp, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return timestamp.UTC(), nil
	}

	timestamp, dateErr := time.Parse("2006-01-02", value)
	if dateErr != nil {
		return time.Time{}, fmt.Errorf("expected RFC3339 timestamp or YYYY-MM-DD date: %w", err)
	}
	if upperBound {
		timestamp = timestamp.AddDate(0, 0, 1).Add(-time.Nanosecond)
	}
	return timestamp.UTC(), nil
}

// TimelineBoundUnixSeconds converts a bound to the precision stored in extracted_at.
// A fractional inclusive lower bound must round up; upper bounds round down via Unix.
func TimelineBoundUnixSeconds(timestamp time.Time, upperBound bool) float64 {
	seconds := timestamp.Unix()
	if !upperBound && timestamp.Nanosecond() > 0 {
		seconds++
	}
	return float64(seconds)
}

// FormatTimelineCursor creates a versioned cursor with the complete timestamp and row ID.
func FormatTimelineCursor(timestamp time.Time, id string) string {
	return timelineCursorVersion + timestamp.UTC().Format(time.RFC3339Nano) + "|" + id
}

// ParseTimelineCursor returns legacy=true for timestamp-only RFC3339 cursors.
// Such cursors cannot resume within an equal-timestamp group because they contain no ID.
func ParseTimelineCursor(value string) (timestamp time.Time, id string, legacy bool, err error) {
	if strings.HasPrefix(value, timelineCursorVersion) {
		parts := strings.SplitN(strings.TrimPrefix(value, timelineCursorVersion), "|", 2)
		if len(parts) != 2 || parts[0] == "" {
			return time.Time{}, "", false, fmt.Errorf("malformed v1 timeline cursor")
		}
		timestamp, err = time.Parse(time.RFC3339Nano, parts[0])
		if err != nil {
			return time.Time{}, "", false, fmt.Errorf("malformed timestamp in timeline cursor: %w", err)
		}
		parsedID, idErr := uuid.Parse(parts[1])
		if idErr != nil {
			return time.Time{}, "", false, fmt.Errorf("malformed ID in timeline cursor: %w", idErr)
		}
		return timestamp.UTC(), parsedID.String(), false, nil
	}

	timestamp, err = time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, "", false, fmt.Errorf("expected versioned or legacy RFC3339 timeline cursor: %w", err)
	}
	return timestamp.UTC(), "", true, nil
}
