// SQLite repository implementation
package storage

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// SQLiteRepository implements all repository interfaces using SQLite
type SQLiteRepository struct {
	db   *sql.DB
	opts SQLiteOptions
}

// SQLiteOptions holds configuration for SQLite repository
type SQLiteOptions struct {
	SessionNoteArchiveDays int
	DebugLogArchiveDays    int
}

// DefaultSQLiteOptions returns default options
func DefaultSQLiteOptions() SQLiteOptions {
	return SQLiteOptions{
		SessionNoteArchiveDays: 30,
		DebugLogArchiveDays:    7,
	}
}

// NewSQLiteRepository creates a new SQLite repository
func NewSQLiteRepository(dbPath string, opts SQLiteOptions) (*SQLiteRepository, error) {
	if opts.SessionNoteArchiveDays <= 0 {
		opts.SessionNoteArchiveDays = 30
	}
	if opts.DebugLogArchiveDays <= 0 {
		opts.DebugLogArchiveDays = 7
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=-64000&_mmap_size=268435456")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(30 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	repo := &SQLiteRepository{db: db, opts: opts}
	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	return repo, nil
}

// Close closes the database connection
func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

// Begin starts a transaction
func (r *SQLiteRepository) Begin() (*sql.Tx, error) {
	return r.db.Begin()
}

// DB returns the underlying database connection
func (r *SQLiteRepository) DB() *sql.DB {
	return r.db
}

// StoreVerbatim implements VerbatimRepository
func (r *SQLiteRepository) StoreVerbatim(ctx context.Context, verbatim *entities.Verbatim) error {
	tx, err := r.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := r.StoreVerbatimTx(ctx, tx, verbatim); err != nil {
		return err
	}

	return tx.Commit()
}

// StoreVerbatimTx implements VerbatimRepository
func (r *SQLiteRepository) StoreVerbatimTx(ctx context.Context, tx *sql.Tx, v *entities.Verbatim) error {
	metadataJSON, err := json.Marshal(v.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO verbatim (id, content, token_count, created_at, wing, room, metadata) 
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		v.ID[:], v.Content, v.TokenCount, float64(v.CreatedAt.Unix()), v.Wing, v.Room, string(metadataJSON),
	)
	return err
}

// GetVerbatimByID implements VerbatimRepository
func (r *SQLiteRepository) GetVerbatimByID(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, content, token_count, created_at, wing, room, metadata FROM verbatim WHERE id = ?`,
		id[:],
	)

	var v entities.Verbatim
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

	v.ID, err = uuid.FromBytes(idBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid verbatim UUID: %w", err)
	}
	v.CreatedAt = time.Unix(int64(createdAt), 0)
	if room.Valid {
		v.Room = &room.String
	}
	if len(metadataJSON) > 0 {
		_ = json.Unmarshal(metadataJSON, &v.Metadata)
	}

	return &v, nil
}

// StoreFingerprint implements FingerprintRepository
func (r *SQLiteRepository) StoreFingerprint(ctx context.Context, fp *entities.Fingerprint) error {
	tx, err := r.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := r.StoreFingerprintTx(ctx, tx, fp); err != nil {
		return err
	}

	return tx.Commit()
}

// StoreFingerprintTx implements FingerprintRepository
func (r *SQLiteRepository) StoreFingerprintTx(ctx context.Context, tx *sql.Tx, fp *entities.Fingerprint) error {
	entitiesJSON, _ := json.Marshal(fp.Entities)
	subjectsJSON, _ := json.Marshal(fp.Subjects)
	dataJSON, _ := json.Marshal(fp.Data)

	var decision *string
	if fp.Decision != nil && *fp.Decision != "" {
		decision = fp.Decision
	}

	_, err := tx.ExecContext(ctx,
		`INSERT INTO fingerprints (id, verbatim_id, ftype, extracted_at, entities, subjects, decision, data, fact_count, token_estimate, model_hash) 
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fp.ID[:], fp.VerbatimID[:], string(fp.Type), float64(fp.ExtractedAt.Unix()),
		string(entitiesJSON), string(subjectsJSON), decision, string(dataJSON),
		fp.FactCount, fp.TokenEstimate, fp.ModelHash,
	)
	return err
}

