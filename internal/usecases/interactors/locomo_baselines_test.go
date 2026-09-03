package interactors

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadBaselineResultsRequiresMeasuredMetadata(t *testing.T) {
	dir := t.TempDir()
	invalid := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(invalid, []byte(`[{"name":"Mem0","p95_latency_ms":0,"extraction_cost_usd_per_interaction":0}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadBaselineResults(invalid); err == nil {
		t.Error("expected a placeholder baseline to be rejected")
	}

	valid := filepath.Join(dir, "valid.json")
	if err := os.WriteFile(valid, []byte(`[{"name":"Mem0","p95_latency_ms":12.5,"extraction_cost_usd_per_interaction":0.002,"dataset":"LoCoMo v1","hardware":"Apple M3, 16 GB","protocol":"200 sessions; 2k-token budget"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	results, err := loadBaselineResults(valid)
	if err != nil {
		t.Fatalf("loadBaselineResults failed: %v", err)
	}
	if len(results) != 1 || results[0].Name != "Mem0" {
		t.Errorf("results = %#v", results)
	}
}

func TestCurrentBenchmarkRuntimeDescribesLocalExecution(t *testing.T) {
	environment := currentBenchmarkRuntime()
	if environment.GOOS != runtime.GOOS || environment.GOARCH != runtime.GOARCH {
		t.Fatalf("environment = %#v, want current Go platform", environment)
	}
	if environment.GoVersion == "" || environment.CPUs < 1 {
		t.Fatalf("environment lacks reproducibility detail: %#v", environment)
	}
}
