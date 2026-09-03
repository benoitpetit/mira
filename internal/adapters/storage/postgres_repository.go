// PostgreSQL repository implementation
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// PostgreSQLRepository implements all repository interfaces using PostgreSQL
type PostgreSQLRepository struct {
	db   *sql.DB
	opts PostgreSQLOptions
}

// PostgreSQLOptions holds configuration for PostgreSQL repository
type PostgreSQLOptions struct {
	URL         string
	MaxConns    int
	MinConns    int
	MaxIdleTime time.Duration
	MaxConnTime time.Duration
}

// NewPostgreSQLRepository creates a new PostgreSQL repository
func NewPostgreSQLRepository(opts PostgreSQLOptions) (*PostgreSQLRepository, error) {
	db, err := sql.Open("pgx", opts.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(opts.MaxConns)
	db.SetMaxIdleConns(opts.MinConns)
	db.SetConnMaxIdleTime(opts.MaxIdleTime)
	db.SetConnMaxLifetime(opts.MaxConnTime)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if err := runPostgresMigrations(db); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	return &PostgreSQLRepository{db: db, opts: opts}, nil
}

// Close closes the database connection
func (r *PostgreSQLRepository) Close() error {
	return r.db.Close()
}

// Begin starts a transaction
func (r *PostgreSQLRepository) Begin() (*sql.Tx, error) {
	return r.db.Begin()
}

// DB returns the underlying database connection
func (r *PostgreSQLRepository) DB() *sql.DB {
	return r.db
}

// StoreVerbatim implements VerbatimRepository
func (r *PostgreSQLRepository) StoreVerbatim(ctx context.Context, verbatim *entities.Verbatim) error {
	tx, err := r.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // intentional: no-op if commit succeeds

	if err := r.StoreVerbatimTx(ctx, tx, verbatim); err != nil {
		return err
	}

	return tx.Commit()
}

// StoreVerbatimTx implements VerbatimRepository
func (r *PostgreSQLRepository) StoreVerbatimTx(ctx context.Context, tx *sql.Tx, v *entities.Verbatim) error {
	metadataJSON, err := json.Marshal(v.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	metricsJSON, _ := json.Marshal(v.Metrics)

	_, err = tx.ExecContext(ctx,
		`INSERT INTO verbatim (id, content, token_count, created_at, valid_from, valid_until, kind, wing, room, metadata, metrics)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		v.ID, v.Content, v.TokenCount, float64(v.CreatedAt.Unix()), unixTimeOrNil(v.ValidFrom), unixTimeOrNil(v.ValidUntil), v.Kind, v.Wing, v.Room, metadataJSON, metricsJSON,
	)
	return err
}

// DeleteVerbatimByID implements VerbatimRepository
func (r *PostgreSQLRepository) DeleteVerbatimByID(ctx context.Context, id uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin delete transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // intentional: no-op if commit succeeds

	if err := r.DeleteVerbatimByIDTx(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteVerbatimByIDTx implements VerbatimRepository
func (r *PostgreSQLRepository) DeleteVerbatimByIDTx(ctx context.Context, tx *sql.Tx, id uuid.UUID) error {
	// Delete causal relations for fingerprints associated with this verbatim
	_, _ = tx.ExecContext(ctx, `
		DELETE FROM causal_edges WHERE from_id IN (
			SELECT id FROM fingerprints WHERE verbatim_id = $1
		) OR to_id IN (
			SELECT id FROM fingerprints WHERE verbatim_id = $2
		)`, id, id)

	// Delete causal nodes
	_, _ = tx.ExecContext(ctx, `
		DELETE FROM causal_nodes WHERE id IN (
			SELECT id FROM fingerprints WHERE verbatim_id = $1
		)`, id)

	// Delete embeddings
	_, _ = tx.ExecContext(ctx, `DELETE FROM embeddings WHERE id = $1`, id)

	// Delete fingerprints
	_, _ = tx.ExecContext(ctx, `DELETE FROM fingerprints WHERE verbatim_id = $1`, id)

	// Delete verbatim
	_, err := tx.ExecContext(ctx, `DELETE FROM verbatim WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete verbatim: %w", err)
	}
	return nil
}

// GetVerbatimByID implements VerbatimRepository
func (r *PostgreSQLRepository) GetVerbatimByID(ctx context.Context, id uuid.UUID) (*entities.Verbatim, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, content, token_count, created_at, valid_from, valid_until, kind, wing, room, metadata, metrics, summary, summary_tokens FROM verbatim WHERE id = $1`,
		id,
	)

	var v entities.Verbatim
	var metadataJSON, metricsJSON []byte
	var room sql.NullString
	var summary sql.NullString
	var createdAt float64
	var validFrom, validUntil sql.NullFloat64

	err := row.Scan(&v.ID, &v.Content, &v.TokenCount, &createdAt, &validFrom, &validUntil, &v.Kind, &v.Wing, &room, &metadataJSON, &metricsJSON, &summary, &v.SummaryTokenCount)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &NotFoundError{Resource: "verbatim"}
		}
		return nil, err
	}

	v.CreatedAt = time.Unix(int64(createdAt), 0)
	v.ValidFrom = nullableUnixTime(validFrom)
	v.ValidUntil = nullableUnixTime(validUntil)
	if room.Valid {
		v.Room = &room.String
	}
	if summary.Valid && summary.String != "" {
		v.Summary = &summary.String
	}
	if len(metadataJSON) > 0 {
		_ = json.Unmarshal(metadataJSON, &v.Metadata)
	}
	if len(metricsJSON) > 0 {
		_ = json.Unmarshal(metricsJSON, &v.Metrics)
	}

	return &v, nil
}

// UpdateVerbatimSummary implements VerbatimRepository
func (r *PostgreSQLRepository) UpdateVerbatimSummary(ctx context.Context, id uuid.UUID, summary string, summaryTokens int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE verbatim SET summary = $1, summary_tokens = $2 WHERE id = $3`,
		summary, summaryTokens, id,
	)
	return err
}

// StoreFingerprint implements FingerprintRepository
func (r *PostgreSQLRepository) StoreFingerprint(ctx context.Context, fp *entities.Fingerprint) error {
	tx, err := r.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // intentional: no-op if commit succeeds

	if err := r.StoreFingerprintTx(ctx, tx, fp); err != nil {
		return err
	}

	return tx.Commit()
}

// StoreFingerprintTx implements FingerprintRepository
func (r *PostgreSQLRepository) StoreFingerprintTx(ctx context.Context, tx *sql.Tx, fp *entities.Fingerprint) error {
	entitiesJSON, _ := json.Marshal(fp.Entities)
	subjectsJSON, _ := json.Marshal(fp.Subjects)
	dataJSON, _ := json.Marshal(fp.Data)

	var decision *string
	if fp.Decision != nil && *fp.Decision != "" {
		decision = fp.Decision
	}

	_, err := tx.ExecContext(ctx,
		`INSERT INTO fingerprints (id, verbatim_id, ftype, extracted_at, entities, subjects, decision, data, fact_count, token_estimate, model_hash)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		fp.ID, fp.VerbatimID, string(fp.Type), float64(fp.ExtractedAt.Unix()),
		entitiesJSON, subjectsJSON, decision, dataJSON,
		fp.FactCount, fp.TokenEstimate, fp.ModelHash,
	)
	return err
}

// GetFingerprintByID implements FingerprintRepository
func (r *PostgreSQLRepository) GetFingerprintByID(ctx context.Context, id uuid.UUID) (*entities.Fingerprint, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT f.id, f.verbatim_id, f.ftype, f.extracted_at, f.entities, f.subjects, f.decision, f.data, f.fact_count, f.token_estimate, f.model_hash
		 FROM fingerprints f
		 WHERE f.id = $1`,
		id,
	)

	var fp entities.Fingerprint
	var ftype string
	var extractedAt float64
	var entitiesJSON, subjectsJSON, dataJSON []byte
	var decision sql.NullString

	err := row.Scan(&fp.ID, &fp.VerbatimID, &ftype, &extractedAt, &entitiesJSON, &subjectsJSON, &decision, &dataJSON, &fp.FactCount, &fp.TokenEstimate, &fp.ModelHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &NotFoundError{Resource: "fingerprint"}
		}
		return nil, err
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
func (r *PostgreSQLRepository) GetFingerprintByVerbatimID(ctx context.Context, verbatimID uuid.UUID) (*entities.Fingerprint, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT f.id, f.verbatim_id, f.ftype, f.extracted_at, f.entities, f.subjects, f.decision, f.data, f.fact_count, f.token_estimate, f.model_hash
		 FROM fingerprints f
		 WHERE f.verbatim_id = $1`,
		verbatimID,
	)

	var fp entities.Fingerprint
	var ftype string
	var extractedAt float64
	var entitiesJSON, subjectsJSON, dataJSON []byte
	var decision sql.NullString

	err := row.Scan(&fp.ID, &fp.VerbatimID, &ftype, &extractedAt, &entitiesJSON, &subjectsJSON, &decision, &dataJSON, &fp.FactCount, &fp.TokenEstimate, &fp.ModelHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &NotFoundError{Resource: "fingerprint"}
		}
		return nil, err
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
func (r *PostgreSQLRepository) GetRecentFingerprintsByWing(ctx context.Context, wing string, excludeID uuid.UUID, limit int) ([]*entities.Fingerprint, error) {
	tx, err := r.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck // intentional: no-op if commit succeeds
	return r.GetRecentFingerprintsByWingTx(ctx, tx, wing, excludeID, limit)
}

// GetRecentFingerprintsByWingTx implements FingerprintRepository
func (r *PostgreSQLRepository) GetRecentFingerprintsByWingTx(ctx context.Context, tx *sql.Tx, wing string, excludeID uuid.UUID, limit int) ([]*entities.Fingerprint, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT f.id, f.verbatim_id, f.ftype, f.extracted_at, f.entities, f.subjects, f.decision, f.data, f.fact_count, f.token_estimate, f.model_hash
		 FROM fingerprints f
		 JOIN verbatim v ON f.verbatim_id = v.id
		 WHERE v.wing = $1 AND f.id != $2
		 ORDER BY f.extracted_at DESC
		 LIMIT $3`,
		wing, excludeID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fingerprints []*entities.Fingerprint
	for rows.Next() {
		fp := &entities.Fingerprint{}
		var ftype string
		var extractedAt float64
		var entitiesJSON, subjectsJSON, dataJSON []byte
		var decision sql.NullString

		err := rows.Scan(&fp.ID, &fp.VerbatimID, &ftype, &extractedAt, &entitiesJSON, &subjectsJSON, &decision, &dataJSON, &fp.FactCount, &fp.TokenEstimate, &fp.ModelHash)
		if err != nil {
			continue
		}

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
func (r *PostgreSQLRepository) StoreEmbedding(ctx context.Context, emb *entities.Embedding) error {
	tx, err := r.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // intentional: no-op if commit succeeds

	if err := r.StoreEmbeddingTx(ctx, tx, emb); err != nil {
		return err
	}

	return tx.Commit()
}

// StoreEmbeddingTx implements EmbeddingRepository
func (r *PostgreSQLRepository) StoreEmbeddingTx(ctx context.Context, tx *sql.Tx, emb *entities.Embedding) error {
	// pgvector uses a specialized format for vectors: [1,2,3]
	// We'll handle this in PGVectorStore which will use the VECTOR type.
	// This Repository method is for generic embedding storage (T2).
	// We'll store it as a BYTEA in Postgres if not using pgvector,
	// but since we want to support pgvector, we'll use its format.
	// Wait, if I use the VECTOR type, I need to pass a string or a float array.

	vectorStr := "["
	for i, v := range emb.Vector {
		if i > 0 {
			vectorStr += ","
		}
		vectorStr += fmt.Sprintf("%f", v)
	}
	vectorStr += "]"

	_, err := tx.ExecContext(ctx,
		`INSERT INTO embeddings (id, model_hash, dim, vector, normalized, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		emb.ID, emb.ModelHash, emb.Dim, vectorStr, emb.Normalized, float64(emb.CreatedAt.Unix()),
	)
	return err
}

// GetEmbeddingByID implements EmbeddingRepository
func (r *PostgreSQLRepository) GetEmbeddingByID(ctx context.Context, id uuid.UUID) (*entities.Embedding, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, model_hash, dim, vector::float4[], normalized, created_at FROM embeddings WHERE id = $1`,
		id,
	)

	var emb entities.Embedding
	var createdAt float64
	var vector []float32

	// pgx can scan float4[] into []float32
	err := row.Scan(&emb.ID, &emb.ModelHash, &emb.Dim, &vector, &emb.Normalized, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("embedding not found")
		}
		return nil, err
	}

	emb.CreatedAt = time.Unix(int64(createdAt), 0)
	emb.Vector = vector

	return &emb, nil
}

// AddNode implements CausalGraphRepository
func (r *PostgreSQLRepository) AddNode(ctx context.Context, node *entities.CausalNode) error {
	tx, err := r.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // intentional: no-op if commit succeeds

	if err := r.AddNodeTx(ctx, tx, node); err != nil {
		return err
	}

	return tx.Commit()
}

// AddNodeTx implements CausalGraphRepository
func (r *PostgreSQLRepository) AddNodeTx(ctx context.Context, tx *sql.Tx, node *entities.CausalNode) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO causal_nodes (id, node_type, summary, timestamp, wing, room)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		node.ID, node.Type, node.Summary, float64(node.Timestamp.Unix()), node.Wing, node.Room,
	)
	return err
}

// AddEdge implements CausalGraphRepository
func (r *PostgreSQLRepository) AddEdge(ctx context.Context, edge *entities.CausalEdge) error {
	tx, err := r.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // intentional: no-op if commit succeeds

	if err := r.AddEdgeTx(ctx, tx, edge); err != nil {
		return err
	}

	return tx.Commit()
}

// AddEdgeTx implements CausalGraphRepository
func (r *PostgreSQLRepository) AddEdgeTx(ctx context.Context, tx *sql.Tx, edge *entities.CausalEdge) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO causal_edges (from_id, to_id, relation, weight, detected_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (from_id, to_id, relation) DO NOTHING`,
		edge.FromID, edge.ToID, string(edge.Relation), edge.Weight, float64(edge.DetectedAt.Unix()),
	)
	return err
}

// HasEdge implements CausalGraphRepository
func (r *PostgreSQLRepository) HasEdge(ctx context.Context, fromID, toID uuid.UUID) bool {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM causal_edges WHERE (from_id = $1 AND to_id = $2) OR (from_id = $3 AND to_id = $4)`,
		fromID, toID, toID, fromID,
	).Scan(&count)
	return err == nil && count > 0
}