// GetFingerprintByID implements FingerprintRepository
func (r *SQLiteRepository) GetFingerprintByID(ctx context.Context, id uuid.UUID) (*entities.Fingerprint, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT f.id, f.verbatim_id, f.ftype, f.extracted_at, f.entities, f.subjects, f.decision, f.data, f.fact_count, f.token_estimate, f.model_hash
		 FROM fingerprints f
		 WHERE f.id = ?`,
		id[:],
	)

	var fp entities.Fingerprint
	var idBytes, verbatimIDBytes []byte
	var ftype string
	var extractedAt float64
	var entitiesJSON, subjectsJSON, dataJSON []byte
	var decision sql.NullString

	err := row.Scan(&idBytes, &verbatimIDBytes, &ftype, &extractedAt, &entitiesJSON, &subjectsJSON, &decision, &dataJSON, &fp.FactCount, &fp.TokenEstimate, &fp.ModelHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("fingerprint not found")
		}
		return nil, err
	}

	fp.ID, err = uuid.FromBytes(idBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid fingerprint UUID: %w", err)
	}

	fp.VerbatimID, err = uuid.FromBytes(verbatimIDBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid verbatim UUID: %w", err)
	}

	fp.Type = valueobjects.MemoryType(ftype)
	fp.ExtractedAt = time.Unix(int64(extractedAt), 0)

	if decision.Valid {
		fp.Decision = &decision.String
	}

	_ = json.Unmarshal(entitiesJSON, &fp.Entities)
	_ = json.Unmarshal(subjectsJSON, &fp.Subjects)
	_ = json.Unmarshal(dataJSON, &fp.Data)

	return &fp, nil
}

// GetFingerprintByVerbatimID implements FingerprintRepository
func (r *SQLiteRepository) GetFingerprintByVerbatimID(ctx context.Context, verbatimID uuid.UUID) (*entities.Fingerprint, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT f.id, f.verbatim_id, f.ftype, f.extracted_at, f.entities, f.subjects, f.decision, f.data, f.fact_count, f.token_estimate, f.model_hash
		 FROM fingerprints f
		 WHERE f.verbatim_id = ?`,
		verbatimID[:],
	)

	var fp entities.Fingerprint
	var idBytes, verbatimIDBytes []byte
	var ftype string
	var extractedAt float64
	var entitiesJSON, subjectsJSON, dataJSON []byte
	var decision sql.NullString

	err := row.Scan(&idBytes, &verbatimIDBytes, &ftype, &extractedAt, &entitiesJSON, &subjectsJSON, &decision, &dataJSON, &fp.FactCount, &fp.TokenEstimate, &fp.ModelHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("fingerprint not found")
		}
		return nil, err
	}

	fp.ID, err = uuid.FromBytes(idBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid fingerprint UUID: %w", err)
	}

	fp.VerbatimID, err = uuid.FromBytes(verbatimIDBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid verbatim UUID: %w", err)
	}

	fp.Type = valueobjects.MemoryType(ftype)
	fp.ExtractedAt = time.Unix(int64(extractedAt), 0)

	if decision.Valid {
		fp.Decision = &decision.String
	}

	_ = json.Unmarshal(entitiesJSON, &fp.Entities)
	_ = json.Unmarshal(subjectsJSON, &fp.Subjects)
	_ = json.Unmarshal(dataJSON, &fp.Data)

	return &fp, nil
}

// GetRecentFingerprintsByWing implements FingerprintRepository
func (r *SQLiteRepository) GetRecentFingerprintsByWing(ctx context.Context, wing string, excludeID uuid.UUID, limit int) ([]*entities.Fingerprint, error) {
	tx, err := r.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	return r.GetRecentFingerprintsByWingTx(ctx, tx, wing, excludeID, limit)
}

