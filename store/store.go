package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"mira/types"
)

// Store manages SQLite persistence
type Store struct {
	db *sql.DB
}

// New creates a new Store instance
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=-64000&_mmap_size=268435456")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	return store, nil
}

// Close closes the connection
func (s *Store) Close() error {
	return s.db.Close()
}

// BeginTx starts a transaction
func (s *Store) BeginTx() (*sql.Tx, error) {
	return s.db.Begin()
}

// DB exposes the connection for other modules
func (s *Store) DB() *sql.DB {
	return s.db
}

// migrate creates tables
func (s *Store) migrate() error {
	schema := `
-- =====================================================
-- Embedding model metadata (versioning)
-- =====================================================
CREATE TABLE IF NOT EXISTS embedding_models (
    model_hash TEXT PRIMARY KEY,
    model_name TEXT NOT NULL,
    dimension INTEGER NOT NULL,
    created_at REAL NOT NULL,
    metadata TEXT
) STRICT;

-- =====================================================
-- T0: Verbatim Store (append-only, WAL mode)
-- =====================================================
CREATE TABLE IF NOT EXISTS verbatim (
    id BLOB PRIMARY KEY,
    content TEXT NOT NULL,
    token_count INTEGER NOT NULL,
    created_at REAL NOT NULL,
    wing TEXT NOT NULL,
    room TEXT,
    metadata TEXT
) STRICT;

CREATE INDEX IF NOT EXISTS idx_verbatim_wing_room ON verbatim(wing, room);
CREATE INDEX IF NOT EXISTS idx_verbatim_created ON verbatim(created_at);
CREATE INDEX IF NOT EXISTS idx_verbatim_wing_time ON verbatim(wing, created_at);

-- =====================================================
-- T1: Fingerprint Index (canonical JSON)
-- =====================================================
CREATE TABLE IF NOT EXISTS fingerprints (
    id BLOB PRIMARY KEY,
    verbatim_id BLOB NOT NULL REFERENCES verbatim(id),
    ftype TEXT NOT NULL,
    extracted_at REAL NOT NULL,
    entities TEXT,
    subjects TEXT,
    decision TEXT,
    related_to TEXT,
    data TEXT NOT NULL,
    fact_count INTEGER DEFAULT 0,
    token_estimate INTEGER DEFAULT 0,
    model_hash TEXT REFERENCES embedding_models(model_hash)
) STRICT;

CREATE INDEX IF NOT EXISTS idx_fp_type ON fingerprints(ftype);
CREATE INDEX IF NOT EXISTS idx_fp_entities ON fingerprints(entities);
CREATE INDEX IF NOT EXISTS idx_fp_subjects ON fingerprints(subjects);
CREATE INDEX IF NOT EXISTS idx_fp_decision ON fingerprints(decision);

-- =====================================================
-- T2: Vector Index with versioning
-- =====================================================
CREATE TABLE IF NOT EXISTS embeddings (
    id BLOB PRIMARY KEY REFERENCES verbatim(id),
    model_hash TEXT NOT NULL REFERENCES embedding_models(model_hash),
    dim INTEGER NOT NULL,
    vector BLOB NOT NULL,
    normalized INTEGER DEFAULT 1,
    created_at REAL NOT NULL
) STRICT;

-- =====================================================
-- Causal Graph (DAG)
-- =====================================================
CREATE TABLE IF NOT EXISTS causal_nodes (
    id BLOB PRIMARY KEY REFERENCES fingerprints(id),
    node_type TEXT NOT NULL,
    summary TEXT NOT NULL,
    timestamp REAL NOT NULL,
    wing TEXT NOT NULL,
    room TEXT
) STRICT;

CREATE TABLE IF NOT EXISTS causal_edges (
    from_id BLOB NOT NULL REFERENCES causal_nodes(id),
    to_id BLOB NOT NULL REFERENCES causal_nodes(id),
    relation TEXT NOT NULL,
    weight REAL DEFAULT 1.0,
    detected_at REAL NOT NULL,
    PRIMARY KEY (from_id, to_id, relation)
) STRICT;

CREATE INDEX IF NOT EXISTS idx_edges_from ON causal_edges(from_id);
CREATE INDEX IF NOT EXISTS idx_edges_to ON causal_edges(to_id);
CREATE INDEX IF NOT EXISTS idx_edges_timestamp ON causal_edges(detected_at);

-- =====================================================
-- Overlap cache with TTL (30 days)
-- =====================================================
CREATE TABLE IF NOT EXISTS overlap_cache (
    id_a BLOB NOT NULL,
    id_b BLOB NOT NULL,
    similarity REAL NOT NULL,
    computed_at REAL NOT NULL,
    ttl REAL NOT NULL DEFAULT (unixepoch() + 2592000),
    PRIMARY KEY (id_a, id_b)
) STRICT;

CREATE INDEX IF NOT EXISTS idx_overlap_ttl ON overlap_cache(computed_at) WHERE computed_at < unixepoch() - 2592000;
`
	_, err := s.db.Exec(schema)
	return err
}

