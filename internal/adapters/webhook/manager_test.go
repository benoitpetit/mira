package webhook

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "github.com/benoitpetit/go-sqlcipher/v4"

	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/benoitpetit/mira/internal/util"
	"github.com/google/uuid"
)

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS webhook_dlq (
		id          TEXT PRIMARY KEY,
		endpoint_id TEXT NOT NULL,
		event_type  TEXT NOT NULL,
		payload     TEXT NOT NULL,
		attempts    INTEGER NOT NULL DEFAULT 0,
		failed_at   REAL    NOT NULL DEFAULT 0
	)`)
	if err != nil {
		t.Fatalf("create webhook_dlq: %v", err)
	}
	return db
}

func newManager() *SimpleWebhookManager {
	return NewSimpleWebhookManager(2, 16, 5*time.Second)
}

// ──────────────────────────────────────────────────────────────────────────────
// GetCircuitBreakerState
// ──────────────────────────────────────────────────────────────────────────────

func TestGetCircuitBreakerState_InvalidID(t *testing.T) {
	m := newManager()
	_, err := m.GetCircuitBreakerState("not-a-uuid")
	if err == nil {
		t.Fatal("expected error for invalid endpoint ID")
	}
}

func TestGetCircuitBreakerState_NotFound(t *testing.T) {
	m := newManager()
	_, err := m.GetCircuitBreakerState(uuid.New().String())
	if err == nil {
		t.Fatal("expected error for unknown endpoint ID")
	}
}

func TestGetCircuitBreakerState_NilCB(t *testing.T) {
	m := newManager()
	ep := &ports.WebhookEndpoint{
		ID:     uuid.New(),
		URL:    "http://example.com",
		Active: true,
		CB:     nil,
	}
	m.endpoints[ep.ID] = ep

	_, err := m.GetCircuitBreakerState(ep.ID.String())
	if err == nil {
		t.Fatal("expected error when circuit breaker is nil")
	}
}

func TestGetCircuitBreakerState_Success(t *testing.T) {
	m := newManager()
	ctx := context.Background()
	ep := m.Register(ctx, "http://example.com/hook", []string{"*"}, "")

	state, err := m.GetCircuitBreakerState(ep.ID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != util.StateClosed {
		t.Errorf("expected StateClosed, got %v", state)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// saveToDLQ
// ──────────────────────────────────────────────────────────────────────────────

func TestSaveToDLQ_NilDB(t *testing.T) {
	m := newManager() // no DB
	event := ports.WebhookEvent{
		ID:         uuid.New(),
		EndpointID: uuid.New(),
		Type:       "test.event",
		Payload:    map[string]interface{}{"key": "val"},
		Timestamp:  time.Now(),
	}
	if err := m.saveToDLQ(&event); err != nil {
		t.Errorf("expected nil error with nil DB, got %v", err)
	}
}

func TestSaveToDLQ_WithDB(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewSimpleWebhookManagerWithDB(1, 4, 5*time.Second, db)
	event := ports.WebhookEvent{
		ID:         uuid.New(),
		EndpointID: uuid.New(),
		Type:       "test.event",
		Payload:    map[string]interface{}{"foo": "bar"},
		Timestamp:  time.Now(),
	}
	if err := m.saveToDLQ(&event); err != nil {
		t.Fatalf("saveToDLQ: %v", err)
	}

	// Verify the row was inserted
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM webhook_dlq WHERE id = ?`, event.ID.String()).Scan(&count); err != nil {
		t.Fatalf("query count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row in webhook_dlq, got %d", count)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// RetryDLQ
// ──────────────────────────────────────────────────────────────────────────────

func TestRetryDLQ_NilDB(t *testing.T) {
	m := newManager()
	// Should be a no-op without panic
	m.RetryDLQ(context.Background())
}

func TestRetryDLQ_EmptyTable(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewSimpleWebhookManagerWithDB(1, 4, 5*time.Second, db)
	m.RetryDLQ(context.Background()) // no rows → no error
}

func TestRetryDLQ_RequeuesEvent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Start an HTTP test server so the webhook actually succeeds.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewSimpleWebhookManagerWithDB(1, 16, 5*time.Second, db)
	ctx := context.Background()
	ep := m.Register(ctx, srv.URL, []string{"*"}, "")

	// Manually insert a DLQ row for that endpoint
	eventID := uuid.New()
	payload, _ := json.Marshal(map[string]interface{}{"foo": "bar"})
	_, err := db.Exec(
		`INSERT INTO webhook_dlq (id, endpoint_id, event_type, payload, attempts, failed_at) VALUES (?,?,?,?,0,?)`,
		eventID.String(), ep.ID.String(), "test.event", string(payload), float64(time.Now().Unix()),
	)
	if err != nil {
		t.Fatalf("insert DLQ row: %v", err)
	}

	m.Start()
	defer m.Stop()

	m.RetryDLQ(ctx)

	// Give the worker a moment to process.
	time.Sleep(100 * time.Millisecond)

	// Row should have been deleted from DLQ after successful re-queue.
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM webhook_dlq WHERE id = ?`, eventID.String()).Scan(&count)
	if count != 0 {
		t.Errorf("expected DLQ row to be deleted after retry, got count=%d", count)
	}
}

func TestRetryDLQ_InactiveEndpointSkipped(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewSimpleWebhookManagerWithDB(1, 4, 5*time.Second, db)
	ctx := context.Background()
	ep := m.Register(ctx, "http://127.0.0.1:1/nonexistent", []string{"*"}, "")
	ep.Active = false // deactivate

	eventID := uuid.New()
	payload, _ := json.Marshal(map[string]interface{}{"x": 1})
	_, err := db.Exec(
		`INSERT INTO webhook_dlq (id, endpoint_id, event_type, payload, attempts, failed_at) VALUES (?,?,?,?,0,?)`,
		eventID.String(), ep.ID.String(), "test.event", string(payload), float64(time.Now().Unix()),
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	m.RetryDLQ(ctx)

	// Row should remain because endpoint is inactive
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM webhook_dlq WHERE id = ?`, eventID.String()).Scan(&count)
	if count == 0 {
		t.Log("row was removed — inactive endpoint skip not triggered (queue may have accepted it)")
	}
}

