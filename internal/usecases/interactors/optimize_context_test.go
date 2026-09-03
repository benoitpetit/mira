package interactors

import (
	"math"
	"testing"
)

func TestMeasureContextEfficiency(t *testing.T) {
	messages := []ContextMessage{
		{Role: "system", Content: "Follow the deployment runbook."},
		{Role: "assistant", Content: "The primary database is PostgreSQL."},
	}
	metric := MeasureContextEfficiency(messages, []string{
		"deployment   runbook",
		"primary database is postgresql",
		"the cache is Redis",
	}, 200)

	if metric.RequiredAssertions != 3 || metric.RetainedAssertions != 2 {
		t.Fatalf("assertions = %d/%d, want 2/3", metric.RetainedAssertions, metric.RequiredAssertions)
	}
	if math.Abs(metric.CoveragePercent-100.0*2.0/3.0) > 1e-9 {
		t.Errorf("coverage = %v, want %v", metric.CoveragePercent, 100.0*2.0/3.0)
	}
	if math.Abs(metric.CoveragePer1KTokens-(100.0*2.0/3.0)/200*1000) > 1e-9 {
		t.Errorf("coverage per 1K = %v", metric.CoveragePer1KTokens)
	}
}

func TestMeasureContextEfficiencyWithoutAssertions(t *testing.T) {
	metric := MeasureContextEfficiency(nil, nil, 0)
	if metric.RequiredAssertions != 0 || metric.RetainedAssertions != 0 || metric.CoveragePercent != 0 || metric.CoveragePer1KTokens != 0 {
		t.Errorf("empty metric = %#v", metric)
	}
}
