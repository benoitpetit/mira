// Webhook ports
package ports

import (
	"context"
	"time"

	"github.com/benoitpetit/mira/internal/util"
	"github.com/google/uuid"
)

// WebhookEvent represents a webhook event
type WebhookEvent struct {
	ID         uuid.UUID
	EndpointID uuid.UUID
	Type       string
	Payload    map[string]interface{}
	Timestamp  time.Time
}

// WebhookEndpoint represents a registered webhook endpoint
type WebhookEndpoint struct {
	ID        uuid.UUID
	URL       string
	Events    []string
	Secret    string
	Active    bool
	CreatedAt time.Time
	CB        *util.CircuitBreaker // Circuit breaker for this endpoint
}

// WebhookStats contains webhook statistics
type WebhookStats struct {
	RegisteredCount int
	QueueDepth      int
}

// WebhookManager defines the interface for webhook management
type WebhookManager interface {
	IsEnabled() bool
	Start()
	Stop()
	Register(ctx context.Context, url string, events []string, secret string) *WebhookEndpoint
	Unregister(ctx context.Context, id uuid.UUID)
	ListWebhooks(ctx context.Context) []*WebhookEndpoint
	GetStats(ctx context.Context) WebhookStats
	Trigger(ctx context.Context, eventType string, payload map[string]interface{})
}
