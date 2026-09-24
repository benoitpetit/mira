package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/google/uuid"
)

func setLifecycleMetadata(metadata map[string]any, state string, supersededBy *uuid.UUID) map[string]any {
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["lifecycle_state"] = state
	if supersededBy == nil {
		delete(metadata, "superseded_by")
	} else {
		metadata["superseded_by"] = supersededBy.String()
	}
	return metadata
}

func hydrateLifecycle(v *entities.Verbatim) {
	if v == nil {
		return
	}
	if v.LifecycleState == "" {
		if value, ok := v.Metadata["lifecycle_state"].(string); ok && value != "" {
			v.LifecycleState = value
		} else {
			v.LifecycleState = entities.LifecycleActive
		}
	}
	if v.SupersededBy == nil {
		value, _ := v.Metadata["superseded_by"].(string)
		if parsed, err := uuid.Parse(value); err == nil {
			v.SupersededBy = &parsed
		}
	}
}

func hydrateLifecycleColumns(v *entities.Verbatim, state string, supersededBy sql.NullString) {
	if v == nil {
		return
	}
	if state != "" {
		v.LifecycleState = state
	}
	if supersededBy.Valid {
		if parsed, err := uuid.Parse(supersededBy.String); err == nil {
			v.SupersededBy = &parsed
		}
	}
	hydrateLifecycle(v)
}

type lifecycleSQL interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func updateLifecycle(ctx context.Context, execer lifecycleSQL, id uuid.UUID, state string, supersededBy *uuid.UUID, postgres bool) error {
	row := execer.QueryRowContext(ctx, func() string {
		if postgres {
			return `SELECT metadata FROM verbatim WHERE id=$1`
		}
		return `SELECT metadata FROM verbatim WHERE id=?`
	}(), func() any {
		if postgres {
			return id
		}
		return id[:]
	}())
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		return err
	}
	metadata := map[string]any{}
	_ = json.Unmarshal(raw, &metadata)
	metadata = setLifecycleMetadata(metadata, state, supersededBy)
	payload, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	query := `UPDATE verbatim SET metadata=$1 WHERE id=$2`
	args := []any{payload, id}
	if !postgres {
		query, args = `UPDATE verbatim SET metadata=? WHERE id=?`, []any{string(payload), id[:]}
	}
	supersededByValue := any(nil)
	if supersededBy != nil {
		supersededByValue = supersededBy.String()
	}
	if postgres {
		query = `UPDATE verbatim SET metadata=$1, lifecycle_state=$2, superseded_by=$3 WHERE id=$4`
		args = []any{payload, state, supersededByValue, id}
	} else {
		query = `UPDATE verbatim SET metadata=?, lifecycle_state=?, superseded_by=? WHERE id=?`
		args = []any{string(payload), state, supersededByValue, id[:]}
	}
	_, err = execer.ExecContext(ctx, query, args...)
	return err
}

func (r *SQLiteRepository) SetVerbatimLifecycle(ctx context.Context, id uuid.UUID, state string, supersededBy *uuid.UUID) error {
	return updateLifecycle(ctx, r.db, id, state, supersededBy, false)
}

func (r *PostgreSQLRepository) SetVerbatimLifecycle(ctx context.Context, id uuid.UUID, state string, supersededBy *uuid.UUID) error {
	return updateLifecycle(ctx, r.db, id, state, supersededBy, true)
}
