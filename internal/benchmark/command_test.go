package benchmark

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBenchmarkCommand(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		var out bytes.Buffer
		if err := RunBenchmarkCommand(context.Background(), []string{"--help"}, &out, &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "mira-benchmark run") {
			t.Fatalf("help output = %q", out.String())
		}
	})
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing config", []string{"run", "--track", "quality", "--output", "x.json"}, "--config is required"},
		{"unknown track", []string{"run", "--track", "rank", "--config", "x", "--output", "x.json"}, "unknown track"},
		{"invalid repetitions", []string{"run", "--track", "quality", "--config", "x", "--output", "x.json", "--repetitions", "0"}, "repetitions must be positive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			err := RunBenchmarkCommand(context.Background(), tt.args, &out, &errOut)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want contains %q", err, tt.want)
			}
		})
	}
}

func TestBenchmarkCommandWritesExplicitUnavailableQualityTrack(t *testing.T) {
	t.Setenv("MIRA_BENCH_MODEL_DIR", "")
	dir := t.TempDir()
	tempRoot := filepath.Join(dir, "temp-root")
	t.Setenv("MIRA_BENCH_TMP_DIR", tempRoot)
	configPath := filepath.Join(dir, "benchmark.json")
	if err := os.WriteFile(configPath, []byte(`{"corpus_sizes":[100],"backend":"sqlite"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(dir, "report.json")
	var out, errOut bytes.Buffer
	err := RunBenchmarkCommand(context.Background(), []string{"run", "--track", "quality", "--config", configPath, "--output", outputPath, "--warmups", "0", "--repetitions", "1"}, &out, &errOut)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	report, readErr := LoadReport(file)
	closeErr := file.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if report.Quality.Status != TrackUnavailable || report.Quality.UnavailableReason == "" {
		t.Fatalf("missing-model state is not explicit: %+v", report.Quality)
	}
	if err := ValidateReport(report); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(tempRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("benchmark workspace was not cleaned up: %v", entries)
	}
}

func TestValidateDisposableDatabase(t *testing.T) {
	for _, raw := range []string{"postgres://user:pass@localhost:5432/mira_test", "postgresql://localhost/mira_test?sslmode=disable"} {
		if err := ValidateDisposableDatabase(raw); err != nil {
			t.Errorf("valid disposable URL %q rejected: %v", raw, err)
		}
	}
	for _, raw := range []string{"", "postgres://localhost/mira", "sqlite:///tmp/mira_test", "postgres://localhost/"} {
		if err := ValidateDisposableDatabase(raw); err == nil {
			t.Errorf("unsafe database URL %q accepted", raw)
		}
	}
}

func TestExportSiteSnapshotChecksumProvenance(t *testing.T) {
	report := validReport()
	var reportBytes bytes.Buffer
	if err := WriteReport(&reportBytes, report); err != nil {
		t.Fatal(err)
	}
	var first, second bytes.Buffer
	if err := ExportSiteSnapshot(bytes.NewReader(reportBytes.Bytes()), "https://github.com/example/mira/blob/0123456789abcdef0123456789abcdef01234567/benchmarks/results/run-1.json", &first); err != nil {
		t.Fatal(err)
	}
	if err := ExportSiteSnapshot(bytes.NewReader(reportBytes.Bytes()), "https://github.com/example/mira/blob/0123456789abcdef0123456789abcdef01234567/benchmarks/results/run-1.json", &second); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("site export is not deterministic")
	}
	var snapshot SiteSnapshot
	if err := json.Unmarshal(first.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.SourceReportSHA256 == "" || snapshot.SourceCommit != report.Source.Commit || snapshot.SourceURL == "" {
		t.Fatalf("missing snapshot provenance: %+v", snapshot)
	}
	sum := sha256.Sum256(reportBytes.Bytes())
	if snapshot.SourceReportSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("source checksum = %s, want %x", snapshot.SourceReportSHA256, sum)
	}
	bad := bytes.NewReader([]byte(strings.Replace(reportBytes.String(), `"dirty": false`, `"dirty": true`, 1)))
	var rejected bytes.Buffer
	if err := ExportSiteSnapshot(bad, "https://example.com/report.json", &rejected); err == nil {
		t.Fatal("dirty/mismatched official report exported")
	}
}
