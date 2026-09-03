package interactors

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
	_ "github.com/benoitpetit/go-sqlcipher/v4"
)

// ── defaultRoomForType ────────────────────────────────────────────────────────

func TestDefaultRoomForType_AllCases(t *testing.T) {
	cases := []struct {
		memType valueobjects.MemoryType
		want    string
		wantNil bool
	}{
		{valueobjects.TypeDecision, "decisions", false},
		{valueobjects.TypeFact, "facts", false},
		{valueobjects.TypePreference, "preferences", false},
		{valueobjects.TypeSessionNote, "session", false},
		{valueobjects.TypeDebugLog, "debug", false},
		{valueobjects.MemoryType("unknown"), "", true},
	}
	for _, tc := range cases {
		t.Run(string(tc.memType), func(t *testing.T) {
			got := defaultRoomForType(tc.memType)
			if tc.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %q", *got)
				}
			} else {
				if got == nil {
					t.Fatalf("expected %q, got nil", tc.want)
				}
				if *got != tc.want {
					t.Errorf("expected %q, got %q", tc.want, *got)
				}
			}
		})
	}
}

// ── Validate extra cases ──────────────────────────────────────────────────────

func TestStoreMemoryValidate_ContentTooLong(t *testing.T) {
	in := StoreMemoryInput{Content: strings.Repeat("x", 65537), Wing: "w"}
	if err := in.Validate(); err == nil {
		t.Error("expected error for content > 65536 chars")
	}
}

func TestStoreMemoryValidate_RoomTooLong(t *testing.T) {
	room := strings.Repeat("r", 101)
	in := StoreMemoryInput{Content: "hi", Wing: "w", Room: &room}
	if err := in.Validate(); err == nil {
		t.Error("expected error for room > 100 chars")
	}
}

func TestStoreMemoryValidate_MetricsTooLarge(t *testing.T) {
	big := map[string]any{"key": strings.Repeat("v", 10001)}
	in := StoreMemoryInput{Content: "hi", Wing: "w", Metrics: big}
	if err := in.Validate(); err == nil {
		t.Error("expected error for metrics serialized size > 10000 bytes")
	}
}

// ── Error-injection helpers ───────────────────────────────────────────────────

type storeErrorExtractor struct{ *mockStoreExtractor }

func (e *storeErrorExtractor) ExtractPipeline(_ context.Context, _ *entities.Verbatim, _ *valueobjects.MemoryType) (*entities.Fingerprint, *entities.Embedding, error) {
	return nil, nil, errors.New("extraction error")
}

type failBeginStoreRepo struct{ *mockStoreRepository }

func (r *failBeginStoreRepo) Begin() (*sql.Tx, error) { return nil, errors.New("begin error") }

type failStoreVerbatimTxRepo struct{ *mockStoreRepository }

func (r *failStoreVerbatimTxRepo) StoreVerbatimTx(_ context.Context, _ *sql.Tx, _ *entities.Verbatim) error {
	return errors.New("verbatim tx error")
}

type failStoreFingerprintTxRepo struct{ *mockStoreRepository }

func (r *failStoreFingerprintTxRepo) StoreFingerprintTx(_ context.Context, _ *sql.Tx, _ *entities.Fingerprint) error {
	return errors.New("fingerprint tx error")
}

type failStoreEmbeddingTxRepo struct{ *mockStoreRepository }

func (r *failStoreEmbeddingTxRepo) StoreEmbeddingTx(_ context.Context, _ *sql.Tx, _ *entities.Embedding) error {
	return errors.New("embedding tx error")
}

// preRolledBackTxRepo returns a pre-rolled-back transaction so that Commit fails.
type preRolledBackTxRepo struct{ *mockStoreRepository }

func (r *preRolledBackTxRepo) Begin() (*sql.Tx, error) {
	db, _ := sql.Open("sqlite3", ":memory:")
	tx, _ := db.Begin()
	_ = tx.Rollback()
	return tx, nil
}

type failAddCandidateStore struct{ *mockStoreVectorStore }

func (s *failAddCandidateStore) AddCandidate(_ context.Context, _ *entities.Candidate) error {
	return errors.New("add candidate error")
}

type failAddNodeStoreRepo struct{ *mockStoreRepository }

func (r *failAddNodeStoreRepo) AddNode(_ context.Context, _ *entities.CausalNode) error {
	return errors.New("add node error")
}

