package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/usecases/ports"
)

type recordingAuditRepository struct {
	ports.AuditRepository
	logs chan *entities.AuditLog
}

func (r *recordingAuditRepository) SaveAuditLog(_ context.Context, entry *entities.AuditLog) error {
	r.logs <- entry
	return nil
}

func TestAuditMiddlewareDoesNotStoreBearerToken(t *testing.T) {
	const bearer = "recognizable-bearer-secret-token-123456"
	audit := &recordingAuditRepository{logs: make(chan *entities.AuditLog, 1)}
	handler := auditMiddleware(audit, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer "+bearer)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}

	select {
	case entry := <-audit.logs:
		if entry.Actor != "token:present" {
			t.Errorf("audit actor = %q, want token:present", entry.Actor)
		}
		stored := strings.Join([]string{entry.Actor, entry.Action, entry.Resource, entry.Metadata}, "\n")
		for _, prefix := range []string{bearer[:8], bearer[:16], bearer} {
			if strings.Contains(stored, prefix) {
				t.Errorf("audit entry leaked bearer prefix %q: %+v", prefix, entry)
			}
		}
		if entry.Action != "GET /api/v1/status" || entry.Status != http.StatusNoContent {
			t.Errorf("audit event = action %q status %d, want successful status event", entry.Action, entry.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("audit event was not saved")
	}
}

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
		// admin wing
		{http.MethodPost, "/api/v1/memories/consolidate", WingAdmin},
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
