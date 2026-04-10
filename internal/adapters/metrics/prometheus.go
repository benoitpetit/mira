// Prometheus metrics adapter - implements ports.MetricsCollector
package metrics

import (
	"context"
	"net/http"
	"time"

	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusCollector expose les métriques au format Prometheus
type PrometheusCollector struct {
	// Métriques de durée
	storeDuration  prometheus.Histogram
	recallDuration prometheus.Histogram
	searchDuration prometheus.Histogram
	embedDuration  prometheus.Histogram

	// Métriques de comptage
	storeTotal  prometheus.Counter
	recallTotal prometheus.Counter
	searchTotal prometheus.Counter
	errorsTotal prometheus.Counter

	// Métriques de jauge (état actuel)
	memoryCount prometheus.Gauge
	vectorCount prometheus.Gauge

	// Registre Prometheus
	registry *prometheus.Registry
}

// NewPrometheusCollector crée un nouveau collecteur Prometheus
func NewPrometheusCollector() *PrometheusCollector {
	registry := prometheus.NewRegistry()

	pc := &PrometheusCollector{
		registry: registry,

		storeDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "mira_store_duration_seconds",
			Help:    "Duration of store operations",
			Buckets: prometheus.DefBuckets,
		}),

		recallDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "mira_recall_duration_seconds",
			Help:    "Duration of recall operations",
			Buckets: prometheus.DefBuckets,
		}),

		searchDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "mira_search_duration_seconds",
			Help:    "Duration of vector search operations",
			Buckets: prometheus.DefBuckets,
		}),

		embedDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "mira_embed_duration_seconds",
			Help:    "Duration of embedding operations",
			Buckets: prometheus.DefBuckets,
		}),

		storeTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mira_store_total",
			Help: "Total number of store operations",
		}),

		recallTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mira_recall_total",
			Help: "Total number of recall operations",
		}),

		searchTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mira_search_total",
			Help: "Total number of search operations",
		}),

		errorsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mira_errors_total",
			Help: "Total number of errors",
		}),

		memoryCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "mira_memory_count",
			Help: "Current number of memories in the database",
		}),

		vectorCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "mira_vector_count",
			Help: "Current number of vectors in the index",
		}),
	}

	// Enregistrer toutes les métriques
	registry.MustRegister(
		pc.storeDuration,
		pc.recallDuration,
		pc.searchDuration,
		pc.embedDuration,
		pc.storeTotal,
		pc.recallTotal,
		pc.searchTotal,
		pc.errorsTotal,
		pc.memoryCount,
		pc.vectorCount,
	)

	return pc
}

// IsEnabled returns whether metrics are enabled
func (pc *PrometheusCollector) IsEnabled() bool {
	return true
}

// RecordStore records a store operation
func (pc *PrometheusCollector) RecordStore(duration time.Duration) {
	pc.storeDuration.Observe(duration.Seconds())
	pc.storeTotal.Inc()
}

// RecordRecall records a recall operation
func (pc *PrometheusCollector) RecordRecall(duration time.Duration) {
	pc.recallDuration.Observe(duration.Seconds())
	pc.recallTotal.Inc()
}

// RecordSearch records a search operation
func (pc *PrometheusCollector) RecordSearch(duration time.Duration, usedHNSW bool) {
	pc.searchDuration.Observe(duration.Seconds())
	pc.searchTotal.Inc()
}

// RecordEmbed records an embedding operation
func (pc *PrometheusCollector) RecordEmbed(duration time.Duration) {
	pc.embedDuration.Observe(duration.Seconds())
}

// RecordError records an error
func (pc *PrometheusCollector) RecordError() {
	pc.errorsTotal.Inc()
}

// UpdateMemoryCount updates the memory count gauge
func (pc *PrometheusCollector) UpdateMemoryCount(count int) {
	pc.memoryCount.Set(float64(count))
}

// UpdateVectorCount updates the vector count gauge
func (pc *PrometheusCollector) UpdateVectorCount(count int) {
	pc.vectorCount.Set(float64(count))
}

// GetReport returns a metrics report
func (pc *PrometheusCollector) GetReport(ctx context.Context) ports.MetricsReport {
	return ports.MetricsReport{
		Timestamp: time.Now(),
	}
}

// Handler retourne le handler HTTP pour les métriques
func (pc *PrometheusCollector) Handler() http.Handler {
	return promhttp.HandlerFor(pc.registry, promhttp.HandlerOpts{})
}

// StartServer démarre un serveur HTTP pour exposer les métriques
func (pc *PrometheusCollector) StartServer(addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", pc.Handler())

	return http.ListenAndServe(addr, mux)
}

// Ensure interface is implemented
var _ ports.MetricsCollector = (*PrometheusCollector)(nil)
