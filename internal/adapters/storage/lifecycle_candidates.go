package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

// ListActiveConsolidationCandidates returns only active session notes. Timeline
// deliberately remains a historical query and must not be used to select work
// for a state-changing consolidation operation.
func (r *SQLiteRepository) ListActiveConsolidationCandidates(ctx context.Context, wing string) ([]*valueobjects.TimelineItem, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT v.id, f.id, f.ftype, f.extracted_at, f.data, v.wing, COALESCE(v.lifecycle_state, 'active')
		FROM fingerprints f JOIN verbatim v ON f.verbatim_id=v.id
		WHERE f.ftype=? AND v.wing=? AND COALESCE(v.lifecycle_state, 'active')='active'
		ORDER BY f.extracted_at DESC, f.id DESC LIMIT 1000`, string(valueobjects.TypeSessionNote), wing)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConsolidationCandidates(rows, false)
}

// ListActiveConsolidationCandidates returns only active session notes. Timeline
// deliberately remains a historical query and must not be used to select work
// for a state-changing consolidation operation.
func (r *PostgreSQLRepository) ListActiveConsolidationCandidates(ctx context.Context, wing string) ([]*valueobjects.TimelineItem, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT v.id, f.id, f.ftype, f.extracted_at, f.data, v.wing, COALESCE(v.lifecycle_state, 'active')
		FROM fingerprints f JOIN verbatim v ON f.verbatim_id=v.id
		WHERE f.ftype=$1 AND v.wing=$2 AND COALESCE(v.lifecycle_state, 'active')='active'
		ORDER BY f.extracted_at DESC, f.id DESC LIMIT 1000`, string(valueobjects.TypeSessionNote), wing)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConsolidationCandidates(rows, true)
}

type consolidationCandidateRows interface {
	Next() bool
	Scan(...any) error
	Err() error
}

func scanConsolidationCandidates(rows consolidationCandidateRows, postgres bool) ([]*valueobjects.TimelineItem, error) {
	items := make([]*valueobjects.TimelineItem, 0)
	for rows.Next() {
		var rawID, rawFingerprintID any
		var typeValue string
		var extractedAt float64
		var dataJSON []byte
		var wing string
		var lifecycleState string
		if err := rows.Scan(&rawID, &rawFingerprintID, &typeValue, &extractedAt, &dataJSON, &wing, &lifecycleState); err != nil {
			return nil, err
		}
		id, err := candidateUUID(rawID, postgres)
		if err != nil {
			return nil, fmt.Errorf("parse consolidation candidate ID: %w", err)
		}
		fingerprintID, err := candidateUUID(rawFingerprintID, postgres)
		if err != nil {
			return nil, fmt.Errorf("parse consolidation fingerprint ID: %w", err)
		}
		items = append(items, makeTimelineItem(id, fingerprintID, typeValue, extractedAt, dataJSON, wing, lifecycleState))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func candidateUUID(value any, postgres bool) (uuid.UUID, error) {
	if postgres {
		id, ok := value.(uuid.UUID)
		if !ok {
			return uuid.Nil, fmt.Errorf("unexpected PostgreSQL UUID type %T", value)
		}
		return id, nil
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
