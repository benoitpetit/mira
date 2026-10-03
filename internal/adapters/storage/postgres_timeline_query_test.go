package storage

import (
	"strings"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

func TestBuildPostgreSQLTimelineQueryUsesNumericBoundsAndStableCursor(t *testing.T) {
	room := "room-a"
	memType := valueobjects.TypeFact
	since := "2026-10-03T16:15:16+02:00"
	until := "2026-10-04"
	id := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	cursor := valueobjects.FormatTimelineCursor(time.Date(2026, 10, 3, 14, 15, 16, 0, time.UTC), id.String())

	query, args, err := buildPostgreSQLTimelineQuery("wing-a", &room, &memType, &since, &until, 7, &cursor)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"f.extracted_at >= $4",
		"f.extracted_at <= $5",
		"f.extracted_at < $6 OR (f.extracted_at = $7 AND v.id < $8)",
		"ORDER BY f.extracted_at DESC, v.id DESC LIMIT 7",
	} {
		if !strings.Contains(query, fragment) {
			t.Errorf("query does not contain %q: %s", fragment, query)
		}
	}
	if len(args) != 8 {
		t.Fatalf("got %d query args, want 8: %#v", len(args), args)
	}
	if args[3] != float64(time.Date(2026, 10, 3, 14, 15, 16, 0, time.UTC).Unix()) {
		t.Errorf("since arg = %#v, want numeric Unix seconds", args[3])
	}
	if args[4] != float64(time.Date(2026, 10, 4, 23, 59, 59, 0, time.UTC).Unix()) {
		t.Errorf("date-only until arg = %#v, want end of UTC day in Unix seconds", args[4])
	}
	if args[5] != args[6] || args[7] != id {
		t.Errorf("cursor args = %#v, want repeated timestamp and UUID tie-breaker", args[5:])
	}
}

func TestBuildPostgreSQLTimelineQueryAcceptsLegacyTimestampCursor(t *testing.T) {
	cursor := "2026-10-03T14:15:16Z"
	query, args, err := buildPostgreSQLTimelineQuery("", nil, nil, nil, nil, 10, &cursor)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "AND f.extracted_at < $1") || strings.Contains(query, "v.id <") {
		t.Fatalf("legacy cursor query = %s, want timestamp-only compatibility predicate", query)
	}
	if len(args) != 1 || args[0] != float64(time.Date(2026, 10, 3, 14, 15, 16, 0, time.UTC).Unix()) {
		t.Fatalf("legacy cursor args = %#v, want numeric timestamp", args)
	}
}
