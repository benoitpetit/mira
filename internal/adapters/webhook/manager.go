// Package webhook provides webhook notification adapters.
// Webhook manager adapter - implements ports.WebhookManager
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/benoitpetit/mira/internal/util"
	"github.com/google/uuid"
)

// SimpleWebhookManager implements a basic webhook manager
type SimpleWebhookManager struct {
	mu        sync.RWMutex
	endpoints map[uuid.UUID]*ports.WebhookEndpoint
	client    *http.Client
	workers   int
	queueSize int
	timeout   time.Duration
	queue     chan ports.WebhookEvent
	stopChan  chan struct{}
	running   bool
	wg        sync.WaitGroup
	db        *sql.DB
}

// NewSimpleWebhookManager creates a new simple webhook manager
func NewSimpleWebhookManager(workers, queueSize int, timeout time.Duration) *SimpleWebhookManager {
	return NewSimpleWebhookManagerWithDB(workers, queueSize, timeout, nil)
}

// NewSimpleWebhookManagerWithDB creates a webhook manager with DLQ persistence
func NewSimpleWebhookManagerWithDB(workers, queueSize int, timeout time.Duration, db *sql.DB) *SimpleWebhookManager {
	return &SimpleWebhookManager{
		endpoints: make(map[uuid.UUID]*ports.WebhookEndpoint),
		client:    &http.Client{Timeout: timeout},
		workers:   workers,
		queueSize: queueSize,
		timeout:   timeout,
		queue:     make(chan ports.WebhookEvent, queueSize),
		stopChan:  make(chan struct{}),
		db:        db,
	}
}

// IsEnabled returns whether webhooks are enabled
func (m *SimpleWebhookManager) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running && len(m.endpoints) > 0
}

// Start starts the webhook manager
func (m *SimpleWebhookManager) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return
	}

	m.running = true
	m.stopChan = make(chan struct{})

	// Start workers
	for i := 0; i < m.workers; i++ {
		m.wg.Add(1)
		go m.worker()
	}

	// Start DLQ retry loop
	if m.db != nil {
		m.wg.Add(1)
		go m.dlqRetryLoop()
	}
}

// Stop stops the webhook manager
func (m *SimpleWebhookManager) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	close(m.stopChan)
	m.mu.Unlock()

	// Wait for workers to finish
	m.wg.Wait()
}

// Register registers a new webhook endpoint
// If secret is provided, all webhook payloads will be signed with HMAC-SHA256
// The signature will be sent in the X-MIRA-Signature header as: sha256=<hex>
func (m *SimpleWebhookManager) Register(ctx context.Context, url string, events []string, secret string) *ports.WebhookEndpoint {
	m.mu.Lock()
	defer m.mu.Unlock()

	endpoint := &ports.WebhookEndpoint{
		ID:        uuid.New(),
		URL:       url,
		Events:    events,
		Secret:    secret,
		Active:    true,
		CreatedAt: time.Now(),
		CB:        util.NewCircuitBreaker(util.DefaultCircuitBreakerConfig()),
	}

	m.endpoints[endpoint.ID] = endpoint
	return endpoint
}

// Unregister removes a webhook endpoint
func (m *SimpleWebhookManager) Unregister(ctx context.Context, id uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.endpoints, id)
}

// ListWebhooks returns all registered webhooks
func (m *SimpleWebhookManager) ListWebhooks(ctx context.Context) []*ports.WebhookEndpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*ports.WebhookEndpoint, 0, len(m.endpoints))
	for _, endpoint := range m.endpoints {
		result = append(result, endpoint)
	}
	return result
}

// GetStats returns webhook statistics
func (m *SimpleWebhookManager) GetStats(ctx context.Context) ports.WebhookStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return ports.WebhookStats{
		RegisteredCount: len(m.endpoints),
		QueueDepth:      len(m.queue),
	}
}