type recentFingerprintsRepo struct {
	*mockStoreRepository
	fps []*entities.Fingerprint
}

func (r *recentFingerprintsRepo) GetRecentFingerprintsByWing(_ context.Context, _ string, _ uuid.UUID, _ int) ([]*entities.Fingerprint, error) {
	return r.fps, nil
}

type failGetRecentFpsRepo struct{ *mockStoreRepository }

func (r *failGetRecentFpsRepo) GetRecentFingerprintsByWing(_ context.Context, _ string, _ uuid.UUID, _ int) ([]*entities.Fingerprint, error) {
	return nil, errors.New("db error")
}

type fixedCausalDetector struct {
	edges []*entities.CausalEdge
}

func (d *fixedCausalDetector) DetectCausalRelations(_ context.Context, _ *entities.Fingerprint, _ []*entities.Fingerprint, _ string) ([]*entities.CausalEdge, error) {
	return d.edges, nil
}

type errCausalDetector struct{}

func (d *errCausalDetector) DetectCausalRelations(_ context.Context, _ *entities.Fingerprint, _ []*entities.Fingerprint, _ string) ([]*entities.CausalEdge, error) {
	return nil, errors.New("causal detection failed")
}

// exactSearchStore implements the optional SearchExact interface.
type exactSearchStore struct {
	*mockStoreVectorStore
	result *entities.Candidate
}

func (s *exactSearchStore) SearchExact(_ context.Context, _ string, _ int, _, _ *string) ([]*entities.Candidate, error) {
	if s.result != nil {
		return []*entities.Candidate{s.result}, nil
	}
	return nil, nil
}

// storeMetricsSpy counts calls to RecordStore / RecordStoreResult.
type storeMetricsSpy struct {
	storeCalls  int
	resultCalls int
}

func (m *storeMetricsSpy) IsEnabled() bool                      { return true }
func (m *storeMetricsSpy) RecordStore(_ time.Duration)          { m.storeCalls++ }
func (m *storeMetricsSpy) RecordRecall(_ time.Duration)         {}
func (m *storeMetricsSpy) RecordSearch(_ time.Duration, _ bool) {}
func (m *storeMetricsSpy) RecordEmbed(_ time.Duration)          {}
func (m *storeMetricsSpy) RecordError()                         {}
func (m *storeMetricsSpy) RecordStoreResult(_ int)              { m.resultCalls++ }
func (m *storeMetricsSpy) RecordRecallResult(_ int, _ float64)  {}
func (m *storeMetricsSpy) UpdateMemoryCount(_ int)              {}
func (m *storeMetricsSpy) UpdateVectorCount(_ int)              {}
func (m *storeMetricsSpy) GetReport(_ context.Context) ports.MetricsReport {
	return ports.MetricsReport{}
}

var _ ports.MetricsCollector = (*storeMetricsSpy)(nil)

// ── Execute error-path tests ──────────────────────────────────────────────────

func TestStoreMemory_ValidationFailure(t *testing.T) {
	uc := NewStoreMemory(newMockStoreRepository(), &mockStoreExtractor{}, nil, &mockStoreVectorStore{}, nil, nil)
	_, err := uc.Execute(context.Background(), StoreMemoryInput{Content: "", Wing: "w"})
	if err == nil {
		t.Error("expected validation error")
	}
}

func TestStoreMemory_ExtractionFailure(t *testing.T) {
	uc := NewStoreMemory(newMockStoreRepository(), &storeErrorExtractor{&mockStoreExtractor{}}, nil, &mockStoreVectorStore{}, nil, nil)
	_, err := uc.Execute(context.Background(), StoreMemoryInput{Content: "hello world", Wing: "w"})
	if err == nil || !strings.Contains(err.Error(), "extraction failed") {
		t.Errorf("expected extraction error, got %v", err)
	}
}

func TestStoreMemory_BeginTxFailure(t *testing.T) {
	uc := NewStoreMemory(&failBeginStoreRepo{newMockStoreRepository()}, &mockStoreExtractor{}, nil, &mockStoreVectorStore{}, nil, nil)
	_, err := uc.Execute(context.Background(), StoreMemoryInput{Content: "hello world", Wing: "w"})
	if err == nil || !strings.Contains(err.Error(), "failed to begin transaction") {
		t.Errorf("expected begin tx error, got %v", err)
	}
}

