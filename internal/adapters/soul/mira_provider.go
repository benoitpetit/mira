// Package soul provides adapters that bridge MIRA to SOUL.
package soul

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/interactors"
	"github.com/benoitpetit/soul/pkg/ports"
	"github.com/google/uuid"
)

// MiraProvider implements ports.MiraMemoryProvider using MIRA's SQLite connection.
type MiraProvider struct {
	db          *sql.DB
	storeMemory *interactors.StoreMemory
}

// NewMiraProvider creates a new provider backed by MIRA's DB.
// The storeMemory use case is optional - if nil, StoreMemory will fallback to direct INSERT.
func NewMiraProvider(db *sql.DB, storeMemory *interactors.StoreMemory) *MiraProvider {
	return &MiraProvider{db: db, storeMemory: storeMemory}
}

// GetMiraMemories retrieves factual memories relevant to the query.
// Prefers FTS5 lexical search when available for better relevance ranking;
// falls back to LIKE when FTS5 is not enabled.
func (p *MiraProvider) GetMiraMemories(ctx context.Context, agentID, query string, limit int) ([]ports.MiraMemoryReference, error) {
	if limit <= 0 {
		limit = 5
	}

	ftsQuery := `
		SELECT v.id, f.data, f.ftype, v.created_at, COALESCE(v.wing, ''), v.room
		FROM verbatim v
		JOIN fingerprints f ON f.verbatim_id = v.id
		WHERE v.id IN (SELECT rowid FROM verbatim_fts WHERE verbatim_fts MATCH ?)
		ORDER BY v.created_at DESC
		LIMIT ?`

	rows, err := p.db.QueryContext(ctx, ftsQuery, query, limit)
	if err == nil {
		memories := p.scanMiraMemories(rows)
		rows.Close()
		if len(memories) > 0 {
			return memories, nil
		}
	}

	sqlQuery := `
		SELECT v.id, f.data, f.ftype, v.created_at, COALESCE(v.wing, ''), v.room
		FROM verbatim v
		JOIN fingerprints f ON f.verbatim_id = v.id
		WHERE v.content LIKE ? OR f.data LIKE ?
		ORDER BY v.created_at DESC
		LIMIT ?
	`
	searchPattern := "%" + query + "%"
	rows, err = p.db.QueryContext(ctx, sqlQuery, searchPattern, searchPattern, limit)
	if err != nil {
		return nil, fmt.Errorf("GetMiraMemories query failed: %w", err)
	}
	defer rows.Close()

	return p.scanMiraMemories(rows), nil
}

func (p *MiraProvider) scanMiraMemories(rows *sql.Rows) []ports.MiraMemoryReference {
	memories := []ports.MiraMemoryReference{}
	for rows.Next() {
		mem := ports.MiraMemoryReference{}
		var idBytes []byte
		var createdAtSecs float64
		if err := rows.Scan(&idBytes, &mem.Content, &mem.MemoryType, &createdAtSecs, &mem.Wing, &mem.Room); err != nil {
			continue
		}
		id, err := uuid.FromBytes(idBytes)
		if err != nil {
			continue
		}
		mem.MemoryID = id
		mem.Timestamp = time.Unix(int64(createdAtSecs), 0)
		mem.Relevance = 0.8
		memories = append(memories, mem)
	}
	return memories
}

// GetLinkedMemories retrieves MIRA memories linked to a SOUL identity.
// soul_mira_links stores identity_id/memory_id as TEXT (UUID strings).
// verbatim.id and fingerprints.verbatim_id are BLOB (16 raw bytes).
// hex() is used to compare the BLOB columns against the dash-free uppercase UUID string.
func (p *MiraProvider) GetLinkedMemories(ctx context.Context, identityID uuid.UUID) ([]ports.MiraMemoryReference, error) {
	query := `
		SELECT l.memory_id, COALESCE(v.content, ''), COALESCE(f.ftype, ''), l.linked_at, COALESCE(v.wing, ''), v.room
		FROM soul_mira_links l
		LEFT JOIN verbatim v     ON hex(v.id)           = replace(upper(l.memory_id), '-', '')
		LEFT JOIN fingerprints f ON hex(f.verbatim_id)  = replace(upper(l.memory_id), '-', '')
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
		id, err := uuid.Parse(idStr)
		if err != nil {
			continue
		}
		mem.MemoryID = id
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

// NotifyMiraOfIdentityChange stores an identity change event as a full memory.
// Uses StoreMemory use case if available for complete T0/T1/T2 extraction.
// Falls back to direct INSERT using MIRA-compatible BLOB UUID storage when
// storeMemory is nil (e.g. standalone SOUL without embedded use cases).
func (p *MiraProvider) NotifyMiraOfIdentityChange(ctx context.Context, agentID string, changeType string) error {
	content := fmt.Sprintf("Identity change detected: %s for agent %s at %s",
		changeType, agentID, time.Now().Format(time.RFC3339))

	// Try to use StoreMemory use case for complete extraction
	if p.storeMemory != nil {
		memType := valueobjects.TypeFact
		room := "identity_changes"
		_, err := p.storeMemory.Execute(ctx, interactors.StoreMemoryInput{
			Content: content,
			Wing:    "soul_identity",
			Room:    &room,
			Type:    &memType,
		})
		return err
	}

	// Fallback to direct INSERT using BLOB UUID to match the verbatim.id column type.
	// The verbatim table schema (MIRA migration 001) defines id as BLOB PRIMARY KEY,
	// so we must store the raw 16-byte UUID, NOT its string representation.
	id := uuid.New()
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO verbatim (id, content, created_at, wing, token_count, metadata, metrics)
		 VALUES (?, ?, ?, ?, ?, '{}', '{}')`,
		id[:], content, time.Now().Unix(), "soul_identity", calculateTokenCount(content),
	)
	return err
}

// calculateTokenCount estimates token count for a given content.
func calculateTokenCount(content string) int {
	return len(content) / 4
}

// StoreMemory stores a new memory with full extraction (T0/T1/T2).
// If storeMemory use case is not available, returns an error.
func (p *MiraProvider) StoreMemory(ctx context.Context, content, wing string, room *string, memType *string) (uuid.UUID, error) {
	if p.storeMemory == nil {
		return uuid.Nil, fmt.Errorf("StoreMemory use case not available")
	}

	var mt *valueobjects.MemoryType
	if memType != nil {
		t := valueobjects.MemoryType(*memType)
		mt = &t
	}

	out, err := p.storeMemory.Execute(ctx, interactors.StoreMemoryInput{
		Content: content,
		Wing:    wing,
		Room:    room,
		Type:    mt,
	})
	if err != nil {
		return uuid.Nil, err
	}

	id, err := uuid.Parse(out.FingerprintID)
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

// Compile-time interface check.
var _ ports.MiraMemoryProvider = (*MiraProvider)(nil)