// GetChain implements CausalGraphRepository
func (r *PostgreSQLRepository) GetChain(ctx context.Context, id uuid.UUID, maxDepth int) ([]*entities.CausalNode, error) {
	if maxDepth <= 0 {
		maxDepth = 5
	}

	query := `
		WITH RECURSIVE ancestors(id, node_type, summary, timestamp, wing, room, depth) AS (
			SELECT id, node_type, summary, timestamp, wing, room, 0
			FROM causal_nodes
			WHERE id = $1
			UNION ALL
			SELECT n.id, n.node_type, n.summary, n.timestamp, n.wing, n.room, a.depth + 1
			FROM causal_nodes n
			JOIN causal_edges e ON n.id = e.from_id
			JOIN ancestors a ON e.to_id = a.id
			WHERE a.depth < $2
		)
		SELECT id, node_type, summary, timestamp, wing, room
		FROM ancestors
		WHERE depth > 0
		ORDER BY depth DESC
	`

	rows, err := r.db.QueryContext(ctx, query, id, maxDepth)
	if err != nil {
		return nil, fmt.Errorf("recursive chain query failed: %w", err)
	}
	defer rows.Close()

	return r.scanCausalNodes(rows)
}

// GetConsequences implements CausalGraphRepository
func (r *PostgreSQLRepository) GetConsequences(ctx context.Context, id uuid.UUID, maxDepth int) ([]*entities.CausalNode, error) {
	if maxDepth <= 0 {
		maxDepth = 5
	}

	query := `
		WITH RECURSIVE descendants(id, node_type, summary, timestamp, wing, room, depth) AS (
			SELECT id, node_type, summary, timestamp, wing, room, 0
			FROM causal_nodes
			WHERE id = $1
			UNION ALL
			SELECT n.id, n.node_type, n.summary, n.timestamp, n.wing, n.room, d.depth + 1
			FROM causal_nodes n
			JOIN causal_edges e ON n.id = e.to_id
			JOIN descendants d ON e.from_id = d.id
			WHERE d.depth < $2
		)
		SELECT id, node_type, summary, timestamp, wing, room
		FROM descendants
		WHERE depth > 0
		ORDER BY depth ASC
	`

	rows, err := r.db.QueryContext(ctx, query, id, maxDepth)
	if err != nil {
		return nil, fmt.Errorf("recursive consequences query failed: %w", err)
	}
	defer rows.Close()

	return r.scanCausalNodes(rows)
}

