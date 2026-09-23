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
		v.LifecycleState = entities.LifecycleActive
	}
	if value, ok := v.Metadata["lifecycle_state"].(string); ok && value != "" {
		v.LifecycleState = value
	}
	if value, ok := v.Metadata["superseded_by"].(string); ok {
		if parsed, err := uuid.Parse(value); err == nil {
			v.SupersededBy = &parsed
		}
	}
}

func updateLifecycle(ctx context.Context, db *sql.DB, id uuid.UUID, state string, supersededBy *uuid.UUID, postgres bool) error {
	row := db.QueryRowContext(ctx, func() string {
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
	_, err = db.ExecContext(ctx, query, args...)
	return err
}

func (r *SQLiteRepository) SetVerbatimLifecycle(ctx context.Context, id uuid.UUID, state string, supersededBy *uuid.UUID) error {
	return updateLifecycle(ctx, r.db, id, state, supersededBy, false)
}

func (r *PostgreSQLRepository) SetVerbatimLifecycle(ctx context.Context, id uuid.UUID, state string, supersededBy *uuid.UUID) error {
	return updateLifecycle(ctx, r.db, id, state, supersededBy, true)
}
