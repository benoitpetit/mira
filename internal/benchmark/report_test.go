package benchmark

import (
	"bytes"
	"math"
	"testing"
	"time"
)

func validReport() Report {
	return Report{
		SchemaVersion: ReportSchemaVersion,
		Benchmark:     BenchmarkIdentity{Name: "mira-public", Version: "1.0.0"},
		Run:           RunIdentity{ID: "run-1", StartedAtUTC: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC), Track: "all"},
		Source:        SourceIdentity{Commit: "0123456789abcdef0123456789abcdef01234567"},
		Dataset:       DatasetIdentity{Name: "mira-synthetic", Version: "1.0.0", SHA256: syntheticV1SHA256, Seed: 42, MemoryCount: 20, QueryCount: 6, JudgmentCount: 8},
		Environment:   RuntimeEnvironment{MiraVersion: "0.8.4", GoVersion: "go1.25.0", OS: "linux", Arch: "amd64", Host: "reference-host", CPUModel: "test-cpu", LogicalCPUs: 8, GOMAXPROCS: 8, StorageBackend: "sqlite", DatabaseVersion: "3.50.0"},
		Model:         ModelIdentity{Name: "sentence-transformers/all-MiniLM-L6-v2", Revision: "0123456789abcdef0123456789abcdef01234567", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		Protocol:      Protocol{Warmups: 2, Repetitions: 5, Concurrency: 1, SetupExcluded: true},
		Quality:       QualityTrack{Status: TrackAvailable, QueryCount: 6, Results: []QueryResult{{QueryID: "q1", RankedIDs: []string{"m1"}, Metrics: QualityMetrics{RecallAt5: 1, RecallAt10: 1, RecallAt20: 1, MRR: 1, NDCGAt10: 1}}, {QueryID: "q2"}, {QueryID: "q3"}, {QueryID: "q4"}, {QueryID: "q5"}, {QueryID: "q6"}}},
		Performance:   PerformanceTrack{Status: TrackAvailable, Backend: "sqlite", Concurrency: 1, Cases: []LatencyCase{{Name: "sqlite_full_recall_100", Status: TrackAvailable, Unit: "ms", Repetitions: 5, Samples: []float64{1, 2, 3, 4, 5}, Summary: LatencySummary{P50: 3, P95: 4.8, P99: 4.96, Median: 3, Spread: 4}}}},
	}
}

func TestValidateReport(t *testing.T) {
	r := validReport()
	if err := ValidateReport(r); err != nil {
		t.Fatalf("valid report rejected: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*Report)
	}{
		{"schema", func(r *Report) { r.SchemaVersion = "9" }},
		{"missing source commit", func(r *Report) { r.Source.Commit = "" }},
		{"non-finite metric", func(r *Report) { r.Quality.Results[0].Metrics.MRR = math.NaN() }},
		{"missing query result", func(r *Report) { r.Quality.Results = nil }},
		{"sample mismatch", func(r *Report) { r.Performance.Cases[0].Samples = nil }},
		{"negative latency", func(r *Report) { r.Performance.Cases[0].Samples[0] = -1 }},
		{"bad status", func(r *Report) { r.Quality.Status = "unknown" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validReport()
			tt.mutate(&r)
			if err := ValidateReport(r); err == nil {
				t.Fatal("invalid report accepted")
			}
		})
	}
}

func TestValidateOfficialRun(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Report)
	}{
		{"clean valid", func(*Report) {}},
		{"dirty source", func(r *Report) { r.Source.Dirty = true }},
		{"dataset mismatch", func(r *Report) { r.Dataset.SHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" }},
		{"missing host", func(r *Report) { r.Environment.Host = "" }},
		{"missing model", func(r *Report) { r.Model.SHA256 = "" }},
		{"missing backend", func(r *Report) { r.Environment.StorageBackend = "" }},
		{"too few repetitions", func(r *Report) {
			r.Performance.Cases[0].Repetitions = 4
			r.Protocol.Repetitions = 4
			r.Performance.Cases[0].Samples = r.Performance.Cases[0].Samples[:4]
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validReport()
			tt.mutate(&r)
			err := ValidateOfficialRun(r)
			if tt.name == "clean valid" && err != nil {
				t.Fatal(err)
			}
			if tt.name != "clean valid" && err == nil {
				t.Fatal("invalid official report accepted")
			}
		})
	}
}

func TestValidateOfficialRunAllowsSingleSampleSetupCases(t *testing.T) {
	r := validReport()
	r.Performance.Cases = append(r.Performance.Cases, LatencyCase{
		Name: "sqlite_fixture_setup_100", Status: TrackAvailable, Unit: "ms",
		Repetitions: 1, Samples: []float64{12}, Summary: summarizeLatency([]float64{12}),
	})
	if err := ValidateOfficialRun(r); err != nil {
		t.Fatalf("valid single-sample setup case rejected: %v", err)
	}
}

func TestWriteReportRoundTripAndStableFormatting(t *testing.T) {
	r := validReport()
	var first, second bytes.Buffer
	if err := WriteReport(&first, r); err != nil {
		t.Fatal(err)
	}
	if err := WriteReport(&second, r); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("WriteReport output is not stable")
	}
	decoded, err := LoadReport(bytes.NewReader(first.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateReport(decoded); err != nil {
		t.Fatal(err)
	}
}
