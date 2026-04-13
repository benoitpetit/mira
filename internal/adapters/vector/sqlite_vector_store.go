// SQLite vector store adapter
package vector

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"math"
	"sort"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/benoitpetit/mira/internal/util"
	"github.com/google/uuid"
)

// SQLiteVectorStore implements VectorStore using SQLite with cosine similarity
type SQLiteVectorStore struct {
	db *sql.DB
}

// NewSQLiteVectorStore creates a new SQLite vector store
func NewSQLiteVectorStore(db *sql.DB) *SQLiteVectorStore {
	return &SQLiteVectorStore{db: db}
}

// Search implements VectorStore
func (s *SQLiteVectorStore) Search(ctx context.Context, vector []float32, limit int, wing, room *string) ([]*entities.Candidate, error) {
	// Get all embeddings and compute cosine similarity in memory
	// This is O(n) but acceptable for small datasets
	// We fetch more than limit to ensure good results after sorting by similarity
	fetchLimit := limit * 3
	if fetchLimit < 100 {
		fetchLimit = 100
	}

	query := `
		SELECT v.id, v.content, v.token_count, v.created_at, v.wing, v.room,
		       f.id, f.ftype, f.extracted_at, f.entities, f.subjects, f.decision, f.data, f.fact_count, f.token_estimate, f.model_hash,
		       e.dim, e.vector
		FROM verbatim v
		JOIN fingerprints f ON v.id = f.verbatim_id
		JOIN embeddings e ON v.id = e.id
		WHERE 1=1`
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
	args = append(args, fetchLimit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []*entities.Candidate

	for rows.Next() {
		var v entities.Verbatim
		var fp entities.Fingerprint

		var vID, fpID []byte
		var room sql.NullString
		var createdAt float64
		var extractedAt float64
		var entitiesJSON, subjectsJSON []byte
		var decision sql.NullString
		var dataJSON []byte
		var vectorBytes []byte
		var dim int

		err := rows.Scan(
			&vID, &v.Content, &v.TokenCount, &createdAt, &v.Wing, &room,
			&fpID, &fp.Type, &extractedAt, &entitiesJSON, &subjectsJSON, &decision, &dataJSON, &fp.FactCount, &fp.TokenEstimate, &fp.ModelHash,
			&dim, &vectorBytes,
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
		_ = json.Unmarshal(entitiesJSON, &fp.Entities)
		_ = json.Unmarshal(subjectsJSON, &fp.Subjects)
		_ = json.Unmarshal(dataJSON, &fp.Data)

		// Decode vector
		vecLen := len(vectorBytes) / 4
		embVec := make([]float32, vecLen)
		for i := 0; i < vecLen && i*4+3 < len(vectorBytes); i++ {
			u := binary.LittleEndian.Uint32(vectorBytes[i*4 : i*4+4])
			embVec[i] = math.Float32frombits(u)
		}

		candidates = append(candidates, entities.NewCandidate(&fp, &v, embVec))
	}

	// Sort by cosine similarity to query vector (highest first)
	sort.Slice(candidates, func(i, j int) bool {
		simI := util.CosineSimilarity(candidates[i].Embedding, vector)
		simJ := util.CosineSimilarity(candidates[j].Embedding, vector)
		return simI > simJ
	})

	// Limit results
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	return candidates, nil
}



// AddCandidate implements VectorStore
func (s *SQLiteVectorStore) AddCandidate(ctx context.Context, candidate *entities.Candidate) error {
	// No-op for SQLite - data is already stored via EmbeddingRepository
	return nil
}

// Delete implements VectorStore
func (s *SQLiteVectorStore) Delete(ctx context.Context, id uuid.UUID) error {
	// No-op for now
	return nil
}

// ClearAll implements VectorStore
func (s *SQLiteVectorStore) ClearAll(ctx context.Context) error {
	// No-op: data lives in SQLite and is cleared by the repository
	return nil
}

// ClearByRoom implements VectorStore
func (s *SQLiteVectorStore) ClearByRoom(ctx context.Context, wing string, room *string) error {
	// No-op: data lives in SQLite and is cleared by the repository
	return nil
}

// Ensure SQLiteVectorStore implements VectorStore
var _ ports.VectorStore = (*SQLiteVectorStore)(nil)
