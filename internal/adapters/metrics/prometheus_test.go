package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"context"
)

func TestNewPrometheusCollector(t *testing.T) {
	pc := NewPrometheusCollector()
	if pc == nil {
		t.Fatal("NewPrometheusCollector returned nil")
	}
}

func TestPrometheusCollector_IsEnabled(t *testing.T) {
	pc := NewPrometheusCollector()
	if !pc.IsEnabled() {
		t.Error("Expected IsEnabled to return true")
	}
}

func TestPrometheusCollector_RecordOperations(t *testing.T) {
	pc := NewPrometheusCollector()

	// Enregistrer quelques métriques
	pc.RecordStore(100 * time.Millisecond)
	pc.RecordRecall(50 * time.Millisecond)
	pc.RecordSearch(10*time.Millisecond, true)
	pc.RecordSearch(15*time.Millisecond, false)
	pc.RecordEmbed(20 * time.Millisecond)
	pc.RecordError()
	pc.UpdateMemoryCount(100)
	pc.UpdateVectorCount(50)
}

func TestPrometheusCollector_GetReport(t *testing.T) {
	pc := NewPrometheusCollector()

	report := pc.GetReport(context.Background())
	if report.Timestamp.IsZero() {
		t.Error("Expected non-zero timestamp in report")
	}
}

func TestPrometheusCollector_Handler(t *testing.T) {
	pc := NewPrometheusCollector()

	// Enregistrer une métrique
	pc.RecordStore(100 * time.Millisecond)

	// Créer une requête
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()

	// Appeler le handler
	pc.Handler().ServeHTTP(rec, req)

	// Vérifier le code
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Vérifier le contenu
	body := rec.Body.String()
	if !strings.Contains(body, "mira_store_duration_seconds") {
		t.Error("Expected metrics to contain mira_store_duration_seconds")
	}
	if !strings.Contains(body, "mira_store_total") {
		t.Error("Expected metrics to contain mira_store_total")
	}
	if !strings.Contains(body, "# HELP") {
		t.Error("Expected metrics to contain HELP comments")
	}
	if !strings.Contains(body, "# TYPE") {
		t.Error("Expected metrics to contain TYPE comments")
	}
}

func TestPrometheusCollector_Handler_ContainsAllMetrics(t *testing.T) {
	pc := NewPrometheusCollector()

	// Enregistrer toutes les métriques
	pc.RecordStore(100 * time.Millisecond)
	pc.RecordRecall(50 * time.Millisecond)
	pc.RecordSearch(10*time.Millisecond, true)
	pc.RecordEmbed(20 * time.Millisecond)
	pc.RecordError()
	pc.UpdateMemoryCount(100)
	pc.UpdateVectorCount(50)

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	pc.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Vérifier que toutes les métriques sont présentes
	expectedMetrics := []string{
		"mira_store_duration_seconds",
		"mira_recall_duration_seconds",
		"mira_search_duration_seconds",
		"mira_embed_duration_seconds",
		"mira_store_total",
		"mira_recall_total",
		"mira_search_total",
		"mira_errors_total",
		"mira_memory_count",
		"mira_vector_count",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("Expected metrics to contain %s", metric)
		}
	}
}
