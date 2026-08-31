package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ── wingForRequest ────────────────────────────────────────────────────────────

func TestWingForRequest_AllCases(t *testing.T) {
	cases := []struct {
		method string
		path   string
		want   string
	}{
		// Always-public
		{http.MethodGet, "/openapi.json", ""},
		// read wing
		{http.MethodGet, "/api/v1/memories", WingRead},
		{http.MethodGet, "/api/v1/stats", WingRead},
		{http.MethodPost, "/api/v1/memories/recall", WingRead},
		{http.MethodPost, "/api/v1/memories/search", WingRead},
		// write wing
		{http.MethodPost, "/api/v1/memories", WingWrite},
		{http.MethodPut, "/api/v1/memories/some-id", WingWrite},
		// delete wing
		{http.MethodDelete, "/api/v1/memories/some-id", WingDelete},
		// admin wing — consolidate only fires when path does NOT start with /api/v1/memories
		// (the write rule for /api/v1/memories is evaluated first in the switch)
		{http.MethodPost, "/api/v1/consolidate", WingAdmin},
		{http.MethodPost, "/api/v1/archive", WingAdmin},
		// default fallback (POST to unknown path)
		{http.MethodPost, "/api/v1/unknown-endpoint", WingWrite},
	}

	for _, tc := range cases {
		t.Run(tc.method+":"+tc.path, func(t *testing.T) {
			got := wingForRequest(tc.method, tc.path)
			if got != tc.want {
				t.Errorf("wingForRequest(%q, %q) = %q, want %q", tc.method, tc.path, got, tc.want)
			}
		})
	}
}

// ── recoveryMiddleware ────────────────────────────────────────────────────────

func TestRecoveryMiddleware_NoPanic(t *testing.T) {
	handler := recoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRecoveryMiddleware_PanicReturns500(t *testing.T) {
	handler := recoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// Must not crash the test process.
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 after panic, got %d", rec.Code)
	}
}