// GetRecentFingerprintsByWingTx implements FingerprintRepository
func (r *SQLiteRepository) GetRecentFingerprintsByWingTx(ctx context.Context, tx *sql.Tx, wing string, excludeID uuid.UUID, limit int) ([]*entities.Fingerprint, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT f.id, f.verbatim_id, f.ftype, f.extracted_at, f.entities, f.subjects, f.decision, f.data, f.fact_count, f.token_estimate, f.model_hash
		 FROM fingerprints f
		 JOIN verbatim v ON f.verbatim_id = v.id
		 WHERE v.wing = ? AND f.id != ?
		 ORDER BY f.extracted_at DESC
		 LIMIT ?`,
		wing, excludeID[:], limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fingerprints []*entities.Fingerprint
	for rows.Next() {
		fp := &entities.Fingerprint{}
		var idBytes, verbatimIDBytes []byte
		var ftype string
		var extractedAt float64
		var entitiesJSON, subjectsJSON, dataJSON []byte
		var decision sql.NullString

		err := rows.Scan(&idBytes, &verbatimIDBytes, &ftype, &extractedAt, &entitiesJSON, &subjectsJSON, &decision, &dataJSON, &fp.FactCount, &fp.TokenEstimate, &fp.ModelHash)
		if err != nil {
			continue
		}

		fp.ID, _ = uuid.FromBytes(idBytes)
		fp.VerbatimID, _ = uuid.FromBytes(verbatimIDBytes)
		fp.Type = valueobjects.MemoryType(ftype)
		fp.ExtractedAt = time.Unix(int64(extractedAt), 0)
		if decision.Valid {
			fp.Decision = &decision.String
		}
		_ = json.Unmarshal(entitiesJSON, &fp.Entities)
		_ = json.Unmarshal(subjectsJSON, &fp.Subjects)
		_ = json.Unmarshal(dataJSON, &fp.Data)

		fingerprints = append(fingerprints, fp)
	}

	return fingerprints, nil
}

// StoreEmbedding implements EmbeddingRepository
func (r *SQLiteRepository) StoreEmbedding(ctx context.Context, emb *entities.Embedding) error {
	tx, err := r.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := r.StoreEmbeddingTx(ctx, tx, emb); err != nil {
		return err
	}

	return tx.Commit()
}

// StoreEmbeddingTx implements EmbeddingRepository
func (r *SQLiteRepository) StoreEmbeddingTx(ctx context.Context, tx *sql.Tx, emb *entities.Embedding) error {
	vectorBytes := make([]byte, len(emb.Vector)*4)
	for i, v := range emb.Vector {
		bits := math.Float32bits(v)
		binary.LittleEndian.PutUint32(vectorBytes[i*4:], bits)
	}

	_, err := tx.ExecContext(ctx,
		`INSERT INTO embeddings (id, model_hash, dim, vector, normalized, created_at) 
		 VALUES (?, ?, ?, ?, ?, ?)`,
		emb.ID[:], emb.ModelHash, emb.Dim, vectorBytes, emb.Normalized, float64(emb.CreatedAt.Unix()),
	)
	return err
}

// GetEmbeddingByID implements EmbeddingRepository
func (r *SQLiteRepository) GetEmbeddingByID(ctx context.Context, id uuid.UUID) (*entities.Embedding, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, model_hash, dim, vector, normalized, created_at FROM embeddings WHERE id = ?`,
		id[:],
	)

	var emb entities.Embedding
	var idBytes, vectorBytes []byte
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

	emb.Vector = make([]float32, emb.Dim)
	for i := 0; i < emb.Dim && i*4+3 < len(vectorBytes); i++ {
		u := binary.LittleEndian.Uint32(vectorBytes[i*4 : i*4+4])
		emb.Vector[i] = math.Float32frombits(u)
	}

	return &emb, nil
}

// AddNode implements CausalGraphRepository
func (r *SQLiteRepository) AddNode(ctx context.Context, node *entities.CausalNode) error {
	tx, err := r.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := r.AddNodeTx(ctx, tx, node); err != nil {
		return err
	}

	return tx.Commit()
}