// Trigger sends a webhook event to all registered endpoints
func (m *SimpleWebhookManager) Trigger(ctx context.Context, eventType string, payload map[string]interface{}) {
	m.mu.RLock()
	endpoints := make(map[uuid.UUID]*ports.WebhookEndpoint, len(m.endpoints))
	for k, v := range m.endpoints {
		endpoints[k] = v
	}
	running := m.running
	m.mu.RUnlock()

	if !running {
		return
	}

	for _, endpoint := range endpoints {
		// Check if endpoint subscribes to this event type
		subscribed := false
		for _, e := range endpoint.Events {
			if e == "*" || e == eventType {
				subscribed = true
				break
			}
		}

		if subscribed && endpoint.Active {
			event := ports.WebhookEvent{
				ID:         uuid.New(),
				EndpointID: endpoint.ID,
				Type:       eventType,
				Payload:    payload,
				Timestamp:  time.Now(),
			}

			select {
			case m.queue <- event:
				// Event queued successfully
			default:
				// Queue is full, persist to DLQ
				if err := m.saveToDLQ(event); err != nil {
					log.Printf("[Webhook] Failed to save event to DLQ: %v", err)
				}
			}
		}
	}
}

// GetCircuitBreakerState returns the circuit breaker state for an endpoint (for monitoring)
func (m *SimpleWebhookManager) GetCircuitBreakerState(endpointID string) (util.State, error) {
	id, err := uuid.Parse(endpointID)
	if err != nil {
		return 0, errors.New("invalid endpoint ID")
	}

	m.mu.RLock()
	endpoint, ok := m.endpoints[id]
	m.mu.RUnlock()

	if !ok {
		return 0, errors.New("endpoint not found")
	}

	if endpoint.CB == nil {
		return 0, errors.New("circuit breaker not initialized")
	}

	return endpoint.CB.State(), nil
}

// worker processes webhook events
func (m *SimpleWebhookManager) worker() {
	defer m.wg.Done()

	for {
		select {
		case event := <-m.queue:
			m.sendWebhook(event)
		case <-m.stopChan:
			return
		}
	}
}

// sendWebhook sends a webhook event to the endpoint using circuit breaker
func (m *SimpleWebhookManager) sendWebhook(event ports.WebhookEvent) {
	// Find the endpoint for this event
	m.mu.RLock()
	endpoint, ok := m.endpoints[event.EndpointID]
	m.mu.RUnlock()

	if !ok || endpoint == nil || !endpoint.Active {
		return
	}

	// If circuit breaker is not initialized, initialize it
	if endpoint.CB == nil {
		endpoint.CB = util.NewCircuitBreaker(util.DefaultCircuitBreakerConfig())
	}

	// Use the circuit breaker to send the webhook
	err := endpoint.CB.Execute(func() error {
		return m.doSendWebhook(endpoint, event)
	})

	if errors.Is(err, util.ErrCircuitOpen) {
		log.Printf("[Webhook] Circuit breaker open for %s, skipping", endpoint.URL)
		return
	}

	if err != nil {
		log.Printf("[Webhook] Failed to send to %s: %v", endpoint.URL, err)
	}
}

