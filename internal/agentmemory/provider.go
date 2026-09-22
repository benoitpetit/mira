package agentmemory

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
)

// RecallFunc is MIRA's normal CBA recall boundary. The identity engine uses
// it instead of maintaining a second relevance algorithm.
type RecallFunc func(context.Context, string, int) ([]MemoryReference, error)
type StoreFunc func(context.Context, string, string, *string, *valueobjects.MemoryType) (uuid.UUID, error)

// MiraProvider bridges the built-in identity engine to MIRA's own memory
// pipeline. Raw SQL is only a fallback for diagnostics and old databases.
type MiraProvider struct {
	db      *sql.DB
	recall  RecallFunc
	store   StoreFunc
	dialect SQLDialect
}

func NewMiraProvider(db *sql.DB, recall RecallFunc, store StoreFunc) *MiraProvider {
	return NewMiraProviderWithDialect(db, recall, store, string(SQLiteDialect))
}

// NewMiraProviderWithDialect keeps the provider on the same SQL dialect as
// the repository that owns MIRA's memory tables.
func NewMiraProviderWithDialect(db *sql.DB, recall RecallFunc, store StoreFunc, dialect string) *MiraProvider {
	return &MiraProvider{db: db, recall: recall, store: store, dialect: normalizeDialect(dialect)}
}

func (p *MiraProvider) GetMiraMemories(ctx context.Context, agentID, query string, budget, limit int) ([]MemoryReference, error) {
	if limit <= 0 {
		limit = 5
	}
	if p.recall != nil && strings.TrimSpace(query) != "" {
		memories, err := p.recall(ctx, query, budget)
		if err == nil && len(memories) > limit {
			memories = memories[:limit]
		}
		if err == nil {
			return memories, nil
		}
	}
	if p.db == nil {
		return nil, nil
	}
	pattern := "%" + strings.TrimSpace(query) + "%"
	sqlQuery := `
		SELECT v.id, v.content, COALESCE(f.ftype, ''), v.created_at,
		       COALESCE(v.wing, ''), COALESCE(v.room, '')
		FROM verbatim v LEFT JOIN fingerprints f ON f.verbatim_id = v.id
		WHERE v.content LIKE ? OR COALESCE(f.data, '') LIKE ?
		ORDER BY v.created_at DESC LIMIT ?`
	if p.dialect == PostgreSQLDialect {
		sqlQuery = `
		SELECT v.id, v.content, COALESCE(f.ftype, ''), v.created_at,
		       COALESCE(v.wing, ''), COALESCE(v.room, '')
		FROM verbatim v LEFT JOIN fingerprints f ON f.verbatim_id = v.id
		WHERE v.content LIKE ? OR COALESCE(f.data::text, '') LIKE ?
		ORDER BY v.created_at DESC LIMIT ?`
	}
	rows, err := p.db.QueryContext(ctx, bindPlaceholders(sqlQuery, p.dialect), pattern, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("agent memory recall fallback: %w", err)
	}
	defer rows.Close()
	result := make([]MemoryReference, 0, limit)
	for rows.Next() {
		var memoryID uuid.UUID
		var memory MemoryReference
		var createdAt float64
		if err := rows.Scan(&memoryID, &memory.Content, &memory.MemoryType, &createdAt, &memory.Wing, &memory.Room); err != nil {
			continue
		}
		memory.MemoryID = memoryID
		memory.Timestamp = time.Unix(int64(createdAt), 0)
		memory.Relevance = 0.5
		result = append(result, memory)
	}
	return result, rows.Err()
}

func (p *MiraProvider) LinkIdentityToMemory(ctx context.Context, identityID, memoryID uuid.UUID) error {
	if p.db == nil {
		return nil
	}
	query := `INSERT OR IGNORE INTO agent_memory_links(identity_id, memory_id, linked_at) VALUES (?, ?, ?)`
	var memoryValue interface{} = memoryID[:]
	if p.dialect == PostgreSQLDialect {
		query = `INSERT INTO agent_memory_links(identity_id, memory_id, linked_at) VALUES (?, ?, ?) ON CONFLICT (identity_id, memory_id) DO NOTHING`
		memoryValue = memoryID
	}
	_, err := p.db.ExecContext(ctx, bindPlaceholders(query, p.dialect), identityID.String(), memoryValue, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (p *MiraProvider) NotifyMiraOfIdentityChange(ctx context.Context, agentID, changeType string) error {
	content := fmt.Sprintf("Agent memory continuity change: %s for agent %s", changeType, agentID)
	if p.store != nil {
		room := "identity_changes"
		kind := valueobjects.TypeFact
		_, err := p.store(ctx, content, "agent_memory", &room, &kind)
		return err
	}
	return nil
}
