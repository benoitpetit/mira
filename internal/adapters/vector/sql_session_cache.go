package vector

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// SQLSessionCache persists the IDs selected during recall for a session.
type SQLSessionCache struct {
	db       *sql.DB
	postgres bool
}

func NewSQLSessionCache(db *sql.DB, dialect string) *SQLSessionCache {
	return &SQLSessionCache{db: db, postgres: strings.EqualFold(strings.TrimSpace(dialect), "postgres") || strings.EqualFold(strings.TrimSpace(dialect), "postgresql")}
}

func (s *SQLSessionCache) Load(ctx context.Context, sessionID string, now time.Time) ([]uuid.UUID, error) {
	if s == nil || s.db == nil || strings.TrimSpace(sessionID) == "" {
		return nil, nil
	}
	query := `SELECT memory_id FROM session_memory_cache WHERE session_id = ? AND expires_at > ? ORDER BY memory_id`
	args := []interface{}{sessionID, float64(now.Unix())}
	if s.postgres {
		query = `SELECT memory_id FROM session_memory_cache WHERE session_id = $1 AND expires_at > $2 ORDER BY memory_id`
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if s.postgres {
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
		} else {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				return nil, err
			}
			parsed, err := uuid.FromBytes(raw)
			if err != nil {
				return nil, err
			}
			id = parsed
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *SQLSessionCache) Save(ctx context.Context, sessionID string, ids []uuid.UUID, expiresAt time.Time) error {
	if s == nil || s.db == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	deleteQuery := `DELETE FROM session_memory_cache WHERE session_id = ?`
	insertQuery := `INSERT OR REPLACE INTO session_memory_cache(session_id, memory_id, expires_at) VALUES (?, ?, ?)`
	if s.postgres {
		deleteQuery = `DELETE FROM session_memory_cache WHERE session_id = $1`
		insertQuery = `INSERT INTO session_memory_cache(session_id, memory_id, expires_at) VALUES ($1, $2, $3) ON CONFLICT (session_id, memory_id) DO UPDATE SET expires_at = EXCLUDED.expires_at`
	}
	if _, err := tx.ExecContext(ctx, deleteQuery, sessionID); err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, id := range ids {
		memoryID := interface{}(id[:])
		if s.postgres {
			memoryID = id.String()
		}
		if _, err := tx.ExecContext(ctx, insertQuery, sessionID, memoryID, float64(expiresAt.Unix())); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLSessionCache) PurgeExpired(ctx context.Context, now time.Time) error {
	if s == nil || s.db == nil {
		return nil
	}
	query := `DELETE FROM session_memory_cache WHERE expires_at <= ?`
	if s.postgres {
		query = `DELETE FROM session_memory_cache WHERE expires_at <= $1`
	}
	_, err := s.db.ExecContext(ctx, query, float64(now.Unix()))
	return err
}

var _ ports.SessionCacheStore = (*SQLSessionCache)(nil)