func TestStoreMemory_StoreVerbatimTxFailure(t *testing.T) {
	uc := NewStoreMemory(&failStoreVerbatimTxRepo{newMockStoreRepository()}, &mockStoreExtractor{}, nil, &mockStoreVectorStore{}, nil, nil)
	_, err := uc.Execute(context.Background(), StoreMemoryInput{Content: "hello world", Wing: "w"})
	if err == nil || !strings.Contains(err.Error(), "failed to store verbatim") {
		t.Errorf("expected verbatim store error, got %v", err)
	}
}

func TestStoreMemory_StoreFingerprintTxFailure(t *testing.T) {
	uc := NewStoreMemory(&failStoreFingerprintTxRepo{newMockStoreRepository()}, &mockStoreExtractor{}, nil, &mockStoreVectorStore{}, nil, nil)
	_, err := uc.Execute(context.Background(), StoreMemoryInput{Content: "hello world", Wing: "w"})
	if err == nil || !strings.Contains(err.Error(), "failed to store fingerprint") {
		t.Errorf("expected fingerprint store error, got %v", err)
	}
}

func TestStoreMemory_StoreEmbeddingTxFailure(t *testing.T) {
	uc := NewStoreMemory(&failStoreEmbeddingTxRepo{newMockStoreRepository()}, &mockStoreExtractor{}, nil, &mockStoreVectorStore{}, nil, nil)
	_, err := uc.Execute(context.Background(), StoreMemoryInput{Content: "hello world", Wing: "w"})
	if err == nil || !strings.Contains(err.Error(), "failed to store embedding") {
		t.Errorf("expected embedding store error, got %v", err)
	}
}

func TestStoreMemory_CommitFailure(t *testing.T) {
	uc := NewStoreMemory(&preRolledBackTxRepo{newMockStoreRepository()}, &mockStoreExtractor{}, nil, &mockStoreVectorStore{}, nil, nil)
	_, err := uc.Execute(context.Background(), StoreMemoryInput{Content: "hello world", Wing: "w"})
	if err == nil || !strings.Contains(err.Error(), "failed to commit transaction") {
		t.Errorf("expected commit error, got %v", err)
	}
}

func TestStoreMemory_AddCandidateFailure_NonFatal(t *testing.T) {
	vs := &failAddCandidateStore{&mockStoreVectorStore{}}
	uc := NewStoreMemory(newMockStoreRepository(), &mockStoreExtractor{}, nil, vs, nil, &mockLogger{})
	out, err := uc.Execute(context.Background(), StoreMemoryInput{Content: "hello world test", Wing: "w"})
	if err != nil {
		t.Errorf("AddCandidate failure must be non-fatal, got %v", err)
	}
	if out == nil {
		t.Error("expected non-nil output")
	}
}

func TestStoreMemory_AddNodeFailure_NonFatal(t *testing.T) {
	uc := NewStoreMemory(&failAddNodeStoreRepo{newMockStoreRepository()}, &mockStoreExtractor{}, nil, &mockStoreVectorStore{}, nil, &mockLogger{})
	out, err := uc.Execute(context.Background(), StoreMemoryInput{Content: "hello world test", Wing: "w"})
	if err != nil {
		t.Errorf("AddNode failure must be non-fatal, got %v", err)
	}
	if out == nil {
		t.Error("expected non-nil output")
	}
}

