package webhook

import (
	"context"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

func TestNewSimpleWebhookManager(t *testing.T) {
	m := NewSimpleWebhookManager(3, 100, 30*time.Second)

	if m == nil {
		t.Fatal("NewSimpleWebhookManager returned nil")
	}
	if m.workers != 3 {
		t.Errorf("workers = %d, want 3", m.workers)
	}
	if cap(m.queue) != 100 {
		t.Errorf("queue capacity = %d, want 100", cap(m.queue))
	}
	if m.timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", m.timeout)
	}
	if len(m.endpoints) != 0 {
		t.Error("endpoints should be empty initially")
	}
}

func TestIsEnabled(t *testing.T) {
	tests := []struct {
		name      string
		running   bool
		endpoints int
		expected  bool
	}{
		{"running with endpoints", true, 1, true},
		{"running without endpoints", true, 0, false},
		{"not running with endpoints", false, 1, false},
		{"not running without endpoints", false, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewSimpleWebhookManager(1, 10, time.Second)
			m.running = tt.running
			for i := 0; i < tt.endpoints; i++ {
				m.Register(context.Background(), "http://test", []string{"*"}, "")
			}
			if got := m.IsEnabled(); got != tt.expected {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestStartStop(t *testing.T) {
	m := NewSimpleWebhookManager(2, 10, time.Second)
	m.Register(context.Background(), "http://test", []string{"*"}, "")

	if m.running {
		t.Error("Should not be running initially")
	}

	m.Start()
	if !m.running {
		t.Error("Should be running after Start()")
	}

	// Double start should be safe
	m.Start()
	if !m.running {
		t.Error("Should still be running")
	}

	m.Stop()
	if m.running {
		t.Error("Should not be running after Stop()")
	}

	// Double stop should be safe
	m.Stop()
	if m.running {
		t.Error("Should still not be running")
	}
}

func TestRegister(t *testing.T) {
	m := NewSimpleWebhookManager(1, 10, time.Second)

	endpoint := m.Register(context.Background(), "https://example.com/webhook", []string{"store.completed", "recall.completed"}, "secret123")

	if endpoint == nil {
		t.Fatal("Register returned nil")
	}
	if endpoint.ID == uuid.Nil {
		t.Error("Endpoint should have a non-nil ID")
	}
	if endpoint.URL != "https://example.com/webhook" {
		t.Errorf("URL = %s, want 'https://example.com/webhook'", endpoint.URL)
	}
	if len(endpoint.Events) != 2 {
		t.Errorf("Events length = %d, want 2", len(endpoint.Events))
	}
	if endpoint.Secret != "secret123" {
		t.Error("Secret not set correctly")
	}
	if !endpoint.Active {
		t.Error("Endpoint should be active")
	}

	// Check it was stored
	if len(m.endpoints) != 1 {
		t.Error("Endpoint should be stored")
	}
}

func TestUnregister(t *testing.T) {
	m := NewSimpleWebhookManager(1, 10, time.Second)

	endpoint := m.Register(context.Background(), "http://test", []string{"*"}, "")
	if len(m.endpoints) != 1 {
		t.Fatal("Endpoint should be registered")
	}

	m.Unregister(context.Background(), endpoint.ID)
	if len(m.endpoints) != 0 {
		t.Error("Endpoint should be unregistered")
	}

	// Unregister non-existent should be safe
	m.Unregister(context.Background(), uuid.New())
}

func TestListWebhooks(t *testing.T) {
	m := NewSimpleWebhookManager(1, 10, time.Second)

	// Empty list
	list := m.ListWebhooks(context.Background())
	if len(list) != 0 {
		t.Error("List should be empty initially")
	}

	// Add endpoints
	e1 := m.Register(context.Background(), "http://test1", []string{"*"}, "")
	e2 := m.Register(context.Background(), "http://test2", []string{"*"}, "")

	list = m.ListWebhooks(context.Background())
	if len(list) != 2 {
		t.Errorf("List length = %d, want 2", len(list))
	}

	// Check both endpoints are in the list
	found := 0
	for _, e := range list {
		if e.ID == e1.ID || e.ID == e2.ID {
			found++
		}
	}
	if found != 2 {
		t.Error("Both endpoints should be in the list")
	}
}

func TestGetStats(t *testing.T) {
	m := NewSimpleWebhookManager(1, 100, time.Second)

	// Empty stats
	stats := m.GetStats(context.Background())
	if stats.RegisteredCount != 0 {
		t.Error("RegisteredCount should be 0")
	}

	// Add endpoints
	m.Register(context.Background(), "http://test1", []string{"*"}, "")
	m.Register(context.Background(), "http://test2", []string{"*"}, "")

	stats = m.GetStats(context.Background())
	if stats.RegisteredCount != 2 {
		t.Errorf("RegisteredCount = %d, want 2", stats.RegisteredCount)
	}
}

func TestTriggerNotRunning(t *testing.T) {
	m := NewSimpleWebhookManager(1, 10, time.Second)
	m.Register(context.Background(), "http://test", []string{"*"}, "")

	// Not running, should not queue anything
	m.Trigger(context.Background(), "test.event", map[string]interface{}{"key": "value"})

	if len(m.queue) != 0 {
		t.Error("Should not queue events when not running")
	}
}

func TestTriggerEventFiltering(t *testing.T) {
	// Use 0 workers to prevent event consumption
	m := NewSimpleWebhookManager(0, 10, time.Second)

	// Register endpoint for specific event BEFORE starting
	m.Register(context.Background(), "http://test", []string{"store.completed"}, "")

	// Just set running to true without starting workers
	m.running = true

	// Trigger different event - should not be queued
	m.Trigger(context.Background(), "recall.completed", map[string]interface{}{})

	// Check queue length safely
	m.mu.RLock()
	queueLenAfterWrongEvent := len(m.queue)
	m.mu.RUnlock()

	if queueLenAfterWrongEvent != 0 {
		t.Error("Should not queue events for non-subscribed types")
	}

	// Trigger correct event
	m.Trigger(context.Background(), "store.completed", map[string]interface{}{})

	// Check queue has the event
	m.mu.RLock()
	queueLenAfterCorrectEvent := len(m.queue)
	m.mu.RUnlock()

	if queueLenAfterCorrectEvent != 1 {
		t.Errorf("Queue length = %d, want 1", queueLenAfterCorrectEvent)
	}
}

func TestTriggerWildcard(t *testing.T) {
	m := NewSimpleWebhookManager(1, 10, time.Second)

	// Register endpoint for all events BEFORE starting
	m.Register(context.Background(), "http://test", []string{"*"}, "")

	m.Start()
	defer m.Stop()

	// Trigger any event
	m.Trigger(context.Background(), "any.event.type", map[string]interface{}{"data": "test"})

	// Queue should have the event
	select {
	case <-m.queue:
		// Success - event was queued
	case <-time.After(200 * time.Millisecond):
		// Check if queue has event in a safe way
		m.mu.RLock()
		queueLen := len(m.queue)
		m.mu.RUnlock()
		if queueLen == 0 {
			t.Error("Event should be queued for wildcard subscription")
		}
	}
}

func TestTriggerInactiveEndpoint(t *testing.T) {
	m := NewSimpleWebhookManager(1, 10, time.Second)

	// Register endpoint but mark inactive
	endpoint := m.Register(context.Background(), "http://test", []string{"*"}, "")
	endpoint.Active = false

	m.Start()
	defer m.Stop()

	m.Trigger(context.Background(), "test.event", map[string]interface{}{})

	// Should not queue events for inactive endpoints
	if len(m.queue) != 0 {
		t.Error("Should not queue events for inactive endpoints")
	}
}

func TestTriggerQueueFull(t *testing.T) {
	m := NewSimpleWebhookManager(1, 1, time.Second)
	m.Register(context.Background(), "http://test", []string{"*"}, "")
	m.Start()
	defer m.Stop()

	// Fill the queue
	m.Trigger(context.Background(), "event1", map[string]interface{}{})

	// Wait for event to be queued
	time.Sleep(10 * time.Millisecond)

	// This should be dropped (queue full)
	m.Trigger(context.Background(), "event2", map[string]interface{}{})

	if len(m.queue) > 1 {
		t.Error("Should drop events when queue is full")
	}
}

func TestSendWebhookEndpointNotFound(t *testing.T) {
	m := NewSimpleWebhookManager(1, 10, time.Second)

	event := ports.WebhookEvent{
		ID:         uuid.New(),
		EndpointID: uuid.New(), // Non-existent
		Type:       "test",
		Timestamp:  time.Now(),
	}

	// Should not panic
	m.sendWebhook(event)
}

func TestSendWebhookInactiveEndpoint(t *testing.T) {
	m := NewSimpleWebhookManager(1, 10, time.Second)
	endpoint := m.Register(context.Background(), "http://test", []string{"*"}, "")
	endpoint.Active = false

	event := ports.WebhookEvent{
		ID:         uuid.New(),
		EndpointID: endpoint.ID,
		Type:       "test",
		Timestamp:  time.Now(),
	}

	// Should not send (endpoint inactive)
	m.sendWebhook(event)
}

func TestSendWebhook_WithHMACSignature(t *testing.T) {
	// Créer un serveur de test
	secret := "test-secret"
	receivedSignature := ""
	receivedBody := []byte{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("X-MIRA-Signature")
		receivedBody = make([]byte, r.ContentLength)
		r.Body.Read(receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	m := NewSimpleWebhookManager(1, 10, 5*time.Second)
	endpoint := m.Register(context.Background(), server.URL, []string{"*"}, secret)

	// Créer un événement manuellement et l'envoyer
	event := ports.WebhookEvent{
		ID:         uuid.New(),
		EndpointID: endpoint.ID,
		Type:       "test.event",
		Payload:    map[string]interface{}{"key": "value"},
		Timestamp:  time.Now(),
	}

	// Envoyer directement le webhook
	m.sendWebhook(event)

	// Attendre un peu que la requête soit envoyée
	time.Sleep(100 * time.Millisecond)

	// Vérifier que la signature est présente et correcte
	if receivedSignature == "" {
		t.Error("Expected X-MIRA-Signature header")
	}

	// Vérifier le format sha256=<hex>
	if !strings.HasPrefix(receivedSignature, "sha256=") {
		t.Errorf("Invalid signature format: %s", receivedSignature)
	}

	// Vérifier que la signature est valide
	sigParts := strings.SplitN(receivedSignature, "=", 2)
	if len(sigParts) != 2 {
		t.Fatal("Invalid signature format")
	}

	expectedSig := computeHMAC(receivedBody, secret)
	if sigParts[1] != expectedSig {
		t.Errorf("Signature mismatch: got %s, want %s", sigParts[1], expectedSig)
	}
}

func TestSendWebhook_WithoutSecret(t *testing.T) {
	// Créer un serveur de test sans secret
	receivedSignature := ""

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("X-MIRA-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	m := NewSimpleWebhookManager(1, 10, 5*time.Second)
	endpoint := m.Register(context.Background(), server.URL, []string{"*"}, "") // Pas de secret

	// Créer un événement manuellement et l'envoyer
	event := ports.WebhookEvent{
		ID:         uuid.New(),
		EndpointID: endpoint.ID,
		Type:       "test.event",
		Payload:    map[string]interface{}{"key": "value"},
		Timestamp:  time.Now(),
	}

	// Envoyer directement le webhook
	m.sendWebhook(event)

	// Attendre un peu que la requête soit envoyée
	time.Sleep(100 * time.Millisecond)

	// Vérifier qu'il n'y a pas de signature
	if receivedSignature != "" {
		t.Error("Should not have X-MIRA-Signature header when no secret is configured")
	}
}

func TestComputeHMAC(t *testing.T) {
	payload := []byte(`{"test": "data"}`)
	secret := "my-secret"

	signature := computeHMAC(payload, secret)

	// Vérifier que c'est un hex valide
	if _, err := hex.DecodeString(signature); err != nil {
		t.Errorf("Invalid hex signature: %v", err)
	}

	// Vérifier la vérification
	if !verifyHMAC(payload, signature, secret) {
		t.Error("verifyHMAC should return true for valid signature")
	}

	// Vérifier qu'une mauvaise signature échoue
	if verifyHMAC(payload, "wrong-signature", secret) {
		t.Error("verifyHMAC should return false for invalid signature")
	}

	// Vérifier qu'un mauvais secret échoue
	if verifyHMAC(payload, signature, "wrong-secret") {
		t.Error("verifyHMAC should return false for wrong secret")
	}
}

func TestVerifyWebhookSignature(t *testing.T) {
	payload := []byte(`{"test": "data"}`)
	secret := "my-secret"

	m := NewSimpleWebhookManager(1, 10, 5*time.Second)

	// Calculer la signature attendue
	expectedSig := computeHMAC(payload, secret)

	// Vérifier avec le bon format
	if !m.VerifyWebhookSignature(payload, "sha256="+expectedSig, secret) {
		t.Error("VerifyWebhookSignature should return true for valid signature")
	}

	// Vérifier avec mauvais format (sans sha256=)
	if m.VerifyWebhookSignature(payload, expectedSig, secret) {
		t.Error("VerifyWebhookSignature should require 'sha256=' prefix")
	}

	// Vérifier avec mauvais secret
	if m.VerifyWebhookSignature(payload, "sha256="+expectedSig, "wrong-secret") {
		t.Error("VerifyWebhookSignature should return false for wrong secret")
	}

	// Vérifier avec header vide
	if m.VerifyWebhookSignature(payload, "", secret) {
		t.Error("VerifyWebhookSignature should return false for empty header")
	}

	// Vérifier avec secret vide
	if m.VerifyWebhookSignature(payload, "sha256="+expectedSig, "") {
		t.Error("VerifyWebhookSignature should return false for empty secret")
	}

	// Vérifier avec format invalide
	if m.VerifyWebhookSignature(payload, "md5="+expectedSig, secret) {
		t.Error("VerifyWebhookSignature should return false for wrong algorithm prefix")
	}
}

func BenchmarkRegister(b *testing.B) {
	m := NewSimpleWebhookManager(1, 1000, time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Register(context.Background(), "http://test", []string{"*"}, "")
	}
}

func BenchmarkTrigger(b *testing.B) {
	m := NewSimpleWebhookManager(1, 10000, time.Second)
	m.Register(context.Background(), "http://test", []string{"*"}, "")
	m.Start()
	defer m.Stop()

	payload := map[string]interface{}{"key": "value"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Trigger(context.Background(), "test.event", payload)
	}
}

func BenchmarkComputeHMAC(b *testing.B) {
	payload := []byte(`{"id": "550e8400-e29b-41d4-a716-446655440000", "type": "test.event", "timestamp": "2024-01-01T00:00:00Z", "payload": {"key": "value", "data": "test data for benchmarking"}}`)
	secret := "my-super-secret-key-for-webhook-signing"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		computeHMAC(payload, secret)
	}
}
