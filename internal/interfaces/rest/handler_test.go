package rest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/config"
	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/interfaces/rest"
	"github.com/benoitpetit/mira/internal/usecases/interactors"
	"github.com/google/uuid"
)

// ── Fake executors ────────────────────────────────────────────────────────────

type fakeStore struct {
	out    *interactors.StoreMemoryOutput
	err    error
	inputs []interactors.StoreMemoryInput
}

func (f *fakeStore) Execute(_ context.Context, input interactors.StoreMemoryInput) (*interactors.StoreMemoryOutput, error) {
	f.inputs = append(f.inputs, input)
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
	out    []*interactors.SearchSemanticResult
	err    error
	inputs []interactors.SearchSemanticInput
}

func (f *fakeSearch) Execute(_ context.Context, input interactors.SearchSemanticInput) ([]*interactors.SearchSemanticResult, error) {
	f.inputs = append(f.inputs, input)
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
	out    *interactors.ClearMemoryOutput
	err    error
	inputs []interactors.ClearMemoryInput
}

func (f *fakeClear) Execute(_ context.Context, input interactors.ClearMemoryInput) (*interactors.ClearMemoryOutput, error) {
	f.inputs = append(f.inputs, input)
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
	out    *interactors.GetCausalChainOutput
	err    error
	inputs []interactors.GetCausalChainInput
}

func (f *fakeCausal) Execute(_ context.Context, input interactors.GetCausalChainInput) (*interactors.GetCausalChainOutput, error) {
	f.inputs = append(f.inputs, input)
	return f.out, f.err
}

type fakeStatus struct {
	out *interactors.GetStatusOutput
	err error
}

func (f *fakeStatus) Execute(_ context.Context) (*interactors.GetStatusOutput, error) {
	return f.out, f.err
}

type fakeAgentMemoryStatus struct {
	out *interactors.AgentMemoryStatusSummary
	err error
}