func (r *PostgreSQLRepository) scanCausalNodes(rows *sql.Rows) ([]*entities.CausalNode, error) {
	var nodes []*entities.CausalNode
	for rows.Next() {
		node := &entities.CausalNode{}
		var timestamp float64
		var room sql.NullString

		err := rows.Scan(&node.ID, &node.Type, &node.Summary, &timestamp, &node.Wing, &room)
		if err != nil {
			continue
		}
		node.Timestamp = time.Unix(int64(timestamp), 0)
		if room.Valid {
			node.Room = &room.String
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

// GetParents implements CausalGraphRepository
func (r *PostgreSQLRepository) GetParents(ctx context.Context, nodeID uuid.UUID, relations ...valueobjects.RelationType) ([]*entities.CausalNode, error) {
	query := `
		SELECT n.id, n.node_type, n.summary, n.timestamp, n.wing, n.room
		FROM causal_nodes n
		JOIN causal_edges e ON n.id = e.from_id
		WHERE e.to_id = $1`
	args := []interface{}{nodeID}

	if len(relations) > 0 {
		placeholders := make([]string, len(relations))
		for i, rel := range relations {
			placeholders[i] = fmt.Sprintf("$%d", i+2)
			args = append(args, string(rel))
		}
		query += " AND e.relation IN (" + strings.Join(placeholders, ",") + ")"
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanCausalNodes(rows)
}

// GetChildren implements CausalGraphRepository
func (r *PostgreSQLRepository) GetChildren(ctx context.Context, nodeID uuid.UUID, relations ...valueobjects.RelationType) ([]*entities.CausalNode, error) {
	query := `
		SELECT n.id, n.node_type, n.summary, n.timestamp, n.wing, n.room
		FROM causal_nodes n
		JOIN causal_edges e ON n.id = e.to_id
		WHERE e.from_id = $1`
	args := []interface{}{nodeID}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanCausalNodes(rows)
}

// RegisterModel implements ModelRepository
func (r *PostgreSQLRepository) RegisterModel(ctx context.Context, model *entities.EmbeddingModel) error {
	metadataJSON, _ := json.Marshal(model.Metadata)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO embedding_models (model_hash, model_name, dimension, created_at, metadata)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (model_hash) DO NOTHING`,
		model.ModelHash, model.ModelName, model.Dimension, float64(model.CreatedAt.Unix()), metadataJSON,
	)
	return err
}

// GetAllModels implements ModelRepository
func (r *PostgreSQLRepository) GetAllModels(ctx context.Context) ([]string, error) {
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
func (r *PostgreSQLRepository) GetStats(ctx context.Context) (*valueobjects.Stats, error) {
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
func (r *PostgreSQLRepository) GetTimeline(ctx context.Context, wing string, room *string, memType *valueobjects.MemoryType, since, until *string, limit int, cursor *string) ([]*valueobjects.TimelineItem, error) {
	query := `
		SELECT v.id, f.ftype, f.extracted_at, f.data, v.wing
		FROM fingerprints f
		JOIN verbatim v ON f.verbatim_id = v.id
		WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if wing != "" {
		query += fmt.Sprintf(" AND v.wing = $%d", argIdx)
		args = append(args, wing)
		argIdx++
	}

	if room != nil {
		query += fmt.Sprintf(" AND v.room = $%d", argIdx)
		args = append(args, *room)
		argIdx++
	}
	if memType != nil {
		query += fmt.Sprintf(" AND f.ftype = $%d", argIdx)
		args = append(args, string(*memType))
		argIdx++
	}
	if since != nil {
		query += fmt.Sprintf(" AND f.extracted_at >= $%d", argIdx)
		args = append(args, *since)
		argIdx++
	}
	if until != nil {
		query += fmt.Sprintf(" AND f.extracted_at <= $%d", argIdx)
		args = append(args, *until)
		argIdx++
	}
	if cursor != nil && *cursor != "" {
		t, err := time.Parse(time.RFC3339, *cursor)
		if err == nil {
			query += fmt.Sprintf(" AND f.extracted_at < $%d", argIdx)
			args = append(args, float64(t.Unix()))
			argIdx++
		}
	}

	if limit <= 0 {
		limit = 100
	}
	query += fmt.Sprintf(" ORDER BY f.extracted_at DESC LIMIT %d", limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*valueobjects.TimelineItem
	for rows.Next() {
		var uid uuid.UUID
		var memTypeStr string
		var extractedAt float64
		var dataJSON []byte
		var wingStr string

		if err := rows.Scan(&uid, &memTypeStr, &extractedAt, &dataJSON, &wingStr); err != nil {
			continue
		}

		var data valueobjects.FingerprintData
		_ = json.Unmarshal(dataJSON, &data)

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
			Wing:      wingStr,
		})
	}

	return items, nil
}

// ArchiveOldMemories implements StatsRepository
func (r *PostgreSQLRepository) ArchiveOldMemories(ctx context.Context) (*valueobjects.ArchiveResult, error) {
	// Implementation simplified for now, following SQLite logic
	return &valueobjects.ArchiveResult{}, nil
}

// ClearAll implements StatsRepository
func (r *PostgreSQLRepository) ClearAll(ctx context.Context) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // intentional: no-op if commit succeeds

	_, _ = tx.ExecContext(ctx, `TRUNCATE verbatim CASCADE`)
	_, _ = tx.ExecContext(ctx, `TRUNCATE overlap_cache`)
	_, _ = tx.ExecContext(ctx, `TRUNCATE webhook_dlq`)

	return tx.Commit()
}

// ClearByRoom implements StatsRepository
func (r *PostgreSQLRepository) ClearByRoom(ctx context.Context, wing string, room *string) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck // intentional: no-op if commit succeeds

	var roomCondition string
	args := []interface{}{wing}
	if room != nil {
		roomCondition = "AND room = $2"
		args = append(args, *room)
	} else {
		roomCondition = "AND room IS NULL"
	}

	var count int
	err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM verbatim WHERE wing = $1 "+roomCondition, args...).Scan(&count)
	if err != nil {
		return 0, err
	}

	_, err = tx.ExecContext(ctx, "DELETE FROM verbatim WHERE wing = $1 "+roomCondition, args...)
	if err != nil {
		return 0, err
	}

	return count, tx.Commit()
}

// ClearByIDs implements StatsRepository
func (r *PostgreSQLRepository) ClearByIDs(ctx context.Context, ids []uuid.UUID) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM verbatim WHERE id = ANY($1)`, ids)
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

// StoreTags implements TagRepository
func (r *PostgreSQLRepository) StoreTags(ctx context.Context, verbatimID uuid.UUID, tags []string, tagType string) error {
	if len(tags) == 0 {
		return nil
	}
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		_, _ = r.db.ExecContext(ctx,
			`INSERT INTO memory_tags (verbatim_id, tag, tag_type) VALUES ($1, $2, $3)
			 ON CONFLICT (verbatim_id, tag, tag_type) DO NOTHING`,
			verbatimID, tag, tagType,
		)
	}
	return nil
}

// GetVerbatimsByTags implements TagRepository
func (r *PostgreSQLRepository) GetVerbatimsByTags(ctx context.Context, tags []string, limit int) ([]uuid.UUID, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT verbatim_id FROM memory_tags WHERE tag = ANY($1) LIMIT $2`,
		tags, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// GetTagsForVerbatim implements TagRepository
func (r *PostgreSQLRepository) GetTagsForVerbatim(ctx context.Context, verbatimID uuid.UUID) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT tag FROM memory_tags WHERE verbatim_id = $1`,
		verbatimID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err == nil {
			tags = append(tags, tag)
		}
	}
	return tags, nil
}

// GetCandidatesWithEmbeddings implements EmbeddingSource
func (r *PostgreSQLRepository) GetCandidatesWithEmbeddings(ctx context.Context, ids []uuid.UUID, wing, room *string) ([]*entities.Candidate, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	query := `
		SELECT v.id, v.content, v.wing, v.room, v.token_count, v.created_at, v.valid_from, v.valid_until, v.kind,
			   v.summary, v.summary_tokens,
			   f.id, f.ftype, f.fact_count, f.token_estimate, f.model_hash, f.data,
			   e.vector::float4[]
		FROM verbatim v
		JOIN fingerprints f ON v.id = f.verbatim_id
		JOIN embeddings e ON v.id = e.id
		WHERE v.id = ANY($1)
	`
	rows, err := r.db.QueryContext(ctx, query, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []*entities.Candidate
	for rows.Next() {
		var vID, fID uuid.UUID
		var vContent, vWing, vKind, fType, fModelHash string
		var vRoom sql.NullString
		var vSummary sql.NullString
		var vTokenCount, vSummaryTokens, fFactCount, fTokenEstimate int
		var vCreatedAt float64
		var vValidFrom, vValidUntil sql.NullFloat64
		var fData []byte
		var vector []float32

		err := rows.Scan(
			&vID, &vContent, &vWing, &vRoom, &vTokenCount, &vCreatedAt, &vValidFrom, &vValidUntil, &vKind,
			&vSummary, &vSummaryTokens,
			&fID, &fType, &fFactCount, &fTokenEstimate, &fModelHash, &fData,
			&vector,
		)
		if err != nil {
			continue
		}

		if wing != nil && vWing != *wing {
			continue
		}
		if room != nil && (!vRoom.Valid || vRoom.String != *room) {
			continue
		}

		verbatim := &entities.Verbatim{
			ID:                vID,
			Content:           vContent,
			Wing:              vWing,
			TokenCount:        vTokenCount,
			SummaryTokenCount: vSummaryTokens,
			CreatedAt:         time.Unix(int64(vCreatedAt), 0),
			ValidFrom:         nullableUnixTime(vValidFrom),
			ValidUntil:        nullableUnixTime(vValidUntil),
			Kind:              valueobjects.MemoryKind(vKind),
		}
		if vRoom.Valid {
			verbatim.Room = &vRoom.String
		}
		if vSummary.Valid && vSummary.String != "" {
			verbatim.Summary = &vSummary.String
		}

		fp := &entities.Fingerprint{
			ID:            fID,
			VerbatimID:    vID,
			Type:          valueobjects.MemoryType(fType),
			FactCount:     fFactCount,
			TokenEstimate: fTokenEstimate,
			ModelHash:     fModelHash,
		}
		_ = json.Unmarshal(fData, &fp.Data)

		candidates = append(candidates, entities.NewCandidate(fp, verbatim, vector))
	}

	return candidates, nil
}

// GetAllEmbeddings implements EmbeddingSource
func (r *PostgreSQLRepository) GetAllEmbeddings(ctx context.Context) ([]*entities.Embedding, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT v.id, e.vector::float4[], e.dim
		FROM verbatim v
		JOIN embeddings e ON v.id = e.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var embeddings []*entities.Embedding
	for rows.Next() {
		var id uuid.UUID
		var vector []float32
		var dim int
		if err := rows.Scan(&id, &vector, &dim); err == nil {
			embeddings = append(embeddings, &entities.Embedding{
				ID:     id,
				Vector: vector,
				Dim:    dim,
			})
		}
	}
	return embeddings, nil
}

// SearchLexical implements EmbeddingSource
func (r *PostgreSQLRepository) SearchLexical(ctx context.Context, query string, limit int, wing, room *string) ([]*entities.Candidate, error) {
	// For PostgreSQL, we'll use tsvector for full-text search
	// This requires additional setup in the schema (e.g. tsvector column or functional index)
	// Simplified implementation for now
	return nil, fmt.Errorf("lexical search not yet implemented for PostgreSQL")
}

// SaveAuditLog implements AuditRepository
func (r *PostgreSQLRepository) SaveAuditLog(ctx context.Context, log *entities.AuditLog) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_log (action, actor, resource, status, metadata) VALUES ($1, $2, $3, $4, $5)`,
		log.Action, log.Actor, log.Resource, log.Status, log.Metadata,
	)
	return err
}