// AddNodeTx implements CausalGraphRepository
func (r *SQLiteRepository) AddNodeTx(ctx context.Context, tx *sql.Tx, node *entities.CausalNode) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO causal_nodes (id, node_type, summary, timestamp, wing, room) 
		 VALUES (?, ?, ?, ?, ?, ?)`,
		node.ID[:], node.Type, node.Summary, float64(node.Timestamp.Unix()), node.Wing, node.Room,
	)
	return err
}

// AddEdge implements CausalGraphRepository
func (r *SQLiteRepository) AddEdge(ctx context.Context, edge *entities.CausalEdge) error {
	tx, err := r.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := r.AddEdgeTx(ctx, tx, edge); err != nil {
		return err
	}

	return tx.Commit()
}

// AddEdgeTx implements CausalGraphRepository
func (r *SQLiteRepository) AddEdgeTx(ctx context.Context, tx *sql.Tx, edge *entities.CausalEdge) error {
	_, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO causal_edges (from_id, to_id, relation, weight, detected_at) 
		 VALUES (?, ?, ?, ?, ?)`,
		edge.FromID[:], edge.ToID[:], string(edge.Relation), edge.Weight, float64(edge.DetectedAt.Unix()),
	)
	return err
}

// HasEdge implements CausalGraphRepository
func (r *SQLiteRepository) HasEdge(ctx context.Context, fromID, toID uuid.UUID) bool {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM causal_edges WHERE (from_id = ? AND to_id = ?) OR (from_id = ? AND to_id = ?)`,
		fromID[:], toID[:], toID[:], fromID[:],
	).Scan(&count)
	return err == nil && count > 0
}

// GetChain implements CausalGraphRepository
// Performs a BFS traversal up the causal chain (parents) up to maxDepth levels
func (r *SQLiteRepository) GetChain(ctx context.Context, id uuid.UUID, maxDepth int) ([]*entities.CausalNode, error) {
	if maxDepth <= 0 {
		maxDepth = 5 // Default depth
	}

	var result []*entities.CausalNode
	visited := make(map[uuid.UUID]bool)
	queue := []struct {
		node  uuid.UUID
		depth int
	}{{id, 0}}

	for len(queue) > 0 {
		// Dequeue
		current := queue[0]
		queue = queue[1:]

		if current.depth >= maxDepth {
			continue
		}

		// Get parents of current node
		parents, err := r.GetParents(ctx, current.node)
		if err != nil {
			return nil, err
		}

		for _, parent := range parents {
			if visited[parent.ID] {
				continue
			}
			visited[parent.ID] = true
			result = append(result, parent)
			queue = append(queue, struct {
				node  uuid.UUID
				depth int
			}{parent.ID, current.depth + 1})
		}
	}

	return result, nil
}

// GetConsequences implements CausalGraphRepository
// Performs a BFS traversal down the causal chain (children) up to maxDepth levels
func (r *SQLiteRepository) GetConsequences(ctx context.Context, id uuid.UUID, maxDepth int) ([]*entities.CausalNode, error) {
	if maxDepth <= 0 {
		maxDepth = 5 // Default depth
	}

	var result []*entities.CausalNode
	visited := make(map[uuid.UUID]bool)
	queue := []struct {
		node  uuid.UUID
		depth int
	}{{id, 0}}

	for len(queue) > 0 {
		// Dequeue
		current := queue[0]
		queue = queue[1:]

		if current.depth >= maxDepth {
			continue
		}

		// Get children of current node
		children, err := r.GetChildren(ctx, current.node)
		if err != nil {
			return nil, err
		}

		for _, child := range children {
			if visited[child.ID] {
				continue
			}
			visited[child.ID] = true
			result = append(result, child)
			queue = append(queue, struct {
				node  uuid.UUID
				depth int
			}{child.ID, current.depth + 1})
		}
	}

	return result, nil
}

// GetParents implements CausalGraphRepository
func (r *SQLiteRepository) GetParents(ctx context.Context, nodeID uuid.UUID, relations ...valueobjects.RelationType) ([]*entities.CausalNode, error) {
	query := `
		SELECT n.id, n.node_type, n.summary, n.timestamp, n.wing, n.room
		FROM causal_nodes n
		JOIN causal_edges e ON n.id = e.from_id
		WHERE e.to_id = ?`
	args := []interface{}{nodeID[:]}

	if len(relations) > 0 {
		placeholders := make([]string, len(relations))
		for i, rel := range relations {
			placeholders[i] = "?"
			args = append(args, string(rel))
		}
		query += " AND e.relation IN (" + placeholders[0] + ")"
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*entities.CausalNode
	for rows.Next() {
		node := &entities.CausalNode{}
		var idBytes []byte
		var timestamp float64
		var room sql.NullString

		err := rows.Scan(&idBytes, &node.Type, &node.Summary, &timestamp, &node.Wing, &room)
		if err != nil {
			continue
		}
		node.ID, _ = uuid.FromBytes(idBytes)
		node.Timestamp = time.Unix(int64(timestamp), 0)
		if room.Valid {
			node.Room = &room.String
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// GetChildren implements CausalGraphRepository
func (r *SQLiteRepository) GetChildren(ctx context.Context, nodeID uuid.UUID, relations ...valueobjects.RelationType) ([]*entities.CausalNode, error) {
	query := `
		SELECT n.id, n.node_type, n.summary, n.timestamp, n.wing, n.room
		FROM causal_nodes n
		JOIN causal_edges e ON n.id = e.to_id
		WHERE e.from_id = ?`
	args := []interface{}{nodeID[:]}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*entities.CausalNode
	for rows.Next() {
		node := &entities.CausalNode{}
		var idBytes []byte
		var timestamp float64
		var room sql.NullString

		err := rows.Scan(&idBytes, &node.Type, &node.Summary, &timestamp, &node.Wing, &room)
		if err != nil {
			continue
		}
		node.ID, _ = uuid.FromBytes(idBytes)
		node.Timestamp = time.Unix(int64(timestamp), 0)
		if room.Valid {
			node.Room = &room.String
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// RegisterModel implements ModelRepository
func (r *SQLiteRepository) RegisterModel(ctx context.Context, model *entities.EmbeddingModel) error {
	metadataJSON, _ := json.Marshal(model.Metadata)
	_, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO embedding_models (model_hash, model_name, dimension, created_at, metadata) 
		 VALUES (?, ?, ?, ?, ?)`,
		model.ModelHash, model.ModelName, model.Dimension, float64(model.CreatedAt.Unix()), string(metadataJSON),
	)
	return err
}

