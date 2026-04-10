package metrics

import (
	"context"
	"testing"
	"time"
)

func TestNewSimpleMetricsCollector(t *testing.T) {
	m := NewSimpleMetricsCollector()

	if m == nil {
		t.Fatal("NewSimpleMetricsCollector returned nil")
	}
	if !m.IsEnabled() {
		t.Error("Metrics should be enabled by default")
	}
}

func TestRecordStore(t *testing.T) {
	m := NewSimpleMetricsCollector()

	m.RecordStore(100 * time.Millisecond)
	m.RecordStore(200 * time.Millisecond)

	report := m.GetReport(context.Background())
	if report.StoreOps != 2 {
		t.Errorf("StoreOps = %d, want 2", report.StoreOps)
	}
	if report.StoreLatency.Mean != 150.0 {
		t.Errorf("StoreLatency.Mean = %f, want 150.0", report.StoreLatency.Mean)
	}
}

func TestRecordRecall(t *testing.T) {
	m := NewSimpleMetricsCollector()

	m.RecordRecall(50 * time.Millisecond)
	m.RecordRecall(150 * time.Millisecond)
	m.RecordRecall(100 * time.Millisecond)

	report := m.GetReport(context.Background())
	if report.RecallOps != 3 {
		t.Errorf("RecallOps = %d, want 3", report.RecallOps)
	}
	expectedMean := 100.0 // (50 + 150 + 100) / 3
	if report.RecallLatency.Mean != expectedMean {
		t.Errorf("RecallLatency.Mean = %f, want %f", report.RecallLatency.Mean, expectedMean)
	}
}

func TestRecordSearch(t *testing.T) {
	m := NewSimpleMetricsCollector()

	m.RecordSearch(10*time.Millisecond, true)
	m.RecordSearch(20*time.Millisecond, false)

	report := m.GetReport(context.Background())
	if report.SearchLatency.Mean != 15.0 {
		t.Errorf("SearchLatency.Mean = %f, want 15.0", report.SearchLatency.Mean)
	}
}

func TestRecordEmbed(t *testing.T) {
	m := NewSimpleMetricsCollector()

	m.RecordEmbed(500 * time.Millisecond)

	report := m.GetReport(context.Background())
	if report.EmbedLatency.Mean != 500.0 {
		t.Errorf("EmbedLatency.Mean = %f, want 500.0", report.EmbedLatency.Mean)
	}
}

func TestGetReportUptime(t *testing.T) {
	m := NewSimpleMetricsCollector()
	time.Sleep(10 * time.Millisecond)

	report := m.GetReport(context.Background())
	if report.Uptime < time.Millisecond {
		t.Error("Uptime should be positive")
	}
}

func TestGetReportTimestamp(t *testing.T) {
	m := NewSimpleMetricsCollector()
	before := time.Now()

	report := m.GetReport(context.Background())
	after := time.Now()

	if report.Timestamp.Before(before) || report.Timestamp.After(after) {
		t.Error("Timestamp should be current time")
	}
}

func TestAvgDuration(t *testing.T) {
	tests := []struct {
		total    time.Duration
		count    int64
		expected float64
	}{
		{100 * time.Millisecond, 2, 50.0},
		{0, 0, 0.0},
		{100 * time.Millisecond, 0, 0.0},
		{time.Second, 1, 1000.0},
	}

	for _, tt := range tests {
		result := avgDuration(tt.total, tt.count)
		if result != tt.expected {
			t.Errorf("avgDuration(%v, %d) = %f, want %f", tt.total, tt.count, result, tt.expected)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	m := NewSimpleMetricsCollector()

	// Run concurrent operations
	done := make(chan bool, 10)
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				m.RecordStore(time.Millisecond)
				m.RecordRecall(time.Millisecond)
				m.RecordSearch(time.Millisecond, true)
			}
			done <- true
		}()
	}

	// Also read concurrently
	go func() {
		for i := 0; i < 100; i++ {
			_ = m.GetReport(context.Background())
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 6; i++ {
		<-done
	}

	report := m.GetReport(context.Background())
	if report.StoreOps != 500 {
		t.Errorf("StoreOps = %d, want 500", report.StoreOps)
	}
	if report.RecallOps != 500 {
		t.Errorf("RecallOps = %d, want 500", report.RecallOps)
	}
	if report.SearchLatency.Mean != 1.0 {
		t.Errorf("SearchLatency.Mean = %f, want 1.0", report.SearchLatency.Mean)
	}
}

func BenchmarkRecordStore(b *testing.B) {
	m := NewSimpleMetricsCollector()
	duration := time.Millisecond

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordStore(duration)
	}
}

func BenchmarkGetReport(b *testing.B) {
	m := NewSimpleMetricsCollector()
	// Pre-populate with some data
	for i := 0; i < 1000; i++ {
		m.RecordStore(time.Millisecond)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.GetReport(context.Background())
	}
}
