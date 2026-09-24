package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/google/uuid"
)

func beliefSourcesJSON(sources []uuid.UUID) ([]byte, error) {
	values := make([]string, 0, len(sources))
	for _, source := range sources {
		values = append(values, source.String())
	}
	return json.Marshal(values)
}

func parseBeliefSources(raw []byte) []uuid.UUID {
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return []uuid.UUID{}
	}
	sources := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		if source, err := uuid.Parse(value); err == nil {
			sources = append(sources, source)
		}
	}
	return sources
}

func validateBeliefFeedback(feedback entities.BeliefFeedback) error {
	switch feedback {
	case entities.FeedbackUseful, entities.FeedbackStale, entities.FeedbackContradictory, entities.FeedbackIrrelevant:
		return nil
	default:
		return fmt.Errorf("unsupported belief feedback %q", feedback)
	}
}

func beliefCalibration(counts map[entities.BeliefFeedback]int) float64 {
	useful := counts[entities.FeedbackUseful]
	negative := counts[entities.FeedbackStale] + counts[entities.FeedbackContradictory] + counts[entities.FeedbackIrrelevant]
	total := useful + negative
	if total == 0 {
		return 1
	}
	value := 1 + 0.2*float64(useful-negative)/float64(total)
	if value < 0.75 {
		return 0.75
	}
	if value > 1.2 {
		return 1.2
	}
	return value
}

func (r *SQLiteRepository) UpsertBelief(ctx context.Context, belief *entities.Belief) error {
	if belief.UpdatedAt.IsZero() {
		belief.UpdatedAt = time.Now().UTC()
	}
	sources, err := beliefSourcesJSON(belief.Sources)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO beliefs
 (id, subject, predicate, value, valid_from, valid_until, confidence, sources, status, updated_at)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
 ON CONFLICT(id) DO UPDATE SET subject=excluded.subject, predicate=excluded.predicate, value=excluded.value,
 valid_from=excluded.valid_from, valid_until=excluded.valid_until, confidence=excluded.confidence,
 sources=excluded.sources, status=excluded.status, updated_at=excluded.updated_at`,
		belief.ID.String(), belief.Subject, belief.Predicate, belief.Value, nullableBeliefTime(belief.ValidFrom), nullableBeliefTime(belief.ValidUntil), belief.Confidence, string(sources), belief.Status, belief.UpdatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func nullableBeliefTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func (r *SQLiteRepository) ResolveBelief(ctx context.Context, subject, predicate string, at time.Time) (*entities.Belief, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, subject, predicate, value, valid_from, valid_until, confidence, sources, status, updated_at
 FROM beliefs WHERE LOWER(subject)=LOWER(?) AND LOWER(predicate)=LOWER(?) AND status='active'
 AND (valid_from IS NULL OR valid_from <= ?) AND (valid_until IS NULL OR valid_until >= ?)
 ORDER BY confidence DESC, updated_at DESC LIMIT 1`, subject, predicate, at.UTC().Format(time.RFC3339Nano), at.UTC().Format(time.RFC3339Nano))
	return scanSQLiteBelief(row)
}

type sqliteBeliefScanner interface{ Scan(...any) error }

func scanSQLiteBelief(scanner sqliteBeliefScanner) (*entities.Belief, error) {
	var id string
	var belief entities.Belief
	var validFrom, validUntil, updatedAt sql.NullString
	var sources string
	if err := scanner.Scan(&id, &belief.Subject, &belief.Predicate, &belief.Value, &validFrom, &validUntil, &belief.Confidence, &sources, &belief.Status, &updatedAt); err != nil {
		return nil, err
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	belief.ID = parsedID
	if validFrom.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, validFrom.String)
		if err == nil {
			belief.ValidFrom = &parsed
		}
	}
	if validUntil.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, validUntil.String)
		if err == nil {
			belief.ValidUntil = &parsed
		}
	}
	if updatedAt.Valid {
		belief.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt.String)
	}
	belief.Sources = parseBeliefSources([]byte(sources))
	return &belief, nil
}

func (r *SQLiteRepository) RetractBelief(ctx context.Context, id uuid.UUID, contested bool) error {
	status := entities.BeliefRevoked
	if contested {
		status = entities.BeliefContested
	}
	_, err := r.db.ExecContext(ctx, `UPDATE beliefs SET status=?, updated_at=? WHERE id=?`, status, time.Now().UTC().Format(time.RFC3339Nano), id.String())
	return err
}

