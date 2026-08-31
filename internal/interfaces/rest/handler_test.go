package rest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/interfaces/rest"
	"github.com/benoitpetit/mira/internal/usecases/interactors"
	"github.com/google/uuid"
)

// ── Fake executors ────────────────────────────────────────────────────────────

type fakeStore struct {
	out *interactors.StoreMemoryOutput
	err error
}

func (f *fakeStore) Execute(_ context.Context, _ interactors.StoreMemoryInput) (*interactors.StoreMemoryOutput, error) {
	return f.out, f.err
}

type fakeRecall struct {
	out *interactors.RecallMemoryOutput
	err error
}

func (f *fakeRecall) Execute(_ context.Context, _ interactors.RecallMemoryInput) (*interactors.RecallMemoryOutput, error) {
	return f.out, f.err
}

type fakeLoad struct {
	out *interactors.LoadMemoryOutput
	err error
}

func (f *fakeLoad) Execute(_ context.Context, _ interactors.LoadMemoryInput) (*interactors.LoadMemoryOutput, error) {
	return f.out, f.err
}

type fakeUpdate struct {
	out *interactors.UpdateMemoryOutput
	err error
}

func (f *fakeUpdate) Execute(_ context.Context, _ interactors.UpdateMemoryInput) (*interactors.UpdateMemoryOutput, error) {
	return f.out, f.err
}

type fakeDelete struct{ err error }

func (f *fakeDelete) Execute(_ context.Context, _ interactors.DeleteMemoryInput) error {
	return f.err
}

type fakeSearch struct {
	out []*interactors.SearchSemanticResult
	err error
}

func (f *fakeSearch) Execute(_ context.Context, _ interactors.SearchSemanticInput) ([]*interactors.SearchSemanticResult, error) {
	return f.out, f.err
}

type fakeConsolidate struct {
	out *interactors.ConsolidateMemoriesOutput
	err error
}

func (f *fakeConsolidate) Execute(_ context.Context, _ interactors.ConsolidateMemoriesInput) (*interactors.ConsolidateMemoriesOutput, error) {
	return f.out, f.err
}

type fakeClear struct {
	out *interactors.ClearMemoryOutput
	err error
}

func (f *fakeClear) Execute(_ context.Context, _ interactors.ClearMemoryInput) (*interactors.ClearMemoryOutput, error) {
	return f.out, f.err
}

type fakeTimeline struct {
	out *interactors.GetTimelineOutput
	err error
}

func (f *fakeTimeline) Execute(_ context.Context, _ interactors.GetTimelineInput) (*interactors.GetTimelineOutput, error) {
	return f.out, f.err
}

type fakeArchive struct {
	out *interactors.ArchiveMemoriesOutput
	err error
}

func (f *fakeArchive) Execute(_ context.Context) (*interactors.ArchiveMemoriesOutput, error) {
	return f.out, f.err
}

type fakeCausal struct {
	out *interactors.GetCausalChainOutput
	err error
}

func (f *fakeCausal) Execute(_ context.Context, _ interactors.GetCausalChainInput) (*interactors.GetCausalChainOutput, error) {
	return f.out, f.err
}

type fakeStatus struct {
	out *interactors.GetStatusOutput
	err error
}

func (f *fakeStatus) Execute(_ context.Context) (*interactors.GetStatusOutput, error) {
	return f.out, f.err
}

type fakeAudit struct {
	logs []*entities.AuditLog
}

func (f *fakeAudit) SaveAuditLog(_ context.Context, log *entities.AuditLog) error {
	f.logs = append(f.logs, log)
	return nil
}

func (f *fakeAudit) ListAuditLogs(_ context.Context, _, _ int) ([]*entities.AuditLog, error) {
	return f.logs, nil
}

func (f *fakeAudit) GetPolicyByTokenHash(_ context.Context, _ string) (*entities.AccessPolicy, error) {
	return nil, errors.New("not found")
}

func (f *fakeAudit) SavePolicy(_ context.Context, _ *entities.AccessPolicy) error {
	return nil
}

func (f *fakeAudit) DeletePolicy(_ context.Context, _ string) error {
	return nil
}

func (f *fakeAudit) ListPolicies(_ context.Context) ([]*entities.AccessPolicy, error) {
	return nil, nil
}

