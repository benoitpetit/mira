// SQLite overlap cache adapter
package vector

import (
	"context"
	"database/sql"
	"time"

	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// SQLiteOverlapCache implements OverlapCache using SQLite
type SQLiteOverlapCache struct {
	db *sql.DB
}

// NewSQLiteOverlapCache creates a new SQLite overlap cache
func NewSQLiteOverlapCache(db *sql.DB) *SQLiteOverlapCache {
	return &SQLiteOverlapCache{db: db}
}

// Get implements OverlapCache
func (c *SQLiteOverlapCache) Get(ctx context.Context, idA, idB uuid.UUID) (float64, bool) {
	// Ensure consistent ordering
	id1, id2 := idA[:], idB[:]
	if string(idA[:]) > string(idB[:]) {
		id1, id2 = id2, id1
	}

	var similarity float64
	err := c.db.QueryRowContext(ctx,
		`SELECT similarity FROM overlap_cache WHERE id_a = ? AND id_b = ? AND ttl > ?`,
		id1, id2, time.Now().Unix(),
	).Scan(&similarity)

	if err != nil {
		return 0, false
	}
	return similarity, true
}

// Set implements OverlapCache
func (c *SQLiteOverlapCache) Set(ctx context.Context, idA, idB uuid.UUID, similarity float64) {
	// Ensure consistent ordering
	id1, id2 := idA[:], idB[:]
	if string(idA[:]) > string(idB[:]) {
		id1, id2 = id2, id1
	}

	now := time.Now().Unix()
	ttl := now + 30*24*3600 // 30 days

	_, _ = c.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO overlap_cache (id_a, id_b, similarity, computed_at, ttl) 
			 VALUES (?, ?, ?, ?, ?)`,
		id1, id2, similarity, now, ttl,
	)
}

// Ensure SQLiteOverlapCache implements OverlapCache
var _ ports.OverlapCache = (*SQLiteOverlapCache)(nil)