func (r *SQLiteRepository) RecordBeliefFeedback(ctx context.Context, id uuid.UUID, feedback entities.BeliefFeedback) error {
	if err := validateBeliefFeedback(feedback); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO belief_feedback (belief_id, feedback, count) VALUES (?, ?, 1)
 ON CONFLICT(belief_id, feedback) DO UPDATE SET count=count+1`, id.String(), feedback)
	return err
}

func (r *SQLiteRepository) CalibrationForSource(sourceID uuid.UUID) float64 {
	rows, err := r.db.Query(`SELECT bf.feedback, bf.count FROM beliefs b JOIN json_each(b.sources) s ON s.value=? JOIN belief_feedback bf ON bf.belief_id=b.id`, sourceID.String())
	if err != nil {
		return 1
	}
	defer rows.Close()
	counts := map[entities.BeliefFeedback]int{}
	for rows.Next() {
		var feedback string
		var count int
		if rows.Scan(&feedback, &count) == nil {
			counts[entities.BeliefFeedback(feedback)] += count
		}
	}
	return beliefCalibration(counts)
}

func (r *PostgreSQLRepository) UpsertBelief(ctx context.Context, belief *entities.Belief) error {
	if belief.UpdatedAt.IsZero() {
		belief.UpdatedAt = time.Now().UTC()
	}
	sources, err := beliefSourcesJSON(belief.Sources)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO beliefs
 (id, subject, predicate, value, valid_from, valid_until, confidence, sources, status, updated_at)
 VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10)
 ON CONFLICT(id) DO UPDATE SET subject=EXCLUDED.subject, predicate=EXCLUDED.predicate, value=EXCLUDED.value,
 valid_from=EXCLUDED.valid_from, valid_until=EXCLUDED.valid_until, confidence=EXCLUDED.confidence,
 sources=EXCLUDED.sources, status=EXCLUDED.status, updated_at=EXCLUDED.updated_at`,
		belief.ID, belief.Subject, belief.Predicate, belief.Value, belief.ValidFrom, belief.ValidUntil, belief.Confidence, string(sources), belief.Status, belief.UpdatedAt.UTC())
	return err
}

func (r *PostgreSQLRepository) ResolveBelief(ctx context.Context, subject, predicate string, at time.Time) (*entities.Belief, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, subject, predicate, value, valid_from, valid_until, confidence, sources, status, updated_at
 FROM beliefs WHERE LOWER(subject)=LOWER($1) AND LOWER(predicate)=LOWER($2) AND status='active'
 AND (valid_from IS NULL OR valid_from <= $3) AND (valid_until IS NULL OR valid_until >= $3)
 ORDER BY confidence DESC, updated_at DESC LIMIT 1`, subject, predicate, at.UTC())
	var belief entities.Belief
	var validFrom, validUntil sql.NullTime
	var sources []byte
	if err := row.Scan(&belief.ID, &belief.Subject, &belief.Predicate, &belief.Value, &validFrom, &validUntil, &belief.Confidence, &sources, &belief.Status, &belief.UpdatedAt); err != nil {
		return nil, err
	}
	if validFrom.Valid {
		belief.ValidFrom = &validFrom.Time
	}
	if validUntil.Valid {
		belief.ValidUntil = &validUntil.Time
	}
	belief.Sources = parseBeliefSources(sources)
	return &belief, nil
}

func (r *PostgreSQLRepository) RetractBelief(ctx context.Context, id uuid.UUID, contested bool) error {
	status := entities.BeliefRevoked
	if contested {
		status = entities.BeliefContested
	}
	_, err := r.db.ExecContext(ctx, `UPDATE beliefs SET status=$1, updated_at=$2 WHERE id=$3`, status, time.Now().UTC(), id)
	return err
}

func (r *PostgreSQLRepository) RecordBeliefFeedback(ctx context.Context, id uuid.UUID, feedback entities.BeliefFeedback) error {
	if err := validateBeliefFeedback(feedback); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO belief_feedback (belief_id, feedback, count) VALUES ($1, $2, 1)
 ON CONFLICT(belief_id, feedback) DO UPDATE SET count=belief_feedback.count+1`, id, feedback)
	return err
}

func (r *PostgreSQLRepository) CalibrationForSource(sourceID uuid.UUID) float64 {
	rows, err := r.db.Query(`SELECT bf.feedback, bf.count FROM beliefs b CROSS JOIN LATERAL jsonb_array_elements_text(b.sources) s(value)
 JOIN belief_feedback bf ON bf.belief_id=b.id WHERE s.value=$1`, sourceID.String())
	if err != nil {
		return 1
	}
	defer rows.Close()
	counts := map[entities.BeliefFeedback]int{}
	for rows.Next() {
		var feedback string
		var count int
		if rows.Scan(&feedback, &count) == nil {
			counts[entities.BeliefFeedback(feedback)] += count
		}
	}
	return beliefCalibration(counts)
}

var _ interface {
	UpsertBelief(context.Context, *entities.Belief) error
	ResolveBelief(context.Context, string, string, time.Time) (*entities.Belief, error)
	RetractBelief(context.Context, uuid.UUID, bool) error
	RecordBeliefFeedback(context.Context, uuid.UUID, entities.BeliefFeedback) error
	CalibrationForSource(uuid.UUID) float64
} = (*SQLiteRepository)(nil)

var _ interface {
	UpsertBelief(context.Context, *entities.Belief) error
	ResolveBelief(context.Context, string, string, time.Time) (*entities.Belief, error)
	RetractBelief(context.Context, uuid.UUID, bool) error
	RecordBeliefFeedback(context.Context, uuid.UUID, entities.BeliefFeedback) error
	CalibrationForSource(uuid.UUID) float64
} = (*PostgreSQLRepository)(nil)
