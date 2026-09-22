package vector

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// SQLOverlapCache stores pairwise embedding overlap on either supported SQL
// backend. It keeps the cache implementation independent from SQLite's BLOB
// representation and PostgreSQL's UUID representation.
type SQLOverlapCache struct {
	db       *sql.DB
	postgres bool
}

func NewSQLOverlapCache(db *sql.DB, dialect string) *SQLOverlapCache {
	return &SQLOverlapCache{db: db, postgres: strings.EqualFold(strings.TrimSpace(dialect), "postgres") || strings.EqualFold(strings.TrimSpace(dialect), "postgresql")}
}

func (c *SQLOverlapCache) ids(idA, idB uuid.UUID) (interface{}, interface{}) {
	if idA.String() > idB.String() {
		idA, idB = idB, idA
	}
	if c.postgres {
		return idA.String(), idB.String()
	}
	return idA[:], idB[:]
}

func (c *SQLOverlapCache) Get(ctx context.Context, idA, idB uuid.UUID) (float64, bool) {
	if c == nil || c.db == nil {
		return 0, false
	}
	a, b := c.ids(idA, idB)
	query := `SELECT similarity FROM overlap_cache WHERE id_a = ? AND id_b = ? AND ttl > ?`
	if c.postgres {
		query = `SELECT similarity FROM overlap_cache WHERE id_a = $1 AND id_b = $2 AND ttl > $3`
	}
	var similarity float64
	if err := c.db.QueryRowContext(ctx, query, a, b, float64(time.Now().Unix())).Scan(&similarity); err != nil {
		return 0, false
	}
	return similarity, true
}

func (c *SQLOverlapCache) Set(ctx context.Context, idA, idB uuid.UUID, similarity float64) {
	if c == nil || c.db == nil {
		return
	}
	a, b := c.ids(idA, idB)
	now := float64(time.Now().Unix())
	ttl := now + 30*24*3600
	query := `INSERT OR REPLACE INTO overlap_cache (id_a, id_b, similarity, computed_at, ttl) VALUES (?, ?, ?, ?, ?)`
	if c.postgres {
		query = `INSERT INTO overlap_cache (id_a, id_b, similarity, computed_at, ttl) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (id_a, id_b) DO UPDATE SET similarity = EXCLUDED.similarity, computed_at = EXCLUDED.computed_at, ttl = EXCLUDED.ttl`
	}
	_, _ = c.db.ExecContext(ctx, query, a, b, similarity, now, ttl)
}

var _ ports.OverlapCache = (*SQLOverlapCache)(nil)
