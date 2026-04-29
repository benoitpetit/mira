// Package soul provides adapters that bridge MIRA to SOUL.
package soul

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/benoitpetit/soul/pkg/ports"
	"github.com/google/uuid"
)

// MiraProvider implements ports.MiraMemoryProvider using MIRA's SQLite connection.
type MiraProvider struct {
	db *sql.DB
}

// NewMiraProvider creates a new provider backed by MIRA's DB.
func NewMiraProvider(db *sql.DB) *MiraProvider {
	return &MiraProvider{db: db}
}

// GetMiraMemories retrieves factual memories relevant to the query.
func (p *MiraProvider) GetMiraMemories(ctx context.Context, agentID, query string, limit int) ([]ports.MiraMemoryReference, error) {
	sqlQuery := `
		SELECT v.id, f.data, f.ftype, v.created_at, v.wing, v.room
		FROM verbatim v
		JOIN fingerprints f ON f.verbatim_id = v.id
		WHERE v.content LIKE ? OR f.data LIKE ?
		ORDER BY v.created_at DESC
		LIMIT ?
	`
	searchPattern := "%" + query + "%"
	rows, err := p.db.QueryContext(ctx, sqlQuery, searchPattern, searchPattern, limit)
	if err != nil {
		return []ports.MiraMemoryReference{}, nil
	}
	defer rows.Close()

	var memories []ports.MiraMemoryReference
	for rows.Next() {
		mem := ports.MiraMemoryReference{}
		var idStr string
		if err := rows.Scan(&idStr, &mem.Content, &mem.MemoryType, &mem.Timestamp, &mem.Wing, &mem.Room); err != nil {
			continue
		}
		mem.MemoryID = uuid.MustParse(idStr)
		mem.Relevance = 0.8
		memories = append(memories, mem)
	}
	return memories, rows.Err()
}

// GetLinkedMemories retrieves MIRA memories linked to a SOUL identity.
func (p *MiraProvider) GetLinkedMemories(ctx context.Context, identityID uuid.UUID) ([]ports.MiraMemoryReference, error) {
	query := `
		SELECT l.memory_id, COALESCE(v.content, ''), COALESCE(f.ftype, ''), l.linked_at, COALESCE(v.wing, ''), v.room
		FROM soul_mira_links l
		LEFT JOIN verbatim v ON v.id = l.memory_id
		LEFT JOIN fingerprints f ON f.verbatim_id = l.memory_id
		WHERE l.identity_id = ?
	`
	rows, err := p.db.QueryContext(ctx, query, identityID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []ports.MiraMemoryReference
	for rows.Next() {
		mem := ports.MiraMemoryReference{}
		var idStr string
		if err := rows.Scan(&idStr, &mem.Content, &mem.MemoryType, &mem.Timestamp, &mem.Wing, &mem.Room); err != nil {
			continue
		}
		mem.MemoryID = uuid.MustParse(idStr)
		memories = append(memories, mem)
	}
	return memories, rows.Err()
}

// LinkIdentityToMemory creates a link between a SOUL identity and a MIRA memory.
func (p *MiraProvider) LinkIdentityToMemory(ctx context.Context, identityID, memoryID uuid.UUID) error {
	_, err := p.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO soul_mira_links (identity_id, memory_id, linked_at) VALUES (?, ?, ?)",
		identityID.String(), memoryID.String(), time.Now(),
	)
	return err
}

// NotifyMiraOfIdentityChange inserts a memory into MIRA documenting the identity change.
func (p *MiraProvider) NotifyMiraOfIdentityChange(ctx context.Context, agentID string, changeType string) error {
	content := fmt.Sprintf("Identity change detected: %s for agent %s at %s",
		changeType, agentID, time.Now().Format(time.RFC3339))
	_, err := p.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO verbatim (id, content, created_at, wing) VALUES (?, ?, ?, ?)",
		uuid.New().String(), content, time.Now(), "soul_identity",
	)
	return err
}

// Compile-time interface check.
var _ ports.MiraMemoryProvider = (*MiraProvider)(nil)
