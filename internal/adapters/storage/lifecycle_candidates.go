package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

// ListActiveConsolidationCandidates returns one page of active session notes.
// Timeline deliberately remains historical and is not a source for state changes.
func (r *SQLiteRepository) ListActiveConsolidationCandidates(ctx context.Context, wing string, limit int, after *valueobjects.ConsolidationCandidateCursor) (valueobjects.ConsolidationCandidatePage, error) {
	if limit <= 0 || limit > maxConsolidationCandidatePageSize {
		limit = maxConsolidationCandidatePageSize
	}
	query := `SELECT v.id, f.id, f.ftype, f.extracted_at, f.data, v.wing, COALESCE(v.lifecycle_state, 'active')
		FROM fingerprints f JOIN verbatim v ON f.verbatim_id=v.id
		WHERE f.ftype=? AND v.wing=? AND COALESCE(v.lifecycle_state, 'active')='active'`
	args := []any{string(valueobjects.TypeSessionNote), wing}
	if after != nil {
		query += ` AND (f.extracted_at < ? OR (f.extracted_at = ? AND f.id < ?))`
		args = append(args, after.ExtractedAt, after.ExtractedAt, after.FingerprintID[:])
	}
	query += ` ORDER BY f.extracted_at DESC, f.id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return valueobjects.ConsolidationCandidatePage{}, err
	}
	defer rows.Close()
	return scanConsolidationCandidatePage(rows, false, limit)
}

// ListActiveConsolidationCandidates returns one page of active session notes.
// Timeline deliberately remains historical and is not a source for state changes.
func (r *PostgreSQLRepository) ListActiveConsolidationCandidates(ctx context.Context, wing string, limit int, after *valueobjects.ConsolidationCandidateCursor) (valueobjects.ConsolidationCandidatePage, error) {
	if limit <= 0 || limit > maxConsolidationCandidatePageSize {
		limit = maxConsolidationCandidatePageSize
	}
	query := `SELECT v.id, f.id, f.ftype, f.extracted_at, f.data, v.wing, COALESCE(v.lifecycle_state, 'active')
		FROM fingerprints f JOIN verbatim v ON f.verbatim_id=v.id
		WHERE f.ftype=$1 AND v.wing=$2 AND COALESCE(v.lifecycle_state, 'active')='active'`
	args := []any{string(valueobjects.TypeSessionNote), wing}
	if after != nil {
		query += ` AND (f.extracted_at < $3 OR (f.extracted_at = $4 AND f.id < $5))`
		args = append(args, after.ExtractedAt, after.ExtractedAt, after.FingerprintID)
	}
	limitPosition := len(args) + 1
	query += fmt.Sprintf(` ORDER BY f.extracted_at DESC, f.id DESC LIMIT $%d`, limitPosition)
	args = append(args, limit+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return valueobjects.ConsolidationCandidatePage{}, err
	}
	defer rows.Close()
	return scanConsolidationCandidatePage(rows, true, limit)
}

const maxConsolidationCandidatePageSize = 1000

type consolidationCandidateRows interface {
	Next() bool
	Scan(...any) error
	Err() error
}

func scanConsolidationCandidatePage(rows consolidationCandidateRows, postgres bool, limit int) (valueobjects.ConsolidationCandidatePage, error) {
	page := valueobjects.ConsolidationCandidatePage{Items: make([]*valueobjects.TimelineItem, 0, limit)}
	hasMore := false
	for rows.Next() {
		var rawID, rawFingerprintID any
		var typeValue string
		var extractedAt float64
		var dataJSON []byte
		var wing string
		var lifecycleState string
		if err := rows.Scan(&rawID, &rawFingerprintID, &typeValue, &extractedAt, &dataJSON, &wing, &lifecycleState); err != nil {
			return valueobjects.ConsolidationCandidatePage{}, err
		}
		if len(page.Items) == limit {
			hasMore = true
			break
		}
		id, err := candidateUUID(rawID, postgres)
		if err != nil {
			return valueobjects.ConsolidationCandidatePage{}, fmt.Errorf("parse consolidation candidate ID: %w", err)
		}
		fingerprintID, err := candidateUUID(rawFingerprintID, postgres)
		if err != nil {
			return valueobjects.ConsolidationCandidatePage{}, fmt.Errorf("parse consolidation fingerprint ID: %w", err)
		}
		page.Items = append(page.Items, makeTimelineItem(id, fingerprintID, typeValue, extractedAt, dataJSON, wing, lifecycleState))
		page.Next = &valueobjects.ConsolidationCandidateCursor{ExtractedAt: extractedAt, FingerprintID: fingerprintID}
	}
	if err := rows.Err(); err != nil {
		return valueobjects.ConsolidationCandidatePage{}, err
	}
	if !hasMore {
		page.Next = nil
	}
	return page, nil
}

func candidateUUID(value any, postgres bool) (uuid.UUID, error) {
	if postgres {
		switch id := value.(type) {
		case uuid.UUID:
			return id, nil
		case []byte:
			if len(id) == 16 {
				return uuid.FromBytes(id)
			}
			parsed, err := uuid.Parse(string(id))
			if err != nil {
				return uuid.Nil, fmt.Errorf("parse PostgreSQL UUID %q: %w", id, err)
			}
			return parsed, nil
		case string:
			parsed, err := uuid.Parse(id)
			if err != nil {
				return uuid.Nil, fmt.Errorf("parse PostgreSQL UUID %q: %w", id, err)
			}
			return parsed, nil
		default:
			return uuid.Nil, fmt.Errorf("unexpected PostgreSQL UUID type %T", value)
		}
	}
	raw, ok := value.([]byte)
	if !ok {
		return uuid.Nil, fmt.Errorf("unexpected SQLite UUID type %T", value)
	}
	return uuid.FromBytes(raw)
}

func makeTimelineItem(id, fingerprintID uuid.UUID, typeValue string, extractedAt float64, dataJSON []byte, wing, lifecycleState string) *valueobjects.TimelineItem {
	var data valueobjects.FingerprintData
	_ = json.Unmarshal(dataJSON, &data)
	summary := ""
	if len(data.Subject) > 0 {
		summary = data.Subject[0]
	}
	if summary == "" && data.Decision != "" {
		summary = data.Decision
	}
	if summary == "" {
		summary = "Memory " + id.String()[:8]
	}
	timestamp := time.Unix(int64(extractedAt), 0)
	return &valueobjects.TimelineItem{
		ID: id.String(), CursorID: fingerprintID.String(),
		Timestamp: timestamp.Format("2006-01-02 15:04"), CursorTimestamp: timestamp.UTC().Format(time.RFC3339Nano),
		Type: valueobjects.MemoryType(typeValue), LifecycleState: lifecycleState, Summary: summary, Wing: wing,
	}
}