// ── builder ───────────────────────────────────────────────────────────────────

type suite struct {
	store       *fakeStore
	recall      *fakeRecall
	load        *fakeLoad
	update      *fakeUpdate
	del         *fakeDelete
	search      *fakeSearch
	consolidate *fakeConsolidate
	clear       *fakeClear
	timeline    *fakeTimeline
	archive     *fakeArchive
	causal      *fakeCausal
	status      *fakeStatus
	audit       *fakeAudit
	server      *httptest.Server
}

func newSuite(t *testing.T) *suite {
	t.Helper()
	s := &suite{
		store:       &fakeStore{},
		recall:      &fakeRecall{},
		load:        &fakeLoad{},
		update:      &fakeUpdate{},
		del:         &fakeDelete{},
		search:      &fakeSearch{},
		consolidate: &fakeConsolidate{},
		clear:       &fakeClear{},
		timeline:    &fakeTimeline{},
		archive:     &fakeArchive{},
		causal:      &fakeCausal{},
		status:      &fakeStatus{},
		audit:       &fakeAudit{},
	}
	h := rest.NewHandler(
		s.store, s.recall, s.load, s.update, s.del,
		s.search, s.consolidate, s.clear, s.timeline,
		s.archive, s.causal, s.status, s.audit, nil,
	)
	srv := rest.NewServer(h, ":0", "", nil, 5*time.Second, 5*time.Second)
	s.server = httptest.NewServer(srv.Handler)
	t.Cleanup(s.server.Close)
	return s
}

func (s *suite) post(path string, body any) *http.Response {
	b, _ := json.Marshal(body)
	resp, err := http.Post(s.server.URL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		panic(err)
	}
	return resp
}

func (s *suite) get(path string) *http.Response {
	resp, err := http.Get(s.server.URL + path)
	if err != nil {
		panic(err)
	}
	return resp
}

func (s *suite) do(method, path string, body any) *http.Response {
	var buf *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	} else {
		buf = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, s.server.URL+path, buf)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestHandleStore_Success(t *testing.T) {
	s := newSuite(t)
	s.store.out = &interactors.StoreMemoryOutput{FingerprintID: "fp-1", Type: "fact", FactCount: 2, TokenCount: 10}

	resp := s.post("/api/v1/memories", map[string]any{
		"content": "Hello world",
		"wing":    "test",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("want 201, got %d", resp.StatusCode)
	}
	var out interactors.StoreMemoryOutput
	decodeJSON(t, resp, &out)
	if out.FingerprintID != "fp-1" {
		t.Errorf("want fp-1, got %s", out.FingerprintID)
	}
}

func TestHandleStore_MissingContent(t *testing.T) {
	s := newSuite(t)
	resp := s.post("/api/v1/memories", map[string]any{"wing": "test"})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", resp.StatusCode)
	}
}

