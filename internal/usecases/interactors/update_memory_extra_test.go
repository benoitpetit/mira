package interactors

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
	_ "github.com/benoitpetit/go-sqlcipher/v4"
)

// ── mock repo for update extra tests ─────────────────────────────────────────

// updateMockRepo is a fully in-memory repository for UpdateMemory unit tests.
// It lets callers inject errors for specific operations.
type updateMockRepo struct {
	verbatims         map[uuid.UUID]*entities.Verbatim
	fingerprints      map[uuid.UUID]*entities.Fingerprint
	embeddings        map[uuid.UUID]*entities.Embedding
	getVerbatimErr    error
	beginErr          error
	beginReturnsNil   bool
	deleteVerbatimErr error
	storeFpErr        error
	storeEmbErr       error
	beginRolledBack   bool // returns a pre-rolled-back tx so Commit fails
}

func newUpdateMockRepo() *updateMockRepo {
	return &updateMockRepo{
		verbatims:    make(map[uuid.UUID]*entities.Verbatim),
		fingerprints: make(map[uuid.UUID]*entities.Fingerprint),
		embeddings:   make(map[uuid.UUID]*entities.Embedding),
	}
}

func (r *updateMockRepo) Begin() (*sql.Tx, error) {
	if r.beginErr != nil {
		return nil, r.beginErr
	}
	if r.beginReturnsNil {
		return nil, nil
	}
	if r.beginRolledBack {
		db, _ := sql.Open("sqlite3", ":memory:")
		tx, _ := db.Begin()
		_ = tx.Rollback()
		return tx, nil
	}
	return testDB.Begin()
}

func (r *updateMockRepo) GetVerbatimByID(_ context.Context, id uuid.UUID) (*entities.Verbatim, error) {
	if r.getVerbatimErr != nil {
		return nil, r.getVerbatimErr
	}
	v, ok := r.verbatims[id]
	if !ok {
		return nil, errors.New("verbatim not found")
	}
	return v, nil
}

func (r *updateMockRepo) DeleteVerbatimByID(_ context.Context, id uuid.UUID) error {
	if r.deleteVerbatimErr != nil {
		return r.deleteVerbatimErr
	}
	delete(r.verbatims, id)
	return nil
}

func (r *updateMockRepo) DeleteVerbatimByIDTx(_ context.Context, _ *sql.Tx, id uuid.UUID) error {
	if r.deleteVerbatimErr != nil {
		return r.deleteVerbatimErr
	}
	delete(r.verbatims, id)
	return nil
}

func (r *updateMockRepo) StoreVerbatim(_ context.Context, v *entities.Verbatim) error {
	r.verbatims[v.ID] = v
	return nil
}
func (r *updateMockRepo) StoreVerbatimTx(_ context.Context, _ *sql.Tx, v *entities.Verbatim) error {
	r.verbatims[v.ID] = v
	return nil
}

func (r *updateMockRepo) StoreFingerprint(_ context.Context, fp *entities.Fingerprint) error {
	if r.storeFpErr != nil {
		return r.storeFpErr
	}
	r.fingerprints[fp.ID] = fp
	return nil
}
func (r *updateMockRepo) StoreFingerprintTx(_ context.Context, _ *sql.Tx, fp *entities.Fingerprint) error {
	if r.storeFpErr != nil {
		return r.storeFpErr
	}
	r.fingerprints[fp.ID] = fp
	return nil
}

func (r *updateMockRepo) StoreEmbedding(_ context.Context, emb *entities.Embedding) error {
	if r.storeEmbErr != nil {
		return r.storeEmbErr
	}
	r.embeddings[emb.ID] = emb
	return nil
}
func (r *updateMockRepo) StoreEmbeddingTx(_ context.Context, _ *sql.Tx, emb *entities.Embedding) error {
	if r.storeEmbErr != nil {
		return r.storeEmbErr
	}
	r.embeddings[emb.ID] = emb
	return nil
}

