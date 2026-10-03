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
	seen := make(map[uuid.UUID]struct{}, len(sources))
	for _, source := range sources {
		if _, ok := seen[source]; ok {
			continue
		}
		seen[source] = struct{}{}
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
	return upsertBeliefWithSources(ctx, r.db, belief, false)
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

type beliefSourceSQL interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// SyncBeliefSourceTx makes a source's belief projection authoritative in the
// caller's SQL transaction. A nil belief unlinks the source from its previous
// assertion. The source relation is normalized; beliefs.sources remains a
// compatibility projection of currently active supports.
func (r *SQLiteRepository) SyncBeliefSourceTx(ctx context.Context, tx *sql.Tx, sourceID uuid.UUID, belief *entities.Belief) error {
	return syncBeliefSource(ctx, tx, sourceID, belief, false)
}

func (r *PostgreSQLRepository) SyncBeliefSourceTx(ctx context.Context, tx *sql.Tx, sourceID uuid.UUID, belief *entities.Belief) error {
	return syncBeliefSource(ctx, tx, sourceID, belief, true)
}

func syncBeliefSource(ctx context.Context, tx beliefSourceSQL, sourceID uuid.UUID, belief *entities.Belief, postgres bool) error {
	selectQuery := `SELECT belief_id FROM belief_sources WHERE source_id=?`
	deleteQuery := `DELETE FROM belief_sources WHERE source_id=?`
	if postgres {
		selectQuery = `SELECT belief_id::text FROM belief_sources WHERE source_id=$1`
		deleteQuery = `DELETE FROM belief_sources WHERE source_id=$1`
	}
	rows, err := tx.QueryContext(ctx, selectQuery, beliefIDParam(sourceID, postgres))
	if err != nil {
		return err
	}
	affected := map[string]struct{}{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		affected[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	if _, err := tx.ExecContext(ctx, deleteQuery, beliefIDParam(sourceID, postgres)); err != nil {
		return err
	}
	if belief != nil {
		lifeQuery := `SELECT COALESCE(lifecycle_state,'active') FROM verbatim WHERE id=?`
		lifeArgs := []any{sourceID[:]}
		if postgres {
			lifeQuery = `SELECT COALESCE(lifecycle_state,'active') FROM verbatim WHERE id=$1`
			lifeArgs = []any{sourceID}
		}
		var lifecycle string
		if err := tx.QueryRowContext(ctx, lifeQuery, lifeArgs...).Scan(&lifecycle); err != nil {
			return fmt.Errorf("belief source %s does not exist: %w", sourceID, err)
		}
		beliefToPersist := *belief
		if existingID, found, err := findEquivalentBeliefID(ctx, tx, belief, postgres); err != nil {
			return err
		} else if found {
			beliefToPersist.ID = existingID
		}
		if err := upsertBeliefTx(ctx, tx, &beliefToPersist, postgres); err != nil {
			return err
		}
		beliefID := beliefToPersist.ID.String()
		active := belief.Status == entities.BeliefActive && lifecycle == entities.LifecycleActive
		activeSQLite := 0
		if active {
			activeSQLite = 1
		}
		insertQuery := `INSERT INTO belief_sources(belief_id, source_id, active) VALUES (?, ?, ?) ON CONFLICT(belief_id, source_id) DO UPDATE SET active=excluded.active`
		args := []any{beliefToPersist.ID.String(), sourceID.String(), activeSQLite}
		if postgres {
			insertQuery = `INSERT INTO belief_sources(belief_id, source_id, active) VALUES ($1, $2, $3) ON CONFLICT(belief_id, source_id) DO UPDATE SET active=EXCLUDED.active`
			args = []any{beliefToPersist.ID, sourceID, active}
		}
		if _, err := tx.ExecContext(ctx, insertQuery, args...); err != nil {
			return err
		}
		affected[beliefID] = struct{}{}
	}
	for id := range affected {
		if err := refreshBeliefProjection(ctx, tx, id, postgres); err != nil {
			return err
		}
	}
	return nil
}

func findEquivalentBeliefID(ctx context.Context, tx beliefSourceSQL, belief *entities.Belief, postgres bool) (uuid.UUID, bool, error) {
	query := `SELECT id FROM beliefs WHERE LOWER(subject)=LOWER(?) AND LOWER(predicate)=LOWER(?) AND LOWER(value)=LOWER(?)
	 AND ((valid_from=? ) OR (valid_from IS NULL AND ? IS NULL))
	 AND ((valid_until=? ) OR (valid_until IS NULL AND ? IS NULL)) ORDER BY updated_at DESC LIMIT 1`
	validFrom, validUntil := nullableBeliefTime(belief.ValidFrom), nullableBeliefTime(belief.ValidUntil)
	args := []any{belief.Subject, belief.Predicate, belief.Value, validFrom, validFrom, validUntil, validUntil}
	if postgres {
		query = `SELECT id::text FROM beliefs WHERE LOWER(subject)=LOWER($1) AND LOWER(predicate)=LOWER($2) AND LOWER(value)=LOWER($3)
		 AND valid_from IS NOT DISTINCT FROM $4 AND valid_until IS NOT DISTINCT FROM $5 ORDER BY updated_at DESC LIMIT 1`
		args = []any{belief.Subject, belief.Predicate, belief.Value, belief.ValidFrom, belief.ValidUntil}
	}
	var idText string
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&idText); err != nil {
		if err == sql.ErrNoRows {
			return uuid.Nil, false, nil
		}
		return uuid.Nil, false, err
	}
	id, err := uuid.Parse(idText)
	return id, err == nil, err
}

func beliefIDParam(id uuid.UUID, postgres bool) any {
	if postgres {
		return id
	}
	return id.String()
}

func upsertBeliefTx(ctx context.Context, tx beliefSourceSQL, belief *entities.Belief, postgres bool) error {
	if belief.UpdatedAt.IsZero() {
		belief.UpdatedAt = time.Now().UTC()
	}
	sources, err := beliefSourcesJSON(belief.Sources)
	if err != nil {
		return err
	}
	query := `INSERT INTO beliefs(id,subject,predicate,value,valid_from,valid_until,confidence,sources,status,updated_at)
	 VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET subject=excluded.subject,predicate=excluded.predicate,value=excluded.value,
	 valid_from=excluded.valid_from,valid_until=excluded.valid_until,confidence=excluded.confidence,sources=excluded.sources,status=excluded.status,updated_at=excluded.updated_at`
	args := []any{belief.ID.String(), belief.Subject, belief.Predicate, belief.Value, nullableBeliefTime(belief.ValidFrom), nullableBeliefTime(belief.ValidUntil), belief.Confidence, string(sources), belief.Status, belief.UpdatedAt.UTC().Format(time.RFC3339Nano)}
	if postgres {
		query = `INSERT INTO beliefs(id,subject,predicate,value,valid_from,valid_until,confidence,sources,status,updated_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10) ON CONFLICT(id) DO UPDATE SET subject=EXCLUDED.subject,predicate=EXCLUDED.predicate,value=EXCLUDED.value,
		 valid_from=EXCLUDED.valid_from,valid_until=EXCLUDED.valid_until,confidence=EXCLUDED.confidence,sources=EXCLUDED.sources,status=EXCLUDED.status,updated_at=EXCLUDED.updated_at`
		args = []any{belief.ID, belief.Subject, belief.Predicate, belief.Value, belief.ValidFrom, belief.ValidUntil, belief.Confidence, string(sources), belief.Status, belief.UpdatedAt.UTC()}
	}
	_, err = tx.ExecContext(ctx, query, args...)
	return err
}

func refreshBeliefProjection(ctx context.Context, tx beliefSourceSQL, beliefID string, postgres bool) error {
	query := `SELECT source_id FROM belief_sources WHERE belief_id=? AND active=1 ORDER BY source_id`
	if postgres {
		query = `SELECT source_id::text FROM belief_sources WHERE belief_id=$1 AND active=TRUE ORDER BY source_id`
	}
	rows, err := tx.QueryContext(ctx, query, beliefProjectionID(beliefID, postgres))
	if err != nil {
		return err
	}
	sources := make([]string, 0)
	for rows.Next() {
		var source string
		if err := rows.Scan(&source); err != nil {
			_ = rows.Close()
			return err
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	payload, err := json.Marshal(sources)
	if err != nil {
		return err
	}
	status := entities.BeliefRevoked
	if len(sources) > 0 {
		status = entities.BeliefActive
	}
	updateQuery := `UPDATE beliefs SET sources=?,status=?,updated_at=? WHERE id=?`
	args := []any{string(payload), status, time.Now().UTC().Format(time.RFC3339Nano), beliefProjectionID(beliefID, false)}
	if postgres {
		updateQuery = `UPDATE beliefs SET sources=$1::jsonb,status=$2,updated_at=$3 WHERE id=$4`
		args = []any{string(payload), status, time.Now().UTC(), beliefProjectionID(beliefID, true)}
	}
	_, err = tx.ExecContext(ctx, updateQuery, args...)
	return err
}

func beliefProjectionID(id string, postgres bool) any {
	if postgres {
		parsed, _ := uuid.Parse(id)
		return parsed
	}
	return id
}

func setBeliefSourceActivity(ctx context.Context, tx beliefSourceSQL, sourceID uuid.UUID, active, postgres bool) error {
	query := `UPDATE belief_sources SET active=? WHERE source_id=?`
	args := []any{active, sourceID.String()}
	if !postgres {
		if active {
			args[0] = 1
		} else {
			args[0] = 0
		}
	} else {
		query = `UPDATE belief_sources SET active=$1 WHERE source_id=$2`
		args = []any{active, sourceID}
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	query = `SELECT belief_id FROM belief_sources WHERE source_id=?`
	if postgres {
		query = `SELECT belief_id::text FROM belief_sources WHERE source_id=$1`
	}
	rows, err := tx.QueryContext(ctx, query, beliefIDParam(sourceID, postgres))
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := refreshBeliefProjection(ctx, tx, id, postgres); err != nil {
			return err
		}
	}
	return nil
}

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
	return upsertBeliefWithSources(ctx, r.db, belief, true)
}

func upsertBeliefWithSources(ctx context.Context, db *sql.DB, belief *entities.Belief, postgres bool) error {
	if len(belief.Sources) == 0 {
		return fmt.Errorf("belief must have at least one source")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit
	for _, sourceID := range belief.Sources {
		if err := syncBeliefSource(ctx, tx, sourceID, belief, postgres); err != nil {
			return err
		}
	}
	return tx.Commit()
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