// ListAuditLogs implements AuditRepository
func (r *PostgreSQLRepository) ListAuditLogs(ctx context.Context, limit, offset int) ([]*entities.AuditLog, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, timestamp, action, actor, resource, status, metadata FROM audit_log ORDER BY timestamp DESC LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*entities.AuditLog
	for rows.Next() {
		l := &entities.AuditLog{}
		if err := rows.Scan(&l.ID, &l.Timestamp, &l.Action, &l.Actor, &l.Resource, &l.Status, &l.Metadata); err != nil {
			continue
		}
		logs = append(logs, l)
	}
	return logs, nil
}

// GetPolicyByTokenHash implements PolicyRepository
func (r *PostgreSQLRepository) GetPolicyByTokenHash(ctx context.Context, hash string) (*entities.AccessPolicy, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT token_hash, name, wings, created_at, last_used FROM access_policies WHERE token_hash = $1`,
		hash,
	)

	var p entities.AccessPolicy
	var wingsStr string
	var lastUsed sql.NullTime
	err := row.Scan(&p.TokenHash, &p.Name, &wingsStr, &p.CreatedAt, &lastUsed)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("policy not found")
		}
		return nil, err
	}

	if wingsStr != "" {
		p.Wings = strings.Split(wingsStr, ",")
	}
	if lastUsed.Valid {
		p.LastUsed = &lastUsed.Time
	}

	return &p, nil
}

// SavePolicy implements PolicyRepository
func (r *PostgreSQLRepository) SavePolicy(ctx context.Context, p *entities.AccessPolicy) error {
	wingsStr := strings.Join(p.Wings, ",")
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO access_policies (token_hash, name, wings, created_at, last_used)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT(token_hash) DO UPDATE SET name=excluded.name, wings=excluded.wings`,
		p.TokenHash, p.Name, wingsStr, p.CreatedAt, p.LastUsed,
	)
	return err
}

// DeletePolicy implements PolicyRepository
func (r *PostgreSQLRepository) DeletePolicy(ctx context.Context, hash string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM access_policies WHERE token_hash = $1`, hash)
	return err
}

// ListPolicies implements PolicyRepository
func (r *PostgreSQLRepository) ListPolicies(ctx context.Context) ([]*entities.AccessPolicy, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT token_hash, name, wings, created_at, last_used FROM access_policies`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []*entities.AccessPolicy
	for rows.Next() {
		p := &entities.AccessPolicy{}
		var wingsStr string
		var lastUsed sql.NullTime
		if err := rows.Scan(&p.TokenHash, &p.Name, &wingsStr, &p.CreatedAt, &lastUsed); err != nil {
			continue
		}
		if wingsStr != "" {
			p.Wings = strings.Split(wingsStr, ",")
		}
		if lastUsed.Valid {
			p.LastUsed = &lastUsed.Time
		}
		policies = append(policies, p)
	}
	return policies, nil
}

var _ ports.Repository = (*PostgreSQLRepository)(nil)
