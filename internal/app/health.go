// Package app provides the application composition root and lifecycle management.
// Health check endpoint for MIRA service
package app

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/benoitpetit/mira/internal/adapters/vector"
)

// HealthStatus represents the health status of the system
type HealthStatus struct {
	Status    string                 `json:"status"` // "healthy", "degraded", "unhealthy"
	Timestamp time.Time              `json:"timestamp"`
	Version   string                 `json:"version"`
	Checks    map[string]HealthCheck `json:"checks"`
}

// HealthCheck represents the status of a component
type HealthCheck struct {
	Status  string `json:"status"` // "pass", "fail", "warn"
	Message string `json:"message,omitempty"`
}

// HealthChecker checks the health of the application
type HealthChecker struct {
	app       *Application
	version   string
	startTime time.Time
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(app *Application, version string) *HealthChecker {
	return &HealthChecker{
		app:       app,
		version:   version,
		startTime: time.Now(),
	}
}

// Check checks the overall health status
func (h *HealthChecker) Check(ctx context.Context) HealthStatus {
	status := HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now(),
		Version:   h.version,
		Checks:    make(map[string]HealthCheck),
	}

	// Check database
	dbCheck := h.checkDatabase(ctx)
	status.Checks["database"] = dbCheck
	if dbCheck.Status == "fail" {
		status.Status = "unhealthy"
	}

	// Check vector store
	vectorCheck := h.checkVectorStore()
	status.Checks["vector_store"] = vectorCheck
	if vectorCheck.Status == "fail" && status.Status == "healthy" {
		status.Status = "degraded"
	}

	// Check embedder
	embedderCheck := h.checkEmbedder(ctx)
	status.Checks["embedder"] = embedderCheck
	if embedderCheck.Status == "fail" && status.Status == "healthy" {
		status.Status = "degraded"
	}

	return status
}

func (h *HealthChecker) checkDatabase(ctx context.Context) HealthCheck {
	if h.app.repository == nil {
		return HealthCheck{Status: "fail", Message: "repository not initialized"}
	}

	// Try to get stats
	_, err := h.app.repository.GetStats(ctx)
	if err != nil {
		return HealthCheck{Status: "fail", Message: err.Error()}
	}

	return HealthCheck{Status: "pass"}
}

func (h *HealthChecker) checkVectorStore() HealthCheck {
	if h.app.vectorStore == nil {
		return HealthCheck{Status: "fail", Message: "vector store not initialized"}
	}

	// Check if HNSW is ready (if it's an HNSWStore)
	if h.app.hnswIndex != nil && !h.app.hnswIndex.IsReady() {
		return HealthCheck{Status: "warn", Message: "HNSW index not ready"}
	}

	return HealthCheck{Status: "pass"}
}

func (h *HealthChecker) checkEmbedder(ctx context.Context) HealthCheck {
	if h.app.embedder == nil {
		return HealthCheck{Status: "fail", Message: "embedder not initialized"}
	}

	// Test a simple encoding
	_, err := h.app.embedder.Encode(ctx, "test")
	if err != nil {
		return HealthCheck{Status: "fail", Message: err.Error()}
	}

	return HealthCheck{Status: "pass"}
}

// Handler returns an http.Handler for the health check
func (h *HealthChecker) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := h.Check(r.Context())

		// Determine HTTP code
		code := http.StatusOK
		if status.Status == "unhealthy" {
			code = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(status)
	})
}

// LivenessHandler returns a simple handler for Kubernetes liveness probes
func (h *HealthChecker) LivenessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "alive",
		})
	})
}

// ReadinessHandler returns a handler for Kubernetes readiness probes
func (h *HealthChecker) ReadinessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := h.Check(r.Context())

		// For readiness, we consider both "healthy" and "degraded" as ready
		code := http.StatusOK
		if status.Status == "unhealthy" {
			code = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   status.Status,
			"ready":    status.Status != "unhealthy",
			"checks":   status.Checks,
		})
	})
}

// RunWithHealthCheck starts the application with a health check HTTP server
func (a *Application) RunWithHealthCheck(addr string) error {
	healthChecker := NewHealthChecker(a, a.config.MCP.Version)

	mux := http.NewServeMux()
	mux.Handle("/health", healthChecker.Handler())
	mux.Handle("/health/live", healthChecker.LivenessHandler())
	mux.Handle("/health/ready", healthChecker.ReadinessHandler())

	return http.ListenAndServe(addr, mux)
}

// Health returns the current health status
func (a *Application) Health(ctx context.Context) HealthStatus {
	checker := NewHealthChecker(a, a.config.MCP.Version)
	return checker.Check(ctx)
}

// IsHNSWReady returns true if the HNSW index is ready
func (a *Application) IsHNSWReady() bool {
	if a.hnswIndex == nil {
		return false
	}
	return a.hnswIndex.IsReady()
}

// GetVectorStoreStats returns statistics about the vector store
func (a *Application) GetVectorStoreStats() map[string]interface{} {
	stats := map[string]interface{}{
		"type": "unknown",
	}

	if a.vectorStore == nil {
		stats["status"] = "not_initialized"
		return stats
	}

	// Check if it's an HNSWStore
	if a.hnswIndex != nil {
		stats["type"] = "hnsw"
		stats["ready"] = a.hnswIndex.IsReady()
		stats["count"] = a.hnswIndex.Stats()
	} else {
		stats["type"] = "sqlite"
	}

	return stats
}

// Ensure HNSWStore has the methods we need
var _ interface {
	IsReady() bool
	Stats() int
} = (*vector.HNSWStore)(nil)