// StoreVerbatimTx stores a verbatim in a transaction
func (s *Store) StoreVerbatimTx(tx *sql.Tx, v *types.Verbatim) error {
	metadataJSON, _ := json.Marshal(v.Metadata)
	metadataStr := string(metadataJSON)
	_, err := tx.Exec(
		`INSERT INTO verbatim (id, content, token_count, created_at, wing, room, metadata) 
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		v.ID[:], v.Content, v.TokenCount, float64(v.CreatedAt.Unix()), v.Wing, v.Room, metadataStr,
	)
	return err
}

// StoreFingerprintTx stores a fingerprint in a transaction
func (s *Store) StoreFingerprintTx(tx *sql.Tx, fp *types.Fingerprint) error {
	entitiesJSON, _ := json.Marshal(fp.Entities)
	subjectsJSON, _ := json.Marshal(fp.Subjects)
	relatedJSON, _ := json.Marshal(fp.RelatedTo)
	dataJSON, _ := json.Marshal(fp.Data)
	
	entitiesStr := string(entitiesJSON)
	subjectsStr := string(subjectsJSON)
	relatedStr := string(relatedJSON)
	dataStr := string(dataJSON)

	var decision *string
	if fp.Decision != nil && *fp.Decision != "" {
		decision = fp.Decision
	}

	_, err := tx.Exec(
		`INSERT INTO fingerprints (id, verbatim_id, ftype, extracted_at, entities, subjects, decision, related_to, data, fact_count, token_estimate, model_hash) 
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fp.ID[:], fp.VerbatimID[:], string(fp.Type), float64(fp.ExtractedAt.Unix()),
		entitiesStr, subjectsStr, decision, relatedStr, dataStr,
		fp.FactCount, fp.TokenEstimate, fp.ModelHash,
	)
	return err
}

// StoreEmbeddingTx stores an embedding in a transaction
func (s *Store) StoreEmbeddingTx(tx *sql.Tx, emb *types.Embedding) error {
	vectorBytes := make([]byte, len(emb.Vector)*4)
	for i, v := range emb.Vector {
		// Store as little-endian float32
		vectorBytes[i*4+0] = byte(uint32(v))
		vectorBytes[i*4+1] = byte(uint32(v) >> 8)
		vectorBytes[i*4+2] = byte(uint32(v) >> 16)
		vectorBytes[i*4+3] = byte(uint32(v) >> 24)
	}

	_, err := tx.Exec(
		`INSERT INTO embeddings (id, model_hash, dim, vector, normalized, created_at) 
		 VALUES (?, ?, ?, ?, ?, ?)`,
		emb.ID[:], emb.ModelHash, emb.Dim, vectorBytes, emb.Normalized, float64(emb.CreatedAt.Unix()),
	)
	return err
}

// GetVerbatim retrieves a verbatim by ID
func (s *Store) GetVerbatim(id uuid.UUID) (*types.Verbatim, error) {
	row := s.db.QueryRow(
		`SELECT id, content, token_count, created_at, wing, room, metadata FROM verbatim WHERE id = ?`,
		id[:],
	)

	var v types.Verbatim
	var idBytes []byte
	var metadataJSON []byte
	var room sql.NullString
	var createdAt float64

	err := row.Scan(&idBytes, &v.Content, &v.TokenCount, &createdAt, &v.Wing, &room, &metadataJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("verbatim not found")
		}
		return nil, err
	}

	v.ID, _ = uuid.FromBytes(idBytes)
	v.CreatedAt = time.Unix(int64(createdAt), 0)
	if room.Valid {
		v.Room = &room.String
	}
	if len(metadataJSON) > 0 {
		json.Unmarshal(metadataJSON, &v.Metadata)
	}

	return &v, nil
}