// Stubs for remaining interface methods.
func (r *updateMockRepo) GetFingerprintByID(_ context.Context, _ uuid.UUID) (*entities.Fingerprint, error) {
	return nil, nil
}
func (r *updateMockRepo) GetFingerprintByVerbatimID(_ context.Context, _ uuid.UUID) (*entities.Fingerprint, error) {
	return nil, errors.New("not found")
}
func (r *updateMockRepo) GetRecentFingerprintsByWing(_ context.Context, _ string, _ uuid.UUID, _ int) ([]*entities.Fingerprint, error) {
	return nil, nil
}
func (r *updateMockRepo) GetRecentFingerprintsByWingTx(_ context.Context, _ *sql.Tx, _ string, _ uuid.UUID, _ int) ([]*entities.Fingerprint, error) {
	return nil, nil
}
func (r *updateMockRepo) GetEmbeddingByID(_ context.Context, _ uuid.UUID) (*entities.Embedding, error) {
	return nil, nil
}
func (r *updateMockRepo) AddNode(_ context.Context, _ *entities.CausalNode) error { return nil }
func (r *updateMockRepo) AddNodeTx(_ context.Context, _ *sql.Tx, _ *entities.CausalNode) error {
	return nil
}
func (r *updateMockRepo) AddEdge(_ context.Context, _ *entities.CausalEdge) error { return nil }
func (r *updateMockRepo) AddEdgeTx(_ context.Context, _ *sql.Tx, _ *entities.CausalEdge) error {
	return nil
}
func (r *updateMockRepo) HasEdge(_ context.Context, _, _ uuid.UUID) bool { return false }
func (r *updateMockRepo) GetChain(_ context.Context, _ uuid.UUID, _ int) ([]*entities.CausalNode, error) {
	return nil, nil
}
func (r *updateMockRepo) GetConsequences(_ context.Context, _ uuid.UUID, _ int) ([]*entities.CausalNode, error) {
	return nil, nil
}
func (r *updateMockRepo) GetParents(_ context.Context, _ uuid.UUID, _ ...valueobjects.RelationType) ([]*entities.CausalNode, error) {
	return nil, nil
}
func (r *updateMockRepo) GetChildren(_ context.Context, _ uuid.UUID, _ ...valueobjects.RelationType) ([]*entities.CausalNode, error) {
	return nil, nil
}
func (r *updateMockRepo) RegisterModel(_ context.Context, _ *entities.EmbeddingModel) error {
	return nil
}
func (r *updateMockRepo) GetAllModels(_ context.Context) ([]string, error) { return nil, nil }
func (r *updateMockRepo) GetStats(_ context.Context) (*valueobjects.Stats, error) {
	return nil, nil
}
func (r *updateMockRepo) GetTimeline(_ context.Context, _ string, _ *string, _ *valueobjects.MemoryType, _, _ *string, _ int, _ *string) ([]*valueobjects.TimelineItem, error) {
	return nil, nil
}
func (r *updateMockRepo) ArchiveOldMemories(_ context.Context) (*valueobjects.ArchiveResult, error) {
	return nil, nil
}
func (r *updateMockRepo) ClearAll(_ context.Context) error { return nil }
func (r *updateMockRepo) ClearByRoom(_ context.Context, _ string, _ *string) (int, error) {
	return 0, nil
}
func (r *updateMockRepo) ClearByIDs(_ context.Context, _ []uuid.UUID) (int, error) {
	return 0, nil
}
func (r *updateMockRepo) StoreTags(_ context.Context, _ uuid.UUID, _ []string, _ string) error {
	return nil
}
func (r *updateMockRepo) GetVerbatimsByTags(_ context.Context, _ []string, _ int) ([]uuid.UUID, error) {
	return nil, nil
}
func (r *updateMockRepo) GetTagsForVerbatim(_ context.Context, _ uuid.UUID) ([]string, error) {
	return nil, nil
}

func (r *updateMockRepo) SaveAuditLog(_ context.Context, _ *entities.AuditLog) error {
	return nil
}

func (r *updateMockRepo) ListAuditLogs(_ context.Context, _, _ int) ([]*entities.AuditLog, error) {
	return nil, nil
}

func (r *updateMockRepo) GetPolicyByTokenHash(_ context.Context, _ string) (*entities.AccessPolicy, error) {
	return nil, nil
}

func (r *updateMockRepo) SavePolicy(_ context.Context, _ *entities.AccessPolicy) error {
	return nil
}

func (r *updateMockRepo) DeletePolicy(_ context.Context, _ string) error {
	return nil
}

func (r *updateMockRepo) ListPolicies(_ context.Context) ([]*entities.AccessPolicy, error) {
	return nil, nil
}

// Add missing methods to satisfy the full Repository interface

func (r *updateMockRepo) DB() *sql.DB { return testDB }

func (r *updateMockRepo) Close() error { return nil }

func (r *updateMockRepo) GetCandidatesWithEmbeddings(_ context.Context, _ []uuid.UUID, _, _ *string) ([]*entities.Candidate, error) {
	return nil, nil
}

func (r *updateMockRepo) GetAllEmbeddings(_ context.Context) ([]*entities.Embedding, error) {
	return nil, nil
}

func (r *updateMockRepo) SearchLexical(_ context.Context, _ string, _ int, _, _ *string) ([]*entities.Candidate, error) {
	return nil, nil
}

func (r *updateMockRepo) UpdateVerbatimSummary(_ context.Context, _ uuid.UUID, _ string, _ int) error {
	return nil
}

var _ ports.Repository = (*updateMockRepo)(nil)

// errUpdateExtractor makes ExtractPipeline fail.
type errUpdateExtractor struct{ *mockUpdateExtractor }

