package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/interactors"
)

type contractStore struct {
	inputs []interactors.StoreMemoryInput
}

func (f *contractStore) Execute(_ context.Context, input interactors.StoreMemoryInput) (*interactors.StoreMemoryOutput, error) {
	f.inputs = append(f.inputs, input)
	return &interactors.StoreMemoryOutput{}, nil
}

type contractSearch struct {
	inputs []interactors.SearchSemanticInput
}

func (f *contractSearch) Execute(_ context.Context, input interactors.SearchSemanticInput) ([]*interactors.SearchSemanticResult, error) {
	f.inputs = append(f.inputs, input)
	return []*interactors.SearchSemanticResult{}, nil
}

type contractClear struct {
	inputs []interactors.ClearMemoryInput
}

func (f *contractClear) Execute(_ context.Context, input interactors.ClearMemoryInput) (*interactors.ClearMemoryOutput, error) {
	f.inputs = append(f.inputs, input)
	return &interactors.ClearMemoryOutput{Mode: input.Mode}, nil
}

type contractArchive struct{}

func (contractArchive) Execute(context.Context) (*interactors.ArchiveMemoriesOutput, error) {
	return &interactors.ArchiveMemoriesOutput{Result: &valueobjects.ArchiveResult{
		TokensArchived: 9,
		TokensFreed:    9,
	}}, nil
}

func contractHandler(store *contractStore, search *contractSearch, clear *contractClear) http.Handler {
	h := NewHandler(store, nil, nil, nil, nil, search, nil, clear, nil, nil, nil, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func contractRequest(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	h.ServeHTTP(w, r)
	return w
}

func TestRESTIngestMinCharsOmittedVersusZero(t *testing.T) {
	store := &contractStore{}
	h := contractHandler(store, nil, nil)
	msg := `"messages":[{"role":"user","content":"short"}],"wing":"test"`

	w := contractRequest(h, http.MethodPost, "/api/v1/memories/ingest", "{"+msg+"}")
	if w.Code != http.StatusUnprocessableEntity || len(store.inputs) != 0 {
		t.Fatalf("omitted min_chars should default to 20; status=%d inputs=%d", w.Code, len(store.inputs))
	}
	w = contractRequest(h, http.MethodPost, "/api/v1/memories/ingest", "{"+msg+`,"min_chars":0}`)
	if w.Code != http.StatusCreated || len(store.inputs) != 1 || store.inputs[0].Content != "short" {
		t.Fatalf("explicit zero should disable only length threshold; status=%d inputs=%#v", w.Code, store.inputs)
	}
	w = contractRequest(h, http.MethodPost, "/api/v1/memories/ingest", `{"wing":"test","min_chars":0,"messages":[{"role":"user","content":""}]}`)
	if w.Code != http.StatusUnprocessableEntity || len(store.inputs) != 1 {
		t.Fatalf("empty message must remain invalid; status=%d inputs=%d", w.Code, len(store.inputs))
	}
}

func TestRESTClearRequiresExplicitGlobalOptIn(t *testing.T) {
	clear := &contractClear{}
	h := contractHandler(nil, nil, clear)
	w := contractRequest(h, http.MethodDelete, "/api/v1/memories", "{}")
	if w.Code != http.StatusUnprocessableEntity || len(clear.inputs) != 0 {
		t.Fatalf("missing clear mode must fail closed; status=%d inputs=%#v", w.Code, clear.inputs)
	}
	w = contractRequest(h, http.MethodDelete, "/api/v1/memories", `{"mode":"global"}`)
	if w.Code != http.StatusOK || len(clear.inputs) != 1 || clear.inputs[0].Mode != "global" {
		t.Fatalf("explicit global clear was not forwarded; status=%d inputs=%#v", w.Code, clear.inputs)
	}
	w = contractRequest(h, http.MethodDelete, "/api/v1/memories", `{"mode":"all"}`)
	if w.Code != http.StatusOK || len(clear.inputs) != 2 || clear.inputs[1].Mode != "global" {
		t.Fatalf("legacy explicit all alias should map to global; status=%d inputs=%#v", w.Code, clear.inputs)
	}
}

func TestRESTSearchRequiresWingOrExplicitGlobal(t *testing.T) {
	search := &contractSearch{}
	h := contractHandler(nil, search, nil)
	w := contractRequest(h, http.MethodPost, "/api/v1/memories/search", `{"query":"find"}`)
	if w.Code != http.StatusUnprocessableEntity || len(search.inputs) != 0 {
		t.Fatalf("unscoped search must fail closed; status=%d inputs=%#v", w.Code, search.inputs)
	}
	w = contractRequest(h, http.MethodPost, "/api/v1/memories/search", `{"query":"find","wing":"project-a","room":"decisions"}`)
	if w.Code != http.StatusOK || len(search.inputs) != 1 || search.inputs[0].Wing != "project-a" || search.inputs[0].Room == nil || *search.inputs[0].Room != "decisions" {
		t.Fatalf("wing-scoped search not forwarded; status=%d inputs=%#v", w.Code, search.inputs)
	}
	w = contractRequest(h, http.MethodPost, "/api/v1/memories/search", `{"query":"find","global":true}`)
	if w.Code != http.StatusOK || len(search.inputs) != 2 || !search.inputs[1].Global {
		t.Fatalf("explicit global search not forwarded; status=%d inputs=%#v", w.Code, search.inputs)
	}
}

func TestRESTGlobalSearchStillRequiresReadPermission(t *testing.T) {
	search := &contractSearch{}
	h := contractHandler(nil, search, nil)
	secured := authMiddleware("", map[string][]string{"write-token": {WingWrite}}, nil, h)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/memories/search", strings.NewReader(`{"query":"find","global":true}`))
	request.Header.Set("Authorization", "Bearer write-token")
	w := httptest.NewRecorder()
	secured.ServeHTTP(w, request)
	if w.Code != http.StatusForbidden || len(search.inputs) != 0 {
		t.Fatalf("global search must retain read authorization; status=%d inputs=%#v", w.Code, search.inputs)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/memories/search", strings.NewReader(`{"query":"find","global":true}`))
	request.Header.Set("Authorization", "Bearer read-token")
	secured = authMiddleware("", map[string][]string{"read-token": {WingRead}}, nil, h)
	w = httptest.NewRecorder()
	secured.ServeHTTP(w, request)
	if w.Code != http.StatusOK || len(search.inputs) != 1 || !search.inputs[0].Global {
		t.Fatalf("authorized explicit global search failed; status=%d inputs=%#v", w.Code, search.inputs)
	}
}

func TestRESTArchiveExposesRenamedFieldAndDeprecatedAlias(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, contractArchive{}, nil, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	w := contractRequest(mux, http.MethodPost, "/api/v1/archive", "")
	if w.Code != http.StatusOK {
		t.Fatalf("archive status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Result map[string]int `json:"result"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode archive response: %v", err)
	}
	if body.Result["tokens_archived"] != 9 || body.Result["tokens_freed"] != 9 {
		t.Fatalf("archive fields = %#v", body.Result)
	}
}