// GetAllModels implements ModelRepository
func (r *SQLiteRepository) GetAllModels(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT model_hash FROM embedding_models`)
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

// GetStats implements StatsRepository
func (r *SQLiteRepository) GetStats(ctx context.Context) (*valueobjects.Stats, error) {
	stats := valueobjects.NewStats()

	_ = r.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(token_count), 0) FROM verbatim`).Scan(&stats.VerbatimCount, &stats.TotalTokens)
	_ = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM fingerprints`).Scan(&stats.FingerprintCount)
	_ = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM embeddings`).Scan(&stats.EmbeddingCount)
	_ = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM causal_nodes`).Scan(&stats.CausalNodeCount)
	_ = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM causal_edges`).Scan(&stats.CausalEdgeCount)

	rows, _ := r.db.QueryContext(ctx, `SELECT ftype, COUNT(*) FROM fingerprints GROUP BY ftype`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var t string
			var count int
			if err := rows.Scan(&t, &count); err == nil {
				stats.TypeCounts[t] = count
			}
		}
	}

	row, _ := r.db.QueryContext(ctx, `SELECT DISTINCT wing FROM verbatim ORDER BY wing LIMIT 20`)
	if row != nil {
		defer row.Close()
		for row.Next() {
			var wing string
			if err := row.Scan(&wing); err == nil {
				stats.ActiveWings = append(stats.ActiveWings, wing)
			}
		}
	}

	return stats, nil
}