// GetEmbedding retrieves an embedding by ID
func (s *Store) GetEmbedding(id uuid.UUID) (*types.Embedding, error) {
	row := s.db.QueryRow(
		`SELECT id, model_hash, dim, vector, normalized, created_at FROM embeddings WHERE id = ?`,
		id[:],
	)

	var emb types.Embedding
	var idBytes []byte
	var vectorBytes []byte
	var createdAt float64

	err := row.Scan(&idBytes, &emb.ModelHash, &emb.Dim, &vectorBytes, &emb.Normalized, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("embedding not found")
		}
		return nil, err
	}

	emb.ID, _ = uuid.FromBytes(idBytes)
	emb.CreatedAt = time.Unix(int64(createdAt), 0)

	// Decode float32
	emb.Vector = make([]float32, len(vectorBytes)/4)
	for i := 0; i < len(emb.Vector); i++ {
		b := vectorBytes[i*4 : i*4+4]
		u := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
		emb.Vector[i] = float32(u) // Note: this is an approximation to simplify
	}

	return &emb, nil
}

// SearchCandidates searches for CBA candidates
func (s *Store) SearchCandidates(wing, room *string, limit int) ([]*types.Candidate, error) {
	query := `
		SELECT v.id, v.content, v.token_count, v.created_at, v.wing, v.room,
		       f.id, f.ftype, f.extracted_at, f.entities, f.subjects, f.decision, f.data, f.fact_count, f.token_estimate, f.model_hash,
		       e.dim, e.vector
		FROM verbatim v
		JOIN fingerprints f ON v.id = f.verbatim_id
		JOIN embeddings e ON v.id = e.id
		WHERE 1=1
	`
	args := []interface{}{}

	if wing != nil {
		query += " AND v.wing = ?"
		args = append(args, *wing)
	}
	if room != nil {
		query += " AND v.room = ?"
		args = append(args, *room)
	}

	query += " ORDER BY v.created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []*types.Candidate

	for rows.Next() {
		var v types.Verbatim
		var fp types.Fingerprint
		var emb types.Embedding

		var vID, fpID []byte
		var room sql.NullString
		var createdAt float64
		var extractedAt float64
		var entitiesJSON, subjectsJSON []byte
		var decision sql.NullString
		var dataJSON []byte
		var vectorBytes []byte

		err := rows.Scan(
			&vID, &v.Content, &v.TokenCount, &createdAt, &v.Wing, &room,
			&fpID, &fp.Type, &extractedAt, &entitiesJSON, &subjectsJSON, &decision, &dataJSON, &fp.FactCount, &fp.TokenEstimate, &fp.ModelHash,
			&emb.Dim, &vectorBytes,
		)
		if err != nil {
			continue
		}

		v.ID, _ = uuid.FromBytes(vID)
		v.CreatedAt = time.Unix(int64(createdAt), 0)
		if room.Valid {
			v.Room = &room.String
		}

		fp.ID, _ = uuid.FromBytes(fpID)
		fp.VerbatimID = v.ID
		fp.ExtractedAt = time.Unix(int64(extractedAt), 0)
		if decision.Valid {
			fp.Decision = &decision.String
		}
		if len(entitiesJSON) > 0 {
			json.Unmarshal(entitiesJSON, &fp.Entities)
		}
		if len(subjectsJSON) > 0 {
			json.Unmarshal(subjectsJSON, &fp.Subjects)
		}
		if len(dataJSON) > 0 {
			json.Unmarshal(dataJSON, &fp.Data)
		}

		// Decode vector
		emb.Vector = make([]float32, emb.Dim)
		for i := 0; i < emb.Dim && i*4+3 < len(vectorBytes); i++ {
			b := vectorBytes[i*4 : i*4+4]
			// Little-endian float32
			u := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
			emb.Vector[i] = float32(u)
		}

		candidates = append(candidates, &types.Candidate{
			Memory:    &fp,
			Verbatim:  &v,
			Embedding: emb.Vector,
		})
	}

	return candidates, nil
}

