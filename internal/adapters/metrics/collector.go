// Metrics collector adapter - implements ports.MetricsCollector
package metrics

import (
	"context"
	"sync"
	"time"

	"github.com/benoitpetit/mira/internal/usecases/ports"
)

// SimpleMetricsCollector implements a basic metrics collector
type SimpleMetricsCollector struct {
	mu sync.RWMutex

	storeCount     int64
	storeDuration  time.Duration
	storeLast      time.Time
	recallCount    int64
	recallDuration time.Duration
	recallLast     time.Time
	searchCount    int64
	searchDuration time.Duration
	searchLast     time.Time
	embedCount     int64
	embedDuration  time.Duration
	embedLast      time.Time
	startTime      time.Time
}

// NewSimpleMetricsCollector creates a new simple metrics collector
func NewSimpleMetricsCollector() *SimpleMetricsCollector {
	return &SimpleMetricsCollector{
		startTime: time.Now(),
	}
}

// IsEnabled returns whether metrics are enabled
func (m *SimpleMetricsCollector) IsEnabled() bool {
	return true
}

// RecordStore records a store operation
func (m *SimpleMetricsCollector) RecordStore(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.storeCount++
	m.storeDuration += duration
	m.storeLast = time.Now()
}

// RecordRecall records a recall operation
func (m *SimpleMetricsCollector) RecordRecall(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recallCount++
	m.recallDuration += duration
	m.recallLast = time.Now()
}

// RecordSearch records a search operation
func (m *SimpleMetricsCollector) RecordSearch(duration time.Duration, usedHNSW bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.searchCount++
	m.searchDuration += duration
	m.searchLast = time.Now()
}

// RecordEmbed records an embedding operation
func (m *SimpleMetricsCollector) RecordEmbed(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.embedCount++
	m.embedDuration += duration
	m.embedLast = time.Now()
}

// GetReport returns a metrics report
func (m *SimpleMetricsCollector) GetReport(ctx context.Context) ports.MetricsReport {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return ports.MetricsReport{
		Timestamp: time.Now(),
		Uptime:    time.Since(m.startTime),
		StoreOps:  m.storeCount,
		RecallOps: m.recallCount,
		StoreLatency: ports.HistogramStats{
			Mean: avgDuration(m.storeDuration, m.storeCount),
		},
		RecallLatency: ports.HistogramStats{
			Mean: avgDuration(m.recallDuration, m.recallCount),
		},
		SearchLatency: ports.HistogramStats{
			Mean: avgDuration(m.searchDuration, m.searchCount),
		},
		EmbedLatency: ports.HistogramStats{
			Mean: avgDuration(m.embedDuration, m.embedCount),
		},
	}
}

func avgDuration(total time.Duration, count int64) float64 {
	if count == 0 {
		return 0
	}
	return float64(total.Milliseconds()) / float64(count)
}

// Ensure interface is implemented
var _ ports.MetricsCollector = (*SimpleMetricsCollector)(nil)