func (f *fakeAgentMemoryStatus) QueryStatus(_ context.Context) (*interactors.AgentMemoryStatusSummary, error) {
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
	handler     *rest.Handler
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
	s.handler = h
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

func TestHandleIngest_Success(t *testing.T) {
	s := newSuite(t)
	s.store.out = &interactors.StoreMemoryOutput{}
	resp := s.post("/api/v1/memories/ingest", map[string]any{
		"wing": "test",
		"messages": []map[string]string{
			{"role": "user", "content": "Keep this durable user decision in memory."},
			{"role": "assistant", "content": "This assistant message is excluded by default."},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("want 201, got %d", resp.StatusCode)
	}
	var out struct {
		Selected int  `json:"selected"`
		Stored   int  `json:"stored"`
		DryRun   bool `json:"dry_run"`
	}
	decodeJSON(t, resp, &out)
	if out.Selected != 1 || out.Stored != 1 || out.DryRun {
		t.Errorf("response = %#v", out)
	}
	if len(s.store.inputs) != 1 || s.store.inputs[0].Kind == nil || *s.store.inputs[0].Kind != valueobjects.KindHistory {
		t.Errorf("stored inputs = %#v", s.store.inputs)
	}
}

func TestHandleIngest_DryRunDoesNotStore(t *testing.T) {
	s := newSuite(t)
	resp := s.post("/api/v1/memories/ingest", map[string]any{
		"wing":     "test",
		"dry_run":  true,
		"messages": []map[string]string{{"role": "user", "content": "Keep this durable user decision in memory."}},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if len(s.store.inputs) != 0 {
		t.Errorf("dry run stored %#v", s.store.inputs)
	}
}

func TestHandleIngest_ExplicitZeroDisablesMinimumLength(t *testing.T) {
	s := newSuite(t)
	resp := s.post("/api/v1/memories/ingest", map[string]any{
		"wing": "test", "min_chars": 0,
		"messages": []map[string]string{{"role": "user", "content": "short but valid"}},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("explicit min_chars=0 should keep a non-empty message; got %d", resp.StatusCode)
	}
	if len(s.store.inputs) != 1 || s.store.inputs[0].Content != "short but valid" {
		t.Fatalf("stored inputs = %#v", s.store.inputs)
	}
}

func TestHandleIngest_ExplicitZeroStillRejectsEmptyMessage(t *testing.T) {
	s := newSuite(t)
	resp := s.post("/api/v1/memories/ingest", map[string]any{
		"wing": "test", "min_chars": 0,
		"messages": []map[string]string{{"role": "user", "content": ""}},
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("empty message should remain invalid at min_chars=0; got %d", resp.StatusCode)
	}
	if len(s.store.inputs) != 0 {
		t.Fatalf("empty message reached storage: %#v", s.store.inputs)
	}
}

func TestHandleIngest_DryRunReportsConfiguredContentLimitFailures(t *testing.T) {
	s := newSuite(t)
	s.handler.SetMaxContentLength(4)
	resp := s.post("/api/v1/memories/ingest", map[string]any{
		"wing":      "test",
		"min_chars": 1,
		"dry_run":   true,
		"messages": []map[string]string{
			{"role": "assistant", "content": "excluded"},
			{"role": "user", "content": "four"},
			{"role": "user", "content": "five!"},
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var out struct {
		Selected            int  `json:"selected"`
		Stored              int  `json:"stored"`
		Failed              int  `json:"failed"`
		WouldFailValidation int  `json:"would_fail_validation"`
		DryRun              bool `json:"dry_run"`
		ValidationErrors    []struct {
			MessageIndex int    `json:"message_index"`
			Error        string `json:"error"`
		} `json:"validation_errors"`
	}
	decodeJSON(t, resp, &out)
	if out.Selected != 2 || out.Stored != 0 || out.Failed != 0 || out.WouldFailValidation != 1 || !out.DryRun {
		t.Errorf("dry-run response = %#v", out)
	}
	if len(out.ValidationErrors) != 1 || out.ValidationErrors[0].MessageIndex != 3 || !strings.Contains(out.ValidationErrors[0].Error, "maximum length of 4") {
		t.Errorf("dry-run validation errors = %#v", out.ValidationErrors)
	}
	if len(s.store.inputs) != 0 {
		t.Errorf("dry run stored %#v", s.store.inputs)
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

	resp := s.post("/api/v1/memories/search", map[string]any{"query": "find this", "top_k": 5, "wing": "project-a"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]any
	decodeJSON(t, resp, &body)
	results, ok := body["results"].([]any)
	if !ok || len(results) != 1 {
		t.Errorf("expected 1 result, got %v", body["results"])
	}
	if len(s.search.inputs) != 1 || s.search.inputs[0].Wing != "project-a" || s.search.inputs[0].Global {
		t.Errorf("search input = %#v, want scoped to project-a", s.search.inputs)
	}
}

func TestHandleSearch_RequiresExplicitWingOrGlobalOptIn(t *testing.T) {
	s := newSuite(t)
	resp := s.post("/api/v1/memories/search", map[string]any{"query": "find this"})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("missing scope should be rejected; got %d", resp.StatusCode)
	}
	if len(s.search.inputs) != 0 {
		t.Fatalf("unscoped search reached executor: %#v", s.search.inputs)
	}
}

func TestHandleSearch_GlobalRequiresExplicitOptIn(t *testing.T) {
	s := newSuite(t)
	resp := s.post("/api/v1/memories/search", map[string]any{"query": "find this", "global": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("explicit global search status = %d", resp.StatusCode)
	}
	if len(s.search.inputs) != 1 || !s.search.inputs[0].Global || s.search.inputs[0].Wing != "" {
		t.Fatalf("search input = %#v, want explicit global scope", s.search.inputs)
	}
}

func TestHandleSearch_EmptyResults(t *testing.T) {
	s := newSuite(t)
	s.search.out = nil // no results

	resp := s.post("/api/v1/memories/search", map[string]any{"query": "nothing", "wing": "test"})
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
	s.clear.out = &interactors.ClearMemoryOutput{DeletedCount: 5, Mode: "global"}

	resp := s.do(http.MethodDelete, "/api/v1/memories", map[string]any{"mode": "global"})
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
		Result: &valueobjects.ArchiveResult{SessionNotes: 2, DebugLogs: 1, TokensArchived: 500, TokensFreed: 500},
	}

	resp := s.post("/api/v1/archive", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body struct {
		Result map[string]any `json:"result"`
	}
	decodeJSON(t, resp, &body)
	if body.Result["tokens_archived"] != float64(500) || body.Result["tokens_freed"] != float64(500) {
		t.Errorf("archive compatibility fields = %#v", body.Result)
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

func TestHandleCausalDefaultsDepthAndSerializesTruncationWithoutListener(t *testing.T) {
	causal := &fakeCausal{out: &interactors.GetCausalChainOutput{Truncated: true}}
	h := rest.NewHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, causal, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/causal/550e8400-e29b-41d4-a716-446655440000", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if len(causal.inputs) != 1 || causal.inputs[0].MaxDepth != valueobjects.DefaultCausalMaxDepth {
		t.Fatalf("causal inputs = %+v, want default depth %d", causal.inputs, valueobjects.DefaultCausalMaxDepth)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["truncated"] != true {
		t.Fatalf("truncated = %v, want true", body["truncated"])
	}
}

func TestHandleCausalRejectsInvalidDepthWithoutListener(t *testing.T) {
	h := rest.NewHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, &fakeCausal{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	for _, depth := range []string{"-1", "11", "not-a-number"} {
		t.Run(depth, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/causal/550e8400-e29b-41d4-a716-446655440000?max_depth="+depth, nil)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, req)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", response.Code)
			}
		})
	}
}

func TestOpenAPICausalContractMatchesRuntimeBounds(t *testing.T) {
	response := httptest.NewRecorder()
	rest.ServeSpec(response, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	var doc map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode OpenAPI document: %v", err)
	}
	paths := doc["paths"].(map[string]any)
	operation := paths["/api/v1/causal/{id}"].(map[string]any)["get"].(map[string]any)
	parameters := operation["parameters"].([]any)
	var depthSchema map[string]any
	for _, parameter := range parameters {
		p := parameter.(map[string]any)
		if p["name"] == "max_depth" {
			depthSchema = p["schema"].(map[string]any)
			break
		}
	}
	if depthSchema == nil || depthSchema["minimum"] != float64(0) || depthSchema["maximum"] != float64(valueobjects.MaxCausalDepth) || depthSchema["default"] != float64(valueobjects.DefaultCausalMaxDepth) {
		t.Fatalf("max_depth schema = %#v", depthSchema)
	}
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	causalProperties := schemas["CausalChainResponse"].(map[string]any)["properties"].(map[string]any)
	if causalProperties["truncated"].(map[string]any)["type"] != "boolean" {
		t.Fatalf("causal response truncated schema = %#v", causalProperties["truncated"])
	}
}

func TestOpenAPIConversationIngestDeclaresMessageLimit(t *testing.T) {
	response := httptest.NewRecorder()
	rest.ServeSpec(response, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	var doc map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode OpenAPI document: %v", err)
	}
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	request := schemas["ConversationIngestRequest"].(map[string]any)
	properties := request["properties"].(map[string]any)
	messages := properties["messages"].(map[string]any)
	if messages["maxItems"] != float64(1000) {
		t.Fatalf("ConversationIngestRequest.messages maxItems = %v, want 1000", messages["maxItems"])
	}
	ingestResponse := schemas["ConversationIngestResponse"].(map[string]any)
	responseProperties := ingestResponse["properties"].(map[string]any)
	if responseProperties["would_fail_validation"].(map[string]any)["type"] != "integer" {
		t.Fatalf("ConversationIngestResponse.would_fail_validation schema = %#v", responseProperties["would_fail_validation"])
	}
	validationErrors := responseProperties["validation_errors"].(map[string]any)
	validationItem := validationErrors["items"].(map[string]any)
	validationProperties := validationItem["properties"].(map[string]any)
	if validationProperties["message_index"].(map[string]any)["type"] != "integer" || validationProperties["error"].(map[string]any)["type"] != "string" {
		t.Fatalf("ConversationIngestResponse.validation_errors item schema = %#v", validationItem)
	}
}

func TestOpenAPITransportScopeAndMinCharsContracts(t *testing.T) {
	response := httptest.NewRecorder()
	rest.ServeSpec(response, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	var document struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Description string `json:"description"`
					Deprecated  bool   `json:"deprecated"`
					Default     int    `json:"default"`
				} `json:"properties"`
				Required []string `json:"required"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.NewDecoder(response.Body).Decode(&document); err != nil {
		t.Fatalf("decode OpenAPI document: %v", err)
	}
	search := document.Components.Schemas["SearchRequest"].Properties
	if !strings.Contains(search["wing"].Description, "Required unless global") || !strings.Contains(search["global"].Description, "Explicitly search") {
		t.Errorf("search scope schema does not explain explicit wing/global scope: %#v", search)
	}
	clear := document.Components.Schemas["ClearMemoriesRequest"].Properties
	if !strings.Contains(clear["mode"].Description, "Required") || !strings.Contains(clear["mode"].Description, "global") {
		t.Errorf("clear scope schema does not require explicit global mode: %#v", clear["mode"])
	}
	if got := document.Components.Schemas["ClearMemoriesRequest"].Required; len(got) != 1 || got[0] != "mode" {
		t.Errorf("clear schema required fields = %#v, want [mode]", got)
	}
	minChars := document.Components.Schemas["ConversationIngestRequest"].Properties["min_chars"].Description
	if !strings.Contains(minChars, "omitted defaults to 20") || !strings.Contains(minChars, "0 disables") || document.Components.Schemas["ConversationIngestRequest"].Properties["min_chars"].Default != 20 {
		t.Errorf("min_chars schema is missing default/zero semantics: %q", minChars)
	}
	archive := document.Components.Schemas["ArchiveResult"].Properties
	if _, ok := archive["tokens_archived"]; !ok || !archive["tokens_freed"].Deprecated {
		t.Errorf("archive schema must expose tokens_archived and mark tokens_freed deprecated: %#v", archive)
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
		Version: config.CurrentVersion,
		Uptime:  "1h",
	}

	resp := s.get("/api/v1/status")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestHandleStatus_IncludesAgentMemory(t *testing.T) {
	s := newSuite(t)
	s.status.out = &interactors.GetStatusOutput{Stats: valueobjects.NewStats()}
	h := rest.NewHandler(
		s.store, s.recall, s.load, s.update, s.del,
		s.search, s.consolidate, s.clear, s.timeline,
		s.archive, s.causal, s.status, s.audit, nil,
	)
	h.SetAgentMemoryQuerier(&fakeAgentMemoryStatus{out: &interactors.AgentMemoryStatusSummary{
		Enabled:    true,
		AgentCount: 1,
		Agents: []interactors.AgentMemoryAgentSummary{{
			AgentID:         "agent-test",
			Version:         2,
			ConfidenceScore: 0.91,
			TraitCount:      4,
		}},
	}})
	srv := rest.NewServer(h, ":0", "", nil, 5*time.Second, 5*time.Second)
	server := httptest.NewServer(srv.Handler)
	t.Cleanup(server.Close)

	resp, err := http.Get(server.URL + "/api/v1/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var body interactors.GetStatusOutput
	decodeJSON(t, resp, &body)
	if body.AgentMemory == nil || !body.AgentMemory.Enabled || body.AgentMemory.AgentCount != 1 {
		t.Fatalf("agent memory status missing or inconsistent: %#v", body.AgentMemory)
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

func TestAuth_DashboardShell_NoToken_Allowed(t *testing.T) {
	s := newSuiteWithAuth(t, "secret-token")
	for _, path := range []string{"/", "/index.html", "/app.js", "/styles.css"} {
		resp := s.get(path)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s without token: want 200, got %d", path, resp.StatusCode)
		}
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
	resp := s.post("/api/v1/memories/search", map[string]any{"query": "test", "wing": "test"})
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
	resp := s.do(http.MethodDelete, "/api/v1/memories", map[string]any{"mode": "global"})
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

func TestHandleClear_EmptyBody(t *testing.T) {
	s := newSuite(t)
	resp := s.do(http.MethodDelete, "/api/v1/memories", nil)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("empty clear body must not mean global delete; want 422, got %d", resp.StatusCode)
	}
	if len(s.clear.inputs) != 0 {
		t.Fatalf("unscoped clear reached executor: %#v", s.clear.inputs)
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

func TestHandleTimeline_InvalidTypeIsBadRequest(t *testing.T) {
	handler := rest.NewHandler(
		&fakeStore{}, &fakeRecall{}, &fakeLoad{}, &fakeUpdate{}, &fakeDelete{},
		&fakeSearch{}, &fakeConsolidate{}, &fakeClear{}, &fakeTimeline{},
		&fakeArchive{}, &fakeCausal{}, &fakeStatus{}, nil, nil,
	)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/timeline?type=invalid_type", nil)
	rest.NewServer(handler, ":0", "", nil, time.Second, time.Second).Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
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

func TestHandleStoreAcceptsContentAboveLegacyLimit(t *testing.T) {
	store := &fakeStore{}
	h := rest.NewHandler(store, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	body, err := json.Marshal(map[string]any{
		"content": strings.Repeat("x", 70000),
		"wing":    "test",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/memories", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("want 201 for content below the configured default limit, got %d: %s", recorder.Code, recorder.Body)
	}
	if len(store.inputs) != 1 || len([]rune(store.inputs[0].Content)) != 70000 {
		t.Fatalf("store received %d inputs, want one containing 70000 runes", len(store.inputs))
	}
}

func TestHandleStoreUsesConfiguredContentLimit(t *testing.T) {
	store := &fakeStore{}
	h := rest.NewHandler(store, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetMaxContentLength(70000)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	for _, tc := range []struct {
		name   string
		runes  int
		status int
	}{
		{name: "at configured limit", runes: 70000, status: http.StatusCreated},
		{name: "above configured limit", runes: 70001, status: http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"content": strings.Repeat("é", tc.runes), "wing": "test"})
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/memories", bytes.NewReader(body))
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, request)
			if recorder.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, tc.status, recorder.Body)
			}
		})
	}
}