// GetTimeline implements StatsRepository
func (r *SQLiteRepository) GetTimeline(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string, limit int, cursor *string) ([]*valueobjects.TimelineItem, error) {
	query := `
		SELECT v.id, f.ftype, f.extracted_at, f.data
		FROM fingerprints f
		JOIN verbatim v ON f.verbatim_id = v.id
		WHERE v.wing = ?`
	args := []interface{}{wing}

	if room != nil {
		query += " AND v.room = ?"
		args = append(args, *room)
	}
	if memType != nil {
		query += " AND f.ftype = ?"
		args = append(args, string(*memType))
	}
	if since != nil {
		query += " AND f.extracted_at >= ?"
		args = append(args, *since)
	}
	if until != nil {
		query += " AND f.extracted_at <= ?"
		args = append(args, *until)
	}
	if cursor != nil && *cursor != "" {
		// Cursor is an RFC3339 timestamp used for pagination
		t, err := time.Parse(time.RFC3339, *cursor)
		if err == nil {
			query += " AND f.extracted_at < ?"
			args = append(args, float64(t.Unix()))
		}
	}

	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	query += fmt.Sprintf(" ORDER BY f.extracted_at DESC LIMIT %d", limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*valueobjects.TimelineItem
	for rows.Next() {
		var id []byte
		var memTypeStr string
		var extractedAt float64
		var dataJSON []byte

		if err := rows.Scan(&id, &memTypeStr, &extractedAt, &dataJSON); err != nil {
			continue
		}

		uid, err := uuid.FromBytes(id)
		if err != nil {
			continue
		}

		var data valueobjects.FingerprintData
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

		items = append(items, &valueobjects.TimelineItem{
			ID:        uid.String(),
			Timestamp: time.Unix(int64(extractedAt), 0).Format("2006-01-02 15:04"),
			Type:      valueobjects.MemoryType(memTypeStr),
			Summary:   summary,
		})
	}

	return items, nil
}

// ArchiveOldMemories implements StatsRepository
func (r *SQLiteRepository) ArchiveOldMemories(ctx context.Context) (*valueobjects.ArchiveResult, error) {
	result := &valueobjects.ArchiveResult{}
	now := float64(time.Now().Unix())

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin archive transaction: %w", err)
	}
	defer tx.Rollback()

	// Archive session notes
	sessionThreshold := now - float64(r.opts.SessionNoteArchiveDays*24*60*60)
	sessionIDs, sessionTokens := r.collectArchiveTargets(ctx, tx, "session_note", sessionThreshold)
	result.SessionNotes = len(sessionIDs)
	result.TokensFreed += sessionTokens

	// Archive debug logs
	debugThreshold := now - float64(r.opts.DebugLogArchiveDays*24*60*60)
	debugIDs, debugTokens := r.collectArchiveTargets(ctx, tx, "debug_log", debugThreshold)
	result.DebugLogs = len(debugIDs)
	result.TokensFreed += debugTokens

	// Delete all related data
	allIDs := append(sessionIDs, debugIDs...)
	for _, id := range allIDs {
		idBytes := id[:]
		_, _ = tx.ExecContext(ctx, `DELETE FROM causal_edges WHERE from_id = ? OR to_id = ?`, idBytes, idBytes)
		_, _ = tx.ExecContext(ctx, `DELETE FROM causal_nodes WHERE id = ?`, idBytes)
		_, _ = tx.ExecContext(ctx, `DELETE FROM embeddings WHERE id = ?`, idBytes)
		_, _ = tx.ExecContext(ctx, `DELETE FROM fingerprints WHERE id = ? OR verbatim_id = ?`, idBytes, idBytes)
		_, _ = tx.ExecContext(ctx, `DELETE FROM verbatim WHERE id = ?`, idBytes)
	}

	_, _ = tx.ExecContext(ctx, `DELETE FROM overlap_cache WHERE ttl < ?`, now)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit archive transaction: %w", err)
	}

	return result, nil
}