func (e *errUpdateExtractor) ExtractPipeline(_ context.Context, _ *entities.Verbatim, _ *valueobjects.MemoryType) (*entities.Fingerprint, *entities.Embedding, error) {
	return nil, nil, errors.New("extractor error")
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestUpdateMemory_GetVerbatimByIDFailure(t *testing.T) {
	repo := newUpdateMockRepo()
	repo.getVerbatimErr = errors.New("not found")
	uc := NewUpdateMemory(repo, &mockUpdateExtractor{}, &mockUpdateVectorStore{})
	_, err := uc.Execute(context.Background(), UpdateMemoryInput{
		ID:      uuid.New(),
		Content: "new content",
	})
	if err == nil {
		t.Fatal("expected error when verbatim is not found")
	}
}

func TestUpdateMemory_ExtractPipelineFailure(t *testing.T) {
	repo := newUpdateMockRepo()
	id := uuid.New()
	repo.verbatims[id] = &entities.Verbatim{ID: id, Content: "original", Wing: "w"}

	uc := NewUpdateMemory(repo, &errUpdateExtractor{&mockUpdateExtractor{}}, &mockUpdateVectorStore{})
	_, err := uc.Execute(context.Background(), UpdateMemoryInput{ID: id, Content: "new content"})
	if err == nil {
		t.Fatal("expected error when extraction fails")
	}
}

func TestUpdateMemory_BeginTxFailure(t *testing.T) {
	repo := newUpdateMockRepo()
	id := uuid.New()
	repo.verbatims[id] = &entities.Verbatim{ID: id, Content: "original", Wing: "w"}
	repo.beginErr = errors.New("begin failed")

	uc := NewUpdateMemory(repo, &mockUpdateExtractor{}, &mockUpdateVectorStore{})
	_, err := uc.Execute(context.Background(), UpdateMemoryInput{ID: id, Content: "new content"})
	if err == nil {
		t.Fatal("expected error when Begin fails")
	}
}

func TestUpdateMemory_DeleteVerbatimTxFailure(t *testing.T) {
	repo := newUpdateMockRepo()
	id := uuid.New()
	repo.verbatims[id] = &entities.Verbatim{ID: id, Content: "original", Wing: "w"}
	repo.deleteVerbatimErr = errors.New("delete failed")

	uc := NewUpdateMemory(repo, &mockUpdateExtractor{}, &mockUpdateVectorStore{})
	_, err := uc.Execute(context.Background(), UpdateMemoryInput{ID: id, Content: "new content"})
	if err == nil {
		t.Fatal("expected error when DeleteVerbatimByIDTx fails")
	}
}

func TestUpdateMemory_StoreFingerprintTxFailure(t *testing.T) {
	repo := newUpdateMockRepo()
	id := uuid.New()
	repo.verbatims[id] = &entities.Verbatim{ID: id, Content: "original", Wing: "w"}
	repo.storeFpErr = errors.New("fp store failed")

	uc := NewUpdateMemory(repo, &mockUpdateExtractor{}, &mockUpdateVectorStore{})
	_, err := uc.Execute(context.Background(), UpdateMemoryInput{ID: id, Content: "new content"})
	if err == nil {
		t.Fatal("expected error when StoreFingerprintTx fails")
	}
}

func TestUpdateMemory_StoreEmbeddingTxFailure(t *testing.T) {
	repo := newUpdateMockRepo()
	id := uuid.New()
	repo.verbatims[id] = &entities.Verbatim{ID: id, Content: "original", Wing: "w"}
	repo.storeEmbErr = errors.New("emb store failed")

	uc := NewUpdateMemory(repo, &mockUpdateExtractor{}, &mockUpdateVectorStore{})
	_, err := uc.Execute(context.Background(), UpdateMemoryInput{ID: id, Content: "new content"})
	if err == nil {
		t.Fatal("expected error when StoreEmbeddingTx fails")
	}
}

func TestUpdateMemory_CommitFailure(t *testing.T) {
	repo := newUpdateMockRepo()
	id := uuid.New()
	repo.verbatims[id] = &entities.Verbatim{ID: id, Content: "original", Wing: "w"}
	repo.beginRolledBack = true // tx is pre-rolled-back → Commit will fail

	uc := NewUpdateMemory(repo, &mockUpdateExtractor{}, &mockUpdateVectorStore{})
	_, err := uc.Execute(context.Background(), UpdateMemoryInput{ID: id, Content: "new content"})
	if err == nil {
		t.Fatal("expected error when Commit fails")
	}
}

func TestUpdateMemory_NilTxFallback(t *testing.T) {
	repo := newUpdateMockRepo()
	id := uuid.New()
	repo.verbatims[id] = &entities.Verbatim{ID: id, Content: "original", Wing: "w"}
	repo.beginReturnsNil = true // triggers the else branch in Execute

	uc := NewUpdateMemory(repo, &mockUpdateExtractor{}, &mockUpdateVectorStore{})
	out, err := uc.Execute(context.Background(), UpdateMemoryInput{ID: id, Content: "updated via fallback"})
	if err != nil {
		t.Fatalf("nil-tx fallback should succeed, got %v", err)
	}
	if out == nil || out.Verbatim == nil {
		t.Fatal("expected non-nil output")
	}
	if out.Verbatim.Content != "updated via fallback" {
		t.Errorf("content not updated: got %q", out.Verbatim.Content)
	}
}
