// Metrics ports
package ports

import (
	"context"
	"time"
)

// MetricsCollector defines the interface for metrics collection
type MetricsCollector interface {
	IsEnabled() bool
	RecordStore(duration time.Duration)
	RecordRecall(duration time.Duration)
	RecordSearch(duration time.Duration, usedHNSW bool)
	RecordEmbed(duration time.Duration)
	GetReport(ctx context.Context) MetricsReport
}

// HistogramStats contains histogram statistics
type HistogramStats struct {
	P50  float64
	P90  float64
	P95  float64
	P99  float64
	Mean float64
}

// MetricsReport contains all metrics
type MetricsReport struct {
	Timestamp      time.Time
	Uptime         time.Duration
	StoreOps       int64
	RecallOps      int64
	StoreLatency   HistogramStats
	RecallLatency  HistogramStats
	SearchLatency  HistogramStats
	EmbedLatency   HistogramStats
	ActiveSearches float64
	PendingEmbeds  float64
	QueueDepth     float64
	CacheHitRate   float64
	HNSWHitRate    float64
}