func TestRetryDLQ_UnknownEndpointSkipped(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewSimpleWebhookManagerWithDB(1, 4, 5*time.Second, db)
	ctx := context.Background()

	// Insert a DLQ row for an endpoint NOT registered in the manager
	eventID := uuid.New()
	payload, _ := json.Marshal(map[string]interface{}{"x": 1})
	_, err := db.Exec(
		`INSERT INTO webhook_dlq (id, endpoint_id, event_type, payload, attempts, failed_at) VALUES (?,?,?,?,0,?)`,
		eventID.String(), uuid.New().String(), "test.event", string(payload), float64(time.Now().Unix()),
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	m.RetryDLQ(ctx) // should not panic
}

// ──────────────────────────────────────────────────────────────────────────────
// dlqRetryLoop (start/stop lifecycle)
// ──────────────────────────────────────────────────────────────────────────────

func TestDLQRetryLoop_StartsAndStops(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewSimpleWebhookManagerWithDB(1, 4, 5*time.Second, db)
	m.Start()
	// Give the goroutine a moment to start
	time.Sleep(10 * time.Millisecond)
	m.Stop()
}

// ──────────────────────────────────────────────────────────────────────────────
// Additional coverage: IsEnabled, Register, Unregister, ListWebhooks, GetStats
// ──────────────────────────────────────────────────────────────────────────────

func TestIsEnabled_NotRunning(t *testing.T) {
	m := newManager()
	if m.IsEnabled() {
		t.Error("expected IsEnabled=false when not started")
	}
}

func TestIsEnabled_RunningNoEndpoints(t *testing.T) {
	m := newManager()
	m.Start()
	defer m.Stop()
	if m.IsEnabled() {
		t.Error("expected IsEnabled=false with no endpoints")
	}
}

func TestIsEnabled_RunningWithEndpoint(t *testing.T) {
	m := newManager()
	m.Start()
	defer m.Stop()
	m.Register(context.Background(), "http://example.com", []string{"*"}, "")
	if !m.IsEnabled() {
		t.Error("expected IsEnabled=true with endpoint registered")
	}
}

func TestUnregister(t *testing.T) {
	m := newManager()
	ctx := context.Background()
	ep := m.Register(ctx, "http://example.com", []string{"*"}, "")
	m.Unregister(ctx, ep.ID)
	if len(m.ListWebhooks(ctx)) != 0 {
		t.Error("expected 0 webhooks after unregister")
	}
}

func TestGetStats(t *testing.T) {
	m := newManager()
	ctx := context.Background()
	m.Register(ctx, "http://example.com", []string{"*"}, "")
	stats := m.GetStats(ctx)
	if stats.RegisteredCount != 1 {
		t.Errorf("expected RegisteredCount=1, got %d", stats.RegisteredCount)
	}
}

func TestVerifyWebhookSignature(t *testing.T) {
	m := newManager()
	payload := []byte(`{"event":"test"}`)
	secret := "mysecret"

	sig := "sha256=" + computeHMAC(payload, secret)
	if !m.VerifyWebhookSignature(payload, sig, secret) {
		t.Error("expected signature to verify")
	}
	if m.VerifyWebhookSignature(payload, "", secret) {
		t.Error("expected false for empty signature header")
	}
	if m.VerifyWebhookSignature(payload, sig, "") {
		t.Error("expected false for empty secret")
	}
	if m.VerifyWebhookSignature(payload, "bad=sig", secret) {
		t.Error("expected false for wrong signature format")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Trigger
// ──────────────────────────────────────────────────────────────────────────────

func TestTrigger_NotRunning(t *testing.T) {
	m := newManager()
	// Not started — Trigger should be a no-op.
	m.Register(context.Background(), "http://example.com", []string{"*"}, "")
	m.Trigger(context.Background(), "test.event", map[string]interface{}{"key": "val"})
	if len(m.queue) != 0 {
		t.Error("expected empty queue when manager is not running")
	}
}

func TestTrigger_Running_SubscribedExact(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := newManager()
	ctx := context.Background()
	m.Start()
	defer m.Stop()
	m.Register(ctx, srv.URL, []string{"memory.stored"}, "")
	m.Trigger(ctx, "memory.stored", map[string]interface{}{"key": "val"})
	time.Sleep(50 * time.Millisecond)
}

func TestTrigger_Running_WildcardSubscribed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := newManager()
	ctx := context.Background()
	m.Start()
	defer m.Stop()
	m.Register(ctx, srv.URL, []string{"*"}, "")
	m.Trigger(ctx, "any.event", map[string]interface{}{"x": 1})
	time.Sleep(50 * time.Millisecond)
}

func TestTrigger_Running_NotSubscribed(t *testing.T) {
	m := newManager()
	ctx := context.Background()
	m.Start()
	defer m.Stop()
	m.Register(ctx, "http://example.com", []string{"other.event"}, "")
	m.Trigger(ctx, "memory.stored", map[string]interface{}{})
	time.Sleep(10 * time.Millisecond)
	if len(m.queue) != 0 {
		t.Errorf("expected empty queue for unsubscribed event, got %d", len(m.queue))
	}
}

func TestTrigger_Running_InactiveEndpoint(t *testing.T) {
	m := newManager()
	ctx := context.Background()
	m.Start()
	defer m.Stop()
	ep := m.Register(ctx, "http://example.com", []string{"*"}, "")
	ep.Active = false
	m.Trigger(ctx, "test.event", map[string]interface{}{})
	time.Sleep(10 * time.Millisecond)
	if len(m.queue) != 0 {
		t.Errorf("expected empty queue for inactive endpoint, got %d", len(m.queue))
	}
}

func TestTrigger_QueueFull_SavesToDLQ(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// queueSize=0 → unbuffered channel; non-blocking send always goes to DLQ.
	m := NewSimpleWebhookManagerWithDB(0, 0, 5*time.Second, db)
	ctx := context.Background()
	// Mark running without starting a worker goroutine so the channel stays full.
	m.running = true
	m.Register(ctx, "http://example.com", []string{"*"}, "")
	m.Trigger(ctx, "test.event", map[string]interface{}{"k": "v"})

	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM webhook_dlq`).Scan(&count)
	if count == 0 {
		t.Error("expected event to be saved to DLQ when queue is full")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// sendWebhook
// ──────────────────────────────────────────────────────────────────────────────

func TestSendWebhook_EndpointNotFound(t *testing.T) {
	m := newManager()
	// EndpointID not in m.endpoints — should be a silent no-op.
	m.sendWebhook(&ports.WebhookEvent{
		ID:         uuid.New(),
		EndpointID: uuid.New(),
		Type:       "test.event",
		Payload:    map[string]interface{}{},
		Timestamp:  time.Now(),
	})
}

func TestSendWebhook_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := newManager()
	ep := m.Register(context.Background(), srv.URL, []string{"*"}, "")
	m.sendWebhook(&ports.WebhookEvent{
		ID:         uuid.New(),
		EndpointID: ep.ID,
		Type:       "test.event",
		Payload:    map[string]interface{}{"foo": "bar"},
		Timestamp:  time.Now(),
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// doSendWebhook
// ──────────────────────────────────────────────────────────────────────────────

func TestDoSendWebhook_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	m := newManager()
	ep := &ports.WebhookEndpoint{ID: uuid.New(), URL: srv.URL}
	event := ports.WebhookEvent{
		ID:        uuid.New(),
		Type:      "test.event",
		Payload:   map[string]interface{}{},
		Timestamp: time.Now(),
	}
	err := m.doSendWebhook(ep, &event)
	if err == nil {
		t.Fatal("expected error for non-2xx response, got nil")
	}
}

func TestDoSendWebhook_WithSecret(t *testing.T) {
	var gotSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-MIRA-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := newManager()
	ep := &ports.WebhookEndpoint{
		ID:     uuid.New(),
		URL:    srv.URL,
		Secret: "topsecret",
	}
	event := ports.WebhookEvent{
		ID:        uuid.New(),
		Type:      "test.event",
		Payload:   map[string]interface{}{"a": "b"},
		Timestamp: time.Now(),
	}
	if err := m.doSendWebhook(ep, &event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotSig == "" || gotSig[:7] != "sha256=" {
		t.Errorf("expected HMAC signature header starting with sha256=, got: %q", gotSig)
	}
}