func TestHandleStore_BadJSON(t *testing.T) {
	s := newSuite(t)
	req, _ := http.NewRequest(http.MethodPost, s.server.URL+"/api/v1/memories", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestHandleLoad_Success(t *testing.T) {
	s := newSuite(t)
	id := uuid.New()
	v := &entities.Verbatim{ID: id, Content: "test", Wing: "w", CreatedAt: time.Now()}
	s.load.out = &interactors.LoadMemoryOutput{Verbatim: v}

	resp := s.get("/api/v1/memories/" + id.String())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestHandleLoad_InvalidUUID(t *testing.T) {
	s := newSuite(t)
	resp := s.get("/api/v1/memories/not-a-uuid")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestHandleUpdate_Success(t *testing.T) {
	s := newSuite(t)
	id := uuid.New()
	v := &entities.Verbatim{ID: id, Content: "updated", Wing: "w", CreatedAt: time.Now()}
	s.update.out = &interactors.UpdateMemoryOutput{Verbatim: v}

	resp := s.do(http.MethodPut, "/api/v1/memories/"+id.String(), map[string]any{"content": "updated"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestHandleUpdate_EmptyContent(t *testing.T) {
	s := newSuite(t)
	id := uuid.New()
	resp := s.do(http.MethodPut, "/api/v1/memories/"+id.String(), map[string]any{"content": ""})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", resp.StatusCode)
	}
}

func TestHandleDelete_Success(t *testing.T) {
	s := newSuite(t)
	id := uuid.New()
	resp := s.do(http.MethodDelete, "/api/v1/memories/"+id.String(), nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("want 204, got %d", resp.StatusCode)
	}
}

func TestHandleRecall_Success(t *testing.T) {
	s := newSuite(t)
	s.recall.out = &interactors.RecallMemoryOutput{TotalTokens: 42, BudgetUsed: 0.5}

	resp := s.post("/api/v1/memories/recall", map[string]any{
		"query":  "something",
		"budget": 1000,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestHandleRecall_MissingQuery(t *testing.T) {
	s := newSuite(t)
	resp := s.post("/api/v1/memories/recall", map[string]any{"budget": 1000})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", resp.StatusCode)
	}
}

func TestHandleSearch_Success(t *testing.T) {
	s := newSuite(t)
	s.search.out = []*interactors.SearchSemanticResult{
		{ID: uuid.New(), Content: "match", Similarity: 0.9},
	}

	resp := s.post("/api/v1/memories/search", map[string]any{"query": "find this", "top_k": 5})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]any
	decodeJSON(t, resp, &body)
	results, ok := body["results"].([]any)
	if !ok || len(results) != 1 {
		t.Errorf("expected 1 result, got %v", body["results"])
	}
}

func TestHandleSearch_EmptyResults(t *testing.T) {
	s := newSuite(t)
	s.search.out = nil // no results

	resp := s.post("/api/v1/memories/search", map[string]any{"query": "nothing"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]any
	decodeJSON(t, resp, &body)
	results, ok := body["results"].([]any)
	if !ok || len(results) != 0 {
		t.Errorf("expected empty results, got %v", body["results"])
	}
}

func TestHandleConsolidate_Success(t *testing.T) {
	s := newSuite(t)
	s.consolidate.out = &interactors.ConsolidateMemoriesOutput{ConsolidatedCount: 3, RemovedCount: 2}

	resp := s.post("/api/v1/memories/consolidate", map[string]any{"wing": "test"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestHandleConsolidate_MissingWing(t *testing.T) {
	s := newSuite(t)
	resp := s.post("/api/v1/memories/consolidate", map[string]any{})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", resp.StatusCode)
	}
}

func TestHandleClear_Success(t *testing.T) {
	s := newSuite(t)
	s.clear.out = &interactors.ClearMemoryOutput{DeletedCount: 5, Mode: "all"}

	resp := s.do(http.MethodDelete, "/api/v1/memories", map[string]any{"mode": "all"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestHandleTimeline_Success(t *testing.T) {
	s := newSuite(t)
	s.timeline.out = &interactors.GetTimelineOutput{
		Items: []*valueobjects.TimelineItem{
			{ID: "1", Timestamp: "2024-01-01", Type: "fact", Summary: "thing"},
		},
	}

	resp := s.get("/api/v1/timeline?wing=test&limit=10")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestHandleArchive_Success(t *testing.T) {
	s := newSuite(t)
	s.archive.out = &interactors.ArchiveMemoriesOutput{
		Result: &valueobjects.ArchiveResult{SessionNotes: 2, DebugLogs: 1, TokensFreed: 500},
	}

	resp := s.post("/api/v1/archive", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestHandleCausal_Success(t *testing.T) {
	s := newSuite(t)
	s.causal.out = &interactors.GetCausalChainOutput{}
	id := uuid.New()

	resp := s.get("/api/v1/causal/" + id.String())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestHandleCausal_InvalidUUID(t *testing.T) {
	s := newSuite(t)
	resp := s.get("/api/v1/causal/bad-id")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestHandleStatus_Success(t *testing.T) {
	s := newSuite(t)
	s.status.out = &interactors.GetStatusOutput{
		Stats:   valueobjects.NewStats(),
		Models:  []string{"model-v1"},
		Version: "0.4.7",
		Uptime:  "1h",
	}

	resp := s.get("/api/v1/status")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestServeSpec(t *testing.T) {
	s := newSuite(t)
	resp := s.get("/openapi.json")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var doc map[string]any
	decodeJSON(t, resp, &doc)
	if doc["openapi"] != "3.1.0" {
		t.Errorf("want openapi=3.1.0, got %v", doc["openapi"])
	}
}

// ── Auth middleware ───────────────────────────────────────────────────────────

func newSuiteWithAuth(t *testing.T, token string) *suite {
	t.Helper()
	s := &suite{
		store:       &fakeStore{out: &interactors.StoreMemoryOutput{FingerprintID: "fp-auth"}},
		recall:      &fakeRecall{},
		load:        &fakeLoad{},
		update:      &fakeUpdate{},
		del:         &fakeDelete{},
		search:      &fakeSearch{},
		consolidate: &fakeConsolidate{},
		clear:       &fakeClear{},
		timeline:    &fakeTimeline{},
		archive:     &fakeArchive{},
		causal:      &fakeCausal{},
		status:      &fakeStatus{},
		audit:       &fakeAudit{},
	}
	h := rest.NewHandler(
		s.store, s.recall, s.load, s.update, s.del,
		s.search, s.consolidate, s.clear, s.timeline,
		s.archive, s.causal, s.status, s.audit, nil,
	)
	srv := rest.NewServer(h, ":0", token, nil, 5*time.Second, 5*time.Second)
	s.server = httptest.NewServer(srv.Handler)
	t.Cleanup(s.server.Close)
	return s
}

func TestAuth_NoToken_Rejected(t *testing.T) {
	s := newSuiteWithAuth(t, "secret-token")
	resp := s.get("/api/v1/status")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestAuth_WrongToken_Rejected(t *testing.T) {
	s := newSuiteWithAuth(t, "secret-token")
	req, _ := http.NewRequest(http.MethodGet, s.server.URL+"/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestAuth_CorrectToken_Allowed(t *testing.T) {
	s := newSuiteWithAuth(t, "secret-token")
	s.status.out = &interactors.GetStatusOutput{Stats: valueobjects.NewStats()}
	req, _ := http.NewRequest(http.MethodGet, s.server.URL+"/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestAuth_OpenAPISpec_NoToken_Allowed(t *testing.T) {
	s := newSuiteWithAuth(t, "secret-token")
	// /openapi.json must be accessible without auth
	resp := s.get("/openapi.json")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

// ── Wing-scoped token tests ───────────────────────────────────────────────────

func newSuiteWithWings(t *testing.T, wingTokens map[string][]string) *suite {
	t.Helper()
	s := &suite{
		store:       &fakeStore{out: &interactors.StoreMemoryOutput{FingerprintID: "fp-wing"}},
		recall:      &fakeRecall{},
		load:        &fakeLoad{},
		update:      &fakeUpdate{},
		del:         &fakeDelete{},
		search:      &fakeSearch{},
		consolidate: &fakeConsolidate{},
		clear:       &fakeClear{},
		timeline:    &fakeTimeline{},
		archive:     &fakeArchive{},
		causal:      &fakeCausal{},
		status:      &fakeStatus{},
		audit:       &fakeAudit{},
	}
	h := rest.NewHandler(
		s.store, s.recall, s.load, s.update, s.del,
		s.search, s.consolidate, s.clear, s.timeline,
		s.archive, s.causal, s.status, s.audit, nil,
	)
	srv := rest.NewServer(h, ":0", "", wingTokens, 5*time.Second, 5*time.Second)
	s.server = httptest.NewServer(srv.Handler)
	t.Cleanup(s.server.Close)
	return s
}

func TestWingToken_ReadOnly_AllowsGET(t *testing.T) {
	s := newSuiteWithWings(t, map[string][]string{"ro-tok": {"read"}})
	s.status.out = &interactors.GetStatusOutput{Stats: valueobjects.NewStats()}
	req, _ := http.NewRequest(http.MethodGet, s.server.URL+"/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer ro-tok")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read wing: want 200 on GET, got %d", resp.StatusCode)
	}
}

func TestWingToken_ReadOnly_BlocksPOST(t *testing.T) {
	s := newSuiteWithWings(t, map[string][]string{"ro-tok": {"read"}})
	req, _ := http.NewRequest(http.MethodPost, s.server.URL+"/api/v1/memories",
		bytes.NewBufferString(`{"content":"x","wing":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer ro-tok")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("read wing: want 403 on POST store, got %d", resp.StatusCode)
	}
}

func TestWingToken_WriteAllowed(t *testing.T) {
	s := newSuiteWithWings(t, map[string][]string{"rw-tok": {"read", "write"}})
	b, _ := json.Marshal(map[string]string{"content": "hello", "wing": "test"})
	req, _ := http.NewRequest(http.MethodPost, s.server.URL+"/api/v1/memories", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer rw-tok")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("write wing: want 201 on POST store, got %d", resp.StatusCode)
	}
}

func TestWingToken_UnknownToken_Rejected(t *testing.T) {
	s := newSuiteWithWings(t, map[string][]string{"ro-tok": {"read"}})
	req, _ := http.NewRequest(http.MethodGet, s.server.URL+"/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer unknown")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401 for unknown token, got %d", resp.StatusCode)
	}
}

func TestWingToken_NoToken_Rejected(t *testing.T) {
	s := newSuiteWithWings(t, map[string][]string{"ro-tok": {"read"}})
	resp := s.get("/api/v1/status")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401 with no token, got %d", resp.StatusCode)
	}
}

func TestWingToken_OpenAPI_AlwaysPublic(t *testing.T) {
	s := newSuiteWithWings(t, map[string][]string{"ro-tok": {"read"}})
	resp := s.get("/openapi.json")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("openapi.json must be public even with wing tokens, got %d", resp.StatusCode)
	}
}

// ── Additional handler coverage ───────────────────────────────────────────────

func TestHandleDelete_BadID(t *testing.T) {
	s := newSuite(t)
	resp := s.do(http.MethodDelete, "/api/v1/memories/not-a-uuid", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestHandleDelete_NotFound(t *testing.T) {
	s := newSuite(t)
	// isNotFound requires a typed error — wrap with a custom type
	s.del.err = &notFoundErr{}
	resp := s.do(http.MethodDelete, "/api/v1/memories/00000000-0000-0000-0000-000000000001", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestHandleDelete_InternalError(t *testing.T) {
	s := newSuite(t)
	s.del.err = errors.New("db error")
	resp := s.do(http.MethodDelete, "/api/v1/memories/00000000-0000-0000-0000-000000000001", nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

func TestHandleArchive_Error(t *testing.T) {
	s := newSuite(t)
	s.archive.err = errors.New("archive failed")
	resp := s.do(http.MethodPost, "/api/v1/archive", nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

func TestHandleStatus_Error(t *testing.T) {
	s := newSuite(t)
	s.status.err = errors.New("status failed")
	resp := s.get("/api/v1/status")
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

// notFoundErr is a test helper that satisfies the isNotFound interface.
type notFoundErr struct{}

func (e *notFoundErr) Error() string    { return "not found" }
func (e *notFoundErr) IsNotFound() bool { return true }

// ── handleLoad error paths ────────────────────────────────────────────────────

func TestHandleLoad_NotFound(t *testing.T) {
	s := newSuite(t)
	s.load.err = &notFoundErr{}
	resp := s.get("/api/v1/memories/00000000-0000-0000-0000-000000000001")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestHandleLoad_InternalError(t *testing.T) {
	s := newSuite(t)
	s.load.err = errors.New("db error")
	resp := s.get("/api/v1/memories/00000000-0000-0000-0000-000000000001")
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

// ── handleUpdate error paths ──────────────────────────────────────────────────

func TestHandleUpdate_InvalidUUID(t *testing.T) {
	s := newSuite(t)
	resp := s.do(http.MethodPut, "/api/v1/memories/not-a-uuid", map[string]any{"content": "x"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestHandleUpdate_BadJSON(t *testing.T) {
	s := newSuite(t)
	id := uuid.New()
	req, _ := http.NewRequest(http.MethodPut, s.server.URL+"/api/v1/memories/"+id.String(), bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestHandleUpdate_NotFound(t *testing.T) {
	s := newSuite(t)
	s.update.err = &notFoundErr{}
	id := uuid.New()
	resp := s.do(http.MethodPut, "/api/v1/memories/"+id.String(), map[string]any{"content": "updated"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestHandleUpdate_InternalError(t *testing.T) {
	s := newSuite(t)
	s.update.err = errors.New("db error")
	id := uuid.New()
	resp := s.do(http.MethodPut, "/api/v1/memories/"+id.String(), map[string]any{"content": "updated"})
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

// ── handleRecall error paths ──────────────────────────────────────────────────

func TestHandleRecall_BadJSON(t *testing.T) {
	s := newSuite(t)
	req, _ := http.NewRequest(http.MethodPost, s.server.URL+"/api/v1/memories/recall", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestHandleRecall_InternalError(t *testing.T) {
	s := newSuite(t)
	s.recall.err = errors.New("recall failed")
	resp := s.post("/api/v1/memories/recall", map[string]any{"query": "test"})
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

// ── handleSearch error paths ──────────────────────────────────────────────────

func TestHandleSearch_BadJSON(t *testing.T) {
	s := newSuite(t)
	req, _ := http.NewRequest(http.MethodPost, s.server.URL+"/api/v1/memories/search", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestHandleSearch_MissingQuery(t *testing.T) {
	s := newSuite(t)
	resp := s.post("/api/v1/memories/search", map[string]any{"top_k": 5})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", resp.StatusCode)
	}
}

func TestHandleSearch_InternalError(t *testing.T) {
	s := newSuite(t)
	s.search.err = errors.New("search failed")
	resp := s.post("/api/v1/memories/search", map[string]any{"query": "test"})
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

// ── handleConsolidate error paths ─────────────────────────────────────────────

func TestHandleConsolidate_BadJSON(t *testing.T) {
	s := newSuite(t)
	req, _ := http.NewRequest(http.MethodPost, s.server.URL+"/api/v1/memories/consolidate", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestHandleConsolidate_InternalError(t *testing.T) {
	s := newSuite(t)
	s.consolidate.err = errors.New("consolidate failed")
	resp := s.post("/api/v1/memories/consolidate", map[string]any{"wing": "test"})
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

// ── handleClear error path ────────────────────────────────────────────────────

func TestHandleClear_InternalError(t *testing.T) {
	s := newSuite(t)
	s.clear.err = errors.New("clear failed")
	resp := s.do(http.MethodDelete, "/api/v1/memories", map[string]any{"mode": "all"})
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

func TestHandleClear_EmptyBody(t *testing.T) {
	s := newSuite(t)
	s.clear.out = &interactors.ClearMemoryOutput{DeletedCount: 0, Mode: "all"}
	resp := s.do(http.MethodDelete, "/api/v1/memories", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

// ── handleTimeline error path ─────────────────────────────────────────────────

func TestHandleTimeline_InternalError(t *testing.T) {
	s := newSuite(t)
	s.timeline.err = errors.New("timeline failed")
	resp := s.get("/api/v1/timeline?wing=test")
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

func TestHandleTimeline_AllParams(t *testing.T) {
	s := newSuite(t)
	s.timeline.out = &interactors.GetTimelineOutput{Items: []*valueobjects.TimelineItem{}}
	resp := s.get("/api/v1/timeline?wing=test&room=r1&type=fact&since=2024-01-01&until=2024-12-31&limit=10&cursor=abc")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

// ── handleCausal error paths ──────────────────────────────────────────────────

func TestHandleCausal_NotFound(t *testing.T) {
	s := newSuite(t)
	s.causal.err = &notFoundErr{}
	resp := s.get("/api/v1/causal/00000000-0000-0000-0000-000000000001")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestHandleCausal_InternalError(t *testing.T) {
	s := newSuite(t)
	s.causal.err = errors.New("causal failed")
	resp := s.get("/api/v1/causal/00000000-0000-0000-0000-000000000001")
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

func TestHandleCausal_WithParams(t *testing.T) {
	s := newSuite(t)
	s.causal.out = &interactors.GetCausalChainOutput{}
	id := uuid.New()
	resp := s.get("/api/v1/causal/" + id.String() + "?max_depth=3&include_consequences=true")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

// ── handleStore error path ────────────────────────────────────────────────────

func TestHandleStore_InternalError(t *testing.T) {
	s := newSuite(t)
	s.store.err = errors.New("store failed")
	resp := s.post("/api/v1/memories", map[string]any{"content": "hello", "wing": "test"})
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}