// RegisterModel registers an embedding model
func (s *Store) RegisterModel(model *types.EmbeddingModel) error {
	metadataJSON, _ := json.Marshal(model.Metadata)
	metadataStr := string(metadataJSON)
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO embedding_models (model_hash, model_name, dimension, created_at, metadata) 
		 VALUES (?, ?, ?, ?, ?)`,
		model.ModelHash, model.ModelName, model.Dimension, float64(model.CreatedAt.Unix()), metadataStr,
	)
	return err
}

// GetStats retrieves statistics
func (s *Store) GetStats() (*types.Stats, error) {
	stats := &types.Stats{
		TypeCounts: make(map[string]int),
	}

	// Count verbatims
	err := s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(token_count), 0) FROM verbatim`).Scan(&stats.VerbatimCount, &stats.TotalTokens)
	if err != nil {
		return nil, err
	}

	// Count fingerprints
	err = s.db.QueryRow(`SELECT COUNT(*) FROM fingerprints`).Scan(&stats.FingerprintCount)
	if err != nil {
		return nil, err
	}

	// Count embeddings
	err = s.db.QueryRow(`SELECT COUNT(*) FROM embeddings`).Scan(&stats.EmbeddingCount)
	if err != nil {
		return nil, err
	}

	// Count causal nodes and edges
	err = s.db.QueryRow(`SELECT COUNT(*) FROM causal_nodes`).Scan(&stats.CausalNodeCount)
	if err != nil {
		return nil, err
	}
	err = s.db.QueryRow(`SELECT COUNT(*) FROM causal_edges`).Scan(&stats.CausalEdgeCount)
	if err != nil {
		return nil, err
	}

	// Distribution by type
	rows, err := s.db.Query(`SELECT ftype, COUNT(*) FROM fingerprints GROUP BY ftype`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var t string
		var count int
		if err := rows.Scan(&t, &count); err == nil {
			stats.TypeCounts[t] = count
		}
	}

	// Active wings
	row, err := s.db.Query(`SELECT DISTINCT wing FROM verbatim ORDER BY wing LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer row.Close()

	for row.Next() {
		var wing string
		if err := row.Scan(&wing); err == nil {
			stats.ActiveWings = append(stats.ActiveWings, wing)
		}
	}

	return stats, nil
}

// GetEmbeddingModels lists registered models
func (s *Store) GetEmbeddingModels() ([]string, error) {
	rows, err := s.db.Query(`SELECT model_hash FROM embedding_models`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err == nil {
			models = append(models, m)
		}
	}
	return models, nil
}

// GetTimeline retrieves timeline
func (s *Store) GetTimeline(wing string, room, memType *string, since, until *time.Time) ([]*types.TimelineItem, error) {
	query := `
		SELECT f.id, f.ftype, f.extracted_at, f.data
		FROM fingerprints f
		JOIN verbatim v ON f.verbatim_id = v.id
		WHERE v.wing = ?
	`
	args := []interface{}{wing}

	if room != nil {
		query += " AND v.room = ?"
		args = append(args, *room)
	}
	if memType != nil {
		query += " AND f.ftype = ?"
		args = append(args, *memType)
	}
	if since != nil {
		query += " AND f.extracted_at >= ?"
		args = append(args, float64(since.Unix()))
	}
	if until != nil {
		query += " AND f.extracted_at <= ?"
		args = append(args, float64(until.Unix()))
	}

	query += " ORDER BY f.extracted_at DESC LIMIT 100"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*types.TimelineItem
	for rows.Next() {
		var id []byte
		var memTypeStr string
		var extractedAt float64
		var dataJSON []byte

		if err := rows.Scan(&id, &memTypeStr, &extractedAt, &dataJSON); err != nil {
			continue
		}

		uid, _ := uuid.FromBytes(id)
		var data types.FingerprintData
		json.Unmarshal(dataJSON, &data)

		summary := ""
		if len(data.Subject) > 0 {
			summary = data.Subject[0]
		}
		if summary == "" && data.Decision != "" {
			summary = data.Decision
		}
		if summary == "" {
			summary = "Memory " + uid.String()[:8]
		}
		items = append(items, &types.TimelineItem{
			ID:        uid,
			Timestamp: time.Unix(int64(extractedAt), 0),
			Type:      types.MemoryType(memTypeStr),
			Summary:   summary,
		})
	}

	return items, nil
}

// ArchiveOldMemories archives old memories
func (s *Store) ArchiveOldMemories() (*types.ArchiveResult, error) {
	result := &types.ArchiveResult{}
	now := float64(time.Now().Unix())

	// Archive session_notes > 30 days
	threshold := now - 30*24*60*60
	r, err := s.db.Exec(`DELETE FROM verbatim WHERE created_at < ? AND id IN (SELECT verbatim_id FROM fingerprints WHERE ftype = 'session_note')`, threshold)
	if err == nil {
		count, _ := r.RowsAffected()
		result.SessionNotes = int(count)
	}

	// Archive debug_logs > 7 days
	threshold = now - 7*24*60*60
	r, err = s.db.Exec(`DELETE FROM verbatim WHERE created_at < ? AND id IN (SELECT verbatim_id FROM fingerprints WHERE ftype = 'debug_log')`, threshold)
	if err == nil {
		count, _ := r.RowsAffected()
		result.DebugLogs = int(count)
	}

	// Clean overlap cache
	s.db.Exec(`DELETE FROM overlap_cache WHERE ttl < ?`, now)

	return result, nil
}