// doSendWebhook performs the actual HTTP request
func (m *SimpleWebhookManager) doSendWebhook(endpoint *ports.WebhookEndpoint, event ports.WebhookEvent) error {
	payload, err := json.Marshal(map[string]interface{}{
		"id":        event.ID,
		"type":      event.Type,
		"timestamp": event.Timestamp,
		"payload":   event.Payload,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", endpoint.URL, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-ID", event.ID.String())
	req.Header.Set("X-Webhook-Event", event.Type)
	req.Header.Set("X-MIRA-Event-ID", event.ID.String())
	req.Header.Set("X-MIRA-Event-Type", event.Type)
	req.Header.Set("X-MIRA-Webhook-ID", endpoint.ID.String())

	// Add HMAC signature if secret is configured
	if endpoint.Secret != "" {
		signature := computeHMAC(payload, endpoint.Secret)
		req.Header.Set("X-MIRA-Signature", "sha256="+signature)
		req.Header.Set("X-MIRA-Signature-Version", "v1")
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Treat non-2xx responses as failures
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("webhook returned status " + resp.Status)
	}

	return nil
}

// computeHMAC computes the HMAC-SHA256 signature
func computeHMAC(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyHMAC verifies an HMAC signature
func verifyHMAC(payload []byte, signature string, secret string) bool {
	expectedMAC := computeHMAC(payload, secret)
	return hmac.Equal([]byte(signature), []byte(expectedMAC))
}

// VerifyWebhookSignature verifies a webhook signature
// Useful for tests or if MIRA receives webhooks
func (m *SimpleWebhookManager) VerifyWebhookSignature(payload []byte, signatureHeader string, secret string) bool {
	if signatureHeader == "" || secret == "" {
		return false
	}

	// Format attendu: "sha256=<hex>"
	parts := strings.SplitN(signatureHeader, "=", 2)
	if len(parts) != 2 || parts[0] != "sha256" {
		return false
	}

	return verifyHMAC(payload, parts[1], secret)
}

// saveToDLQ persists a failed event to the dead-letter queue
func (m *SimpleWebhookManager) saveToDLQ(event ports.WebhookEvent) error {
	if m.db == nil {
		return nil
	}
	payloadJSON, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}
	_, err = m.db.Exec(
		`INSERT INTO webhook_dlq (id, endpoint_id, event_type, payload, attempts, failed_at)
		 VALUES (?, ?, ?, ?, 0, ?)
		 ON CONFLICT(id) DO UPDATE SET attempts = attempts + 1, failed_at = ?`,
		event.ID.String(), event.EndpointID.String(), event.Type, string(payloadJSON),
		float64(event.Timestamp.Unix()), float64(time.Now().Unix()),
	)
	return err
}

// dlqRetryLoop periodically retries DLQ events
func (m *SimpleWebhookManager) dlqRetryLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.RetryDLQ(context.Background())
		case <-m.stopChan:
			return
		}
	}
}

// RetryDLQ attempts to re-send events from the dead-letter queue
func (m *SimpleWebhookManager) RetryDLQ(ctx context.Context) {
	if m.db == nil {
		return
	}

	rows, err := m.db.QueryContext(ctx,
		`SELECT id, endpoint_id, event_type, payload, attempts FROM webhook_dlq WHERE attempts < 3 ORDER BY failed_at ASC LIMIT 100`)
	if err != nil {
		log.Printf("[Webhook] DLQ query failed: %v", err)
		return
	}
	defer rows.Close()

	type dlqEvent struct {
		id         string
		endpointID string
		eventType  string
		payload    string
		attempts   int
	}

	var events []dlqEvent
	for rows.Next() {
		var e dlqEvent
		if err := rows.Scan(&e.id, &e.endpointID, &e.eventType, &e.payload, &e.attempts); err != nil {
			continue
		}
		events = append(events, e)
	}

	for _, e := range events {
		endpointID, err := uuid.Parse(e.endpointID)
		if err != nil {
			continue
		}

		m.mu.RLock()
		endpoint, ok := m.endpoints[endpointID]
		m.mu.RUnlock()

		if !ok || !endpoint.Active {
			continue
		}

		var payload map[string]interface{}
		_ = json.Unmarshal([]byte(e.payload), &payload)

		event := ports.WebhookEvent{
			ID:         uuid.MustParse(e.id),
			EndpointID: endpointID,
			Type:       e.eventType,
			Payload:    payload,
			Timestamp:  time.Now(),
		}

		select {
		case m.queue <- event:
			// Re-queued successfully, delete from DLQ
			_, _ = m.db.ExecContext(ctx, `DELETE FROM webhook_dlq WHERE id = ?`, e.id)
		default:
			// Still can't queue, increment attempts
			_, _ = m.db.ExecContext(ctx,
				`UPDATE webhook_dlq SET attempts = attempts + 1, failed_at = ? WHERE id = ?`,
				float64(time.Now().Unix()), e.id)
		}
	}
}

// Ensure interface is implemented
var _ ports.WebhookManager = (*SimpleWebhookManager)(nil)