// ClearAll removes all memories and related data from the store.
func (r *SQLiteRepository) ClearAll(ctx context.Context) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin clear transaction: %w", err)
	}
	defer tx.Rollback()

	_, _ = tx.ExecContext(ctx, `DELETE FROM causal_edges`)
	_, _ = tx.ExecContext(ctx, `DELETE FROM causal_nodes`)
	_, _ = tx.ExecContext(ctx, `DELETE FROM embeddings`)
	_, _ = tx.ExecContext(ctx, `DELETE FROM fingerprints`)
	_, _ = tx.ExecContext(ctx, `DELETE FROM verbatim`)
	_, _ = tx.ExecContext(ctx, `DELETE FROM overlap_cache`)

	return tx.Commit()
}

// ClearByRoom removes all memories and related data for a specific wing/room.
func (r *SQLiteRepository) ClearByRoom(ctx context.Context, wing string, room *string) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin clear transaction: %w", err)
	}
	defer tx.Rollback()

	var roomCondition string
	args := []interface{}{wing}
	if room != nil {
		roomCondition = "AND room = ?"
		args = append(args, *room)
	} else {
		roomCondition = "AND room IS NULL"
	}

	var count int
	err = tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM verbatim WHERE wing = ? "+roomCondition,
		args...,
	).Scan(&count)
	if err != nil {
		return 0, err
	}
	if count == 0 {
		_ = tx.Commit()
		return 0, nil
	}

	_, _ = tx.ExecContext(ctx,
		`DELETE FROM causal_edges WHERE from_id IN (
			SELECT id FROM fingerprints WHERE verbatim_id IN (
				SELECT id FROM verbatim WHERE wing = ? `+roomCondition+`
			)
		) OR to_id IN (
			SELECT id FROM fingerprints WHERE verbatim_id IN (
				SELECT id FROM verbatim WHERE wing = ? `+roomCondition+`
			)
		)`,
		append(append([]interface{}{}, args...), args...)...,
	)

	_, _ = tx.ExecContext(ctx,
		`DELETE FROM causal_nodes WHERE id IN (
			SELECT id FROM fingerprints WHERE verbatim_id IN (
				SELECT id FROM verbatim WHERE wing = ? `+roomCondition+`
			)
		)`,
		args...,
	)

	_, _ = tx.ExecContext(ctx,
		`DELETE FROM embeddings WHERE id IN (
			SELECT id FROM verbatim WHERE wing = ? `+roomCondition+`
		)`,
		args...,
	)

	_, _ = tx.ExecContext(ctx,
		`DELETE FROM fingerprints WHERE verbatim_id IN (
			SELECT id FROM verbatim WHERE wing = ? `+roomCondition+`
		)`,
		args...,
	)

	_, err = tx.ExecContext(ctx,
		`DELETE FROM verbatim WHERE wing = ? `+roomCondition,
		args...,
	)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit clear transaction: %w", err)
	}

	return count, nil
}

func (r *SQLiteRepository) collectArchiveTargets(ctx context.Context, tx *sql.Tx, ftype string, threshold float64) ([]uuid.UUID, int) {
	rows, err := tx.QueryContext(ctx,
		`SELECT v.id, v.token_count FROM verbatim v
		 JOIN fingerprints f ON v.id = f.verbatim_id
		 WHERE v.created_at < ? AND f.ftype = ?`,
		threshold, ftype,
	)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()

	var ids []uuid.UUID
	totalTokens := 0
	for rows.Next() {
		var idBytes []byte
		var tokenCount int
		if err := rows.Scan(&idBytes, &tokenCount); err != nil {
			continue
		}
		id, err := uuid.FromBytes(idBytes)
		if err != nil {
			continue
		}
		ids = append(ids, id)
		totalTokens += tokenCount
	}

	return ids, totalTokens
}

