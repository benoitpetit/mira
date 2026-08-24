// LoCoMo-style long-conversation memory benchmark for MIRA's CBA recall path.
//
// LoCoMo (Long Conversation Memory) is a public benchmark methodology for
// evaluating long-term memory systems across many sessions of conversation
// with sparse, far-apart facts. This file does not ship the original LoCoMo
// dataset (not redistributed here); instead it generates a structurally
// equivalent synthetic corpus (many sessions, sparse relevant facts, mostly
// noise) and measures MIRA's own recall latency distribution (p50/p95/p99)
// end-to-end through the real CBA algorithm (RecallMemory.Execute), using the
// same in-memory test doubles as the rest of this package (no network, no
// LLM calls — consistent with MIRA's zero-LLM-call memory management claim).
//
// Mem0/Zep are not run here (no local harness for third-party services in
// this repo); reportLocoMoBenchmark() leaves their fields as "unmeasured" so
// numbers are never fabricated. Fill them in from your own head-to-head runs
// via -mira.compare=path/to/baselines.json (see BaselineResult).
package interactors

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
)

var compareBaselinesPath = flag.String("mira.compare", "", "optional path to a JSON file with third-party baseline numbers (Mem0/Zep) to include in the LoCoMo report")

// BaselineResult holds externally-measured numbers for a competing system.
// Populate this yourself (e.g. from your own Mem0/Zep benchmark run) and pass
// it via -mira.compare; MIRA's own numbers are always measured locally.
type BaselineResult struct {
	Name                 string  `json:"name"`
	P95LatencyMs         float64 `json:"p95_latency_ms"`
	ExtractionCostUSD     float64 `json:"extraction_cost_usd_per_interaction"`
	Notes                string  `json:"notes,omitempty"`
}

// LoCoMoReport is the JSON artifact produced by BenchmarkLoCoMoRecall.
type LoCoMoReport struct {
	GeneratedAt    time.Time         `json:"generated_at"`
	Sessions       int               `json:"sessions"`
	FactsPerSession int              `json:"facts_per_session"`
	NoiseRatio     float64           `json:"noise_ratio"`
	Queries        int               `json:"queries"`
	MIRA           LoCoMoMetrics     `json:"mira"`
	Baselines      []BaselineResult  `json:"baselines,omitempty"`
}

// LoCoMoMetrics captures MIRA's measured performance for the run.
type LoCoMoMetrics struct {
	P50LatencyMs      float64 `json:"p50_latency_ms"`
	P95LatencyMs      float64 `json:"p95_latency_ms"`
	P99LatencyMs      float64 `json:"p99_latency_ms"`
	AvgLatencyMs      float64 `json:"avg_latency_ms"`
	ExtractionCostUSD float64 `json:"extraction_cost_usd_per_interaction"` // always 0: no LLM call for memory management
}

// genLoCoMoCorpus builds a synthetic long-conversation memory corpus:
// `sessions` sessions, each contributing `factsPerSession` candidate memories,
// where a `noiseRatio` fraction score low (irrelevant chatter) and the rest
// score high (facts a probe query should recall), spread across old
// timestamps to emulate long-range recall pressure.
func genLoCoMoCorpus(sessions, factsPerSession int, noiseRatio float64) []*entities.Candidate {
	rng := rand.New(rand.NewSource(42)) // deterministic corpus
	now := time.Now()
	candidates := make([]*entities.Candidate, 0, sessions*factsPerSession)
	for s := 0; s < sessions; s++ {
		sessionAge := time.Duration(sessions-s) * 24 * time.Hour
		for f := 0; f < factsPerSession; f++ {
			isNoise := rng.Float64() < noiseRatio
			score := 0.85 + rng.Float64()*0.1
			tokens := 40 + rng.Intn(120)
			if isNoise {
				score = 0.1 + rng.Float64()*0.3
			}
			id := fmt.Sprintf("s%d-f%d", s, f)
			candidates = append(candidates, createTestCandidateWithScore(id, now.Add(-sessionAge), score, tokens))
		}
	}
	return candidates
}

// BenchmarkLoCoMoRecall measures MIRA's CBA recall latency distribution over
// a synthetic LoCoMo-style corpus (200 sessions x 20 facts/session = 4000
// candidate memories, 70% noise), then writes a JSON report to stdout (via
// -mira.report) so numbers can be compared against externally-measured
// Mem0/Zep baselines.
func BenchmarkLoCoMoRecall(b *testing.B) {
	const sessions = 200
	const factsPerSession = 20
	const noiseRatio = 0.7

	candidates := genLoCoMoCorpus(sessions, factsPerSession, noiseRatio)
	interactor := createTestInteractor(candidates)
	ctx := context.Background()

	durations := make([]time.Duration, 0, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		_, err := interactor.Execute(ctx, RecallMemoryInput{
			Query:  "What decision did we make about the database in an earlier session?",
			Budget: 2000,
		})
		elapsed := time.Since(start)
		if err != nil {
			b.Fatalf("recall failed: %v", err)
		}
		durations = append(durations, elapsed)
	}
	b.StopTimer()

	metrics := computeLatencyMetrics(durations)

	report := LoCoMoReport{
		GeneratedAt:     time.Now(),
		Sessions:        sessions,
		FactsPerSession: factsPerSession,
		NoiseRatio:      noiseRatio,
		Queries:         b.N,
		MIRA:            metrics,
	}
	if *compareBaselinesPath != "" {
		if data, err := os.ReadFile(*compareBaselinesPath); err == nil {
			_ = json.Unmarshal(data, &report.Baselines)
		}
	}

	data, _ := json.MarshalIndent(report, "", "  ")
	b.Logf("LoCoMo benchmark report:\n%s", data)
	if reportPath := os.Getenv("MIRA_LOCOMO_REPORT"); reportPath != "" {
		_ = os.WriteFile(reportPath, data, 0o644)
	}
}

func computeLatencyMetrics(durations []time.Duration) LoCoMoMetrics {
	if len(durations) == 0 {
		return LoCoMoMetrics{}
	}
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	percentile := func(p float64) float64 {
		idx := int(p * float64(len(sorted)))
		if idx >= len(sorted) {
			idx = len(sorted) - 1
		}
		return float64(sorted[idx]) / float64(time.Millisecond)
	}

	var sum time.Duration
	for _, d := range sorted {
		sum += d
	}

	return LoCoMoMetrics{
		P50LatencyMs:      percentile(0.50),
		P95LatencyMs:      percentile(0.95),
		P99LatencyMs:      percentile(0.99),
		AvgLatencyMs:      float64(sum) / float64(len(sorted)) / float64(time.Millisecond),
		ExtractionCostUSD: 0, // MIRA never calls an LLM to manage memory
	}
}