func TestStoreMemory_DefaultRoomAssigned(t *testing.T) {
	repo := newMockStoreRepository()
	uc := NewStoreMemory(repo, &mockStoreExtractor{}, nil, &mockStoreVectorStore{}, nil, nil)
	memType := valueobjects.TypeDecision
	_, err := uc.Execute(context.Background(), StoreMemoryInput{
		Content: "a decision was made here",
		Wing:    "w",
		Type:    &memType,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// mockStoreExtractor uses forcedType, so fp.Type = TypeDecision
	// → defaultRoomForType returns "decisions" and sets verbatim.Room
	for _, v := range repo.verbatims {
		if v.Room == nil {
			t.Error("expected Room to be assigned via defaultRoomForType")
		} else if *v.Room != "decisions" {
			t.Errorf("expected room=decisions, got %q", *v.Room)
		}
	}
}

func TestStoreMemory_ExactDedup(t *testing.T) {
	uid := uuid.New()
	existingFp := &entities.Fingerprint{
		ID:        uid,
		Type:      valueobjects.TypeFact,
		FactCount: 2,
		ModelHash: "model-x",
	}
	existingVerbatim := &entities.Verbatim{ID: uid, TokenCount: 42}
	candidate := entities.NewCandidate(existingFp, existingVerbatim, make([]float32, 4))

	vs := &exactSearchStore{mockStoreVectorStore: &mockStoreVectorStore{}, result: candidate}
	uc := NewStoreMemory(newMockStoreRepository(), &mockStoreExtractor{}, nil, vs, nil, nil)
	out, err := uc.Execute(context.Background(), StoreMemoryInput{Content: "duplicate content", Wing: "w"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.FingerprintID != uid.String() {
		t.Errorf("expected dedup FingerprintID %s, got %s", uid, out.FingerprintID)
	}
	if out.Type != string(valueobjects.TypeFact) {
		t.Errorf("expected type fact, got %s", out.Type)
	}
	if out.TokenCount != 42 {
		t.Errorf("expected TokenCount=42, got %d", out.TokenCount)
	}
}

func TestStoreMemory_MetricsCollected(t *testing.T) {
	mc := &storeMetricsSpy{}
	uc := NewStoreMemory(newMockStoreRepository(), &mockStoreExtractor{}, nil, &mockStoreVectorStore{}, mc, nil)
	_, err := uc.Execute(context.Background(), StoreMemoryInput{Content: "hello world test", Wing: "w"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mc.storeCalls != 1 {
		t.Errorf("expected 1 RecordStore call, got %d", mc.storeCalls)
	}
	if mc.resultCalls != 1 {
		t.Errorf("expected 1 RecordStoreResult call, got %d", mc.resultCalls)
	}
}

func TestStoreMemory_CausalEdgesDetected(t *testing.T) {
	from, to := uuid.New(), uuid.New()
	edge := &entities.CausalEdge{
		FromID:   from,
		ToID:     to,
		Relation: valueobjects.RelBecause,
	}
	recentFp := entities.NewFingerprint(uuid.New(), valueobjects.TypeFact, "test-model")
	recentFp.WithData(valueobjects.FingerprintData{Subject: []string{"related topic"}})

	repo := &recentFingerprintsRepo{
		mockStoreRepository: newMockStoreRepository(),
		fps:                 []*entities.Fingerprint{recentFp},
	}
	uc := NewStoreMemory(repo, &mockStoreExtractor{}, &fixedCausalDetector{edges: []*entities.CausalEdge{edge}}, &mockStoreVectorStore{}, nil, &mockLogger{})
	_, err := uc.Execute(context.Background(), StoreMemoryInput{Content: "something causes another thing", Wing: "w"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.edges) != 1 {
		t.Errorf("expected 1 causal edge stored, got %d", len(repo.edges))
	}
}

func TestStoreMemory_CausalDetectionError_NonFatal(t *testing.T) {
	recentFp := entities.NewFingerprint(uuid.New(), valueobjects.TypeFact, "test-model")
	recentFp.WithData(valueobjects.FingerprintData{Subject: []string{"related"}})

	repo := &recentFingerprintsRepo{
		mockStoreRepository: newMockStoreRepository(),
		fps:                 []*entities.Fingerprint{recentFp},
	}
	uc := NewStoreMemory(repo, &mockStoreExtractor{}, &errCausalDetector{}, &mockStoreVectorStore{}, nil, &mockLogger{})
	out, err := uc.Execute(context.Background(), StoreMemoryInput{Content: "causes something", Wing: "w"})
	if err != nil {
		t.Errorf("causal detection error must be non-fatal, got %v", err)
	}
	if out == nil {
		t.Error("expected non-nil output")
	}
}

func TestStoreMemory_GetRecentFpsError_NonFatal(t *testing.T) {
	uc := NewStoreMemory(&failGetRecentFpsRepo{newMockStoreRepository()}, &mockStoreExtractor{}, &fixedCausalDetector{}, &mockStoreVectorStore{}, nil, &mockLogger{})
	out, err := uc.Execute(context.Background(), StoreMemoryInput{Content: "triggers recent fps error", Wing: "w"})
	if err != nil {
		t.Errorf("GetRecentFps error must be non-fatal, got %v", err)
	}
	if out == nil {
		t.Error("expected non-nil output")
	}
}