// GetCandidatesWithEmbeddings implements EmbeddingSource
// Retrieves candidates (fingerprint, verbatim, embedding) by their IDs
func (r *SQLiteRepository) GetCandidatesWithEmbeddings(ctx context.Context, ids []uuid.UUID, wing, room *string) ([]*entities.Candidate, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id[:]
	}

	query := fmt.Sprintf(`
		SELECT v.id, v.content, v.wing, v.room, v.token_count, v.created_at,
			   f.id, f.ftype, f.fact_count, f.token_estimate, f.model_hash, f.data,
			   e.vector, e.dim
		FROM verbatim v
		JOIN fingerprints f ON v.id = f.verbatim_id
		JOIN embeddings e ON v.id = e.id
		WHERE v.id IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("batch query failed: %w", err)
	}
	defer rows.Close()

	var candidates []*entities.Candidate
	for rows.Next() {
		var vID, fID []byte
		var vContent, vWing, fType, fModelHash string
		var vRoom sql.NullString
		var vTokenCount, fFactCount, fTokenEstimate, eDim int
		var vCreatedAt float64
		var fData []byte
		var eVector []byte

		err := rows.Scan(
			&vID, &vContent, &vWing, &vRoom, &vTokenCount, &vCreatedAt,
			&fID, &fType, &fFactCount, &fTokenEstimate, &fModelHash, &fData,
			&eVector, &eDim,
		)
		if err != nil {
			continue
		}

		// Apply wing/room filters
		if wing != nil && vWing != *wing {
			continue
		}
		if room != nil && (!vRoom.Valid || vRoom.String != *room) {
			continue
		}

		// Parse UUID
		id, err := uuid.FromBytes(vID)
		if err != nil {
			continue
		}

		// Decode embedding vector
		vec := make([]float32, eDim)
		vecLen := len(eVector) / 4
		if vecLen > eDim {
			vecLen = eDim
		}
		for i := 0; i < vecLen; i++ {
			u := binary.LittleEndian.Uint32(eVector[i*4 : i*4+4])
			vec[i] = math.Float32frombits(u)
		}

		// Build entities
		verbatim := &entities.Verbatim{
			ID:         id,
			Content:    vContent,
			Wing:       vWing,
			TokenCount: vTokenCount,
			CreatedAt:  time.Unix(int64(vCreatedAt), 0),
		}
		if vRoom.Valid {
			verbatim.Room = &vRoom.String
		}

		fpID, _ := uuid.FromBytes(fID)
		fp := &entities.Fingerprint{
			ID:            fpID,
			VerbatimID:    id,
			Type:          valueobjects.MemoryType(fType),
			FactCount:     fFactCount,
			TokenEstimate: fTokenEstimate,
			ModelHash:     fModelHash,
		}
		_ = json.Unmarshal(fData, &fp.Data)

		candidates = append(candidates, entities.NewCandidate(fp, verbatim, vec))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return candidates, nil
}

// GetAllEmbeddings implements EmbeddingSource
// Retrieves all embeddings from the store
func (r *SQLiteRepository) GetAllEmbeddings(ctx context.Context) ([]*entities.Embedding, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT v.id, e.vector, e.dim
		FROM verbatim v
		JOIN embeddings e ON v.id = e.id
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query embeddings: %w", err)
	}
	defer rows.Close()

	var embeddings []*entities.Embedding
	for rows.Next() {
		var idBytes, vectorBytes []byte
		var dim int
		if err := rows.Scan(&idBytes, &vectorBytes, &dim); err != nil {
			continue
		}

		id, err := uuid.FromBytes(idBytes)
		if err != nil {
			continue
		}

		// Decode vector
		vec := make([]float32, dim)
		vecLen := len(vectorBytes) / 4
		if vecLen > dim {
			vecLen = dim
		}
		for i := 0; i < vecLen; i++ {
			u := binary.LittleEndian.Uint32(vectorBytes[i*4 : i*4+4])
			vec[i] = math.Float32frombits(u)
		}

		embeddings = append(embeddings, &entities.Embedding{
			ID:     id,
			Vector: vec,
			Dim:    dim,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return embeddings, nil
}
