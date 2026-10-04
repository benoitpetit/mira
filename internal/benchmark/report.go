package benchmark

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

const ReportSchemaVersion = "1"
const syntheticV1SHA256 = "2202fb02f9154531a811e227773bba5df328b7203700adbb9d7688f795c2d396"

const (
	TrackAvailable   = "available"
	TrackUnavailable = "unavailable"
)

type Report struct {
	SchemaVersion string             `json:"schema_version"`
	Benchmark     BenchmarkIdentity  `json:"benchmark"`
	Run           RunIdentity        `json:"run"`
	Source        SourceIdentity     `json:"source"`
	Dataset       DatasetIdentity    `json:"dataset"`
	Environment   RuntimeEnvironment `json:"environment"`
	Model         ModelIdentity      `json:"model"`
	Protocol      Protocol           `json:"protocol"`
	Quality       QualityTrack       `json:"quality"`
	Performance   PerformanceTrack   `json:"performance"`
}

type BenchmarkIdentity struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
type RunIdentity struct {
	ID           string    `json:"id"`
	StartedAtUTC time.Time `json:"started_at_utc"`
	Track        string    `json:"track"`
}
type SourceIdentity struct {
	Commit string `json:"commit"`
	Dirty  bool   `json:"dirty"`
}
type DatasetIdentity struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	SHA256        string `json:"sha256"`
	Seed          int64  `json:"seed"`
	MemoryCount   int    `json:"memory_count"`
	QueryCount    int    `json:"query_count"`
	JudgmentCount int    `json:"judgment_count"`
}
type RuntimeEnvironment struct {
	MiraVersion     string   `json:"mira_version"`
	GoVersion       string   `json:"go_version"`
	OS              string   `json:"os"`
	Arch            string   `json:"arch"`
	Host            string   `json:"host"`
	CPUModel        string   `json:"cpu_model"`
	LogicalCPUs     int      `json:"logical_cpus"`
	GOMAXPROCS      int      `json:"gomaxprocs"`
	BuildTags       []string `json:"build_tags,omitempty"`
	StorageBackend  string   `json:"storage_backend"`
	DatabaseVersion string   `json:"database_version"`
}
type ModelIdentity struct {
	Name     string `json:"name"`
	Revision string `json:"revision"`
	SHA256   string `json:"sha256"`
}
type Protocol struct {
	Warmups       int  `json:"warmups"`
	Repetitions   int  `json:"repetitions"`
	Concurrency   int  `json:"concurrency"`
	SetupExcluded bool `json:"setup_excluded"`
}
type QualityTrack struct {
	Status            string         `json:"status"`
	UnavailableReason string         `json:"unavailable_reason,omitempty"`
	QueryCount        int            `json:"query_count"`
	FailedQueries     int            `json:"failed_queries"`
	Metrics           QualityMetrics `json:"metrics"`
	Results           []QueryResult  `json:"results,omitempty"`
}
type QueryResult struct {
	QueryID             string         `json:"query_id"`
	RankedIDs           []string       `json:"ranked_ids"`
	Metrics             QualityMetrics `json:"metrics"`
	SelectedMemoryCount int            `json:"selected_memory_count,omitempty"`
	EstimatedTokens     int            `json:"estimated_tokens,omitempty"`
	LatencySamplesMS    []float64      `json:"latency_samples_ms,omitempty"`
	Error               string         `json:"error,omitempty"`
}
type PerformanceTrack struct {
	Status            string        `json:"status"`
	UnavailableReason string        `json:"unavailable_reason,omitempty"`
	Backend           string        `json:"backend"`
	Concurrency       int           `json:"concurrency"`
	Cases             []LatencyCase `json:"cases,omitempty"`
}
type LatencyCase struct {
	Name              string         `json:"name"`
	Status            string         `json:"status"`
	UnavailableReason string         `json:"unavailable_reason,omitempty"`
	Unit              string         `json:"unit"`
	Repetitions       int            `json:"repetitions"`
	Samples           []float64      `json:"samples"`
	Summary           LatencySummary `json:"summary"`
}
type LatencySummary struct {
	P50    float64 `json:"p50"`
	P95    float64 `json:"p95"`
	P99    float64 `json:"p99"`
	Median float64 `json:"median"`
	Spread float64 `json:"spread"`
}

var commitPattern = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)

func ValidateReport(r Report) error {
	if r.SchemaVersion != ReportSchemaVersion {
		return fmt.Errorf("schema_version must be %q", ReportSchemaVersion)
	}
	if strings.TrimSpace(r.Benchmark.Name) == "" || strings.TrimSpace(r.Benchmark.Version) == "" {
		return fmt.Errorf("benchmark name and version are required")
	}
	if strings.TrimSpace(r.Run.ID) == "" || r.Run.StartedAtUTC.IsZero() {
		return fmt.Errorf("run id and started_at_utc are required")
	}
	if r.Run.Track != "all" && r.Run.Track != "quality" && r.Run.Track != "performance" {
		return fmt.Errorf("run.track must be all, quality, or performance")
	}
	if !commitPattern.MatchString(r.Source.Commit) {
		return fmt.Errorf("source.commit must be a 40-character git SHA")
	}
	if strings.TrimSpace(r.Dataset.Name) == "" || strings.TrimSpace(r.Dataset.Version) == "" || !isSHA256(r.Dataset.SHA256) {
		return fmt.Errorf("dataset name, version, and sha256 are required")
	}
	if r.Dataset.MemoryCount < 1 || r.Dataset.QueryCount < 1 || r.Dataset.JudgmentCount < 1 {
		return fmt.Errorf("dataset counts must be positive")
	}
	if strings.TrimSpace(r.Environment.MiraVersion) == "" || strings.TrimSpace(r.Environment.GoVersion) == "" || strings.TrimSpace(r.Environment.OS) == "" || strings.TrimSpace(r.Environment.Arch) == "" {
		return fmt.Errorf("runtime version and platform provenance are required")
	}
	if r.Environment.LogicalCPUs < 1 || r.Environment.GOMAXPROCS < 1 {
		return fmt.Errorf("logical_cpus and gomaxprocs must be positive")
	}
	if r.Protocol.Warmups < 0 || r.Protocol.Repetitions < 0 || r.Protocol.Concurrency < 1 {
		return fmt.Errorf("protocol warmups/repetitions must be non-negative and concurrency positive")
	}
	if err := validateQualityTrack(r.Quality); err != nil {
		return fmt.Errorf("quality: %w", err)
	}
	if err := validatePerformanceTrack(r.Performance); err != nil {
		return fmt.Errorf("performance: %w", err)
	}
	return nil
}

func ValidateOfficialRun(r Report) error {
	if err := ValidateReport(r); err != nil {
		return err
	}
	if r.Source.Dirty {
		return fmt.Errorf("official run requires a clean source tree")
	}
	if r.Dataset.Name != "mira-synthetic" || r.Dataset.Version != "1.0.0" || r.Dataset.SHA256 != syntheticV1SHA256 {
		return fmt.Errorf("official run dataset identity does not match checked-in synthetic-v1 fixture")
	}
	if r.Environment.Host == "" || r.Environment.CPUModel == "" {
		return fmt.Errorf("official run requires host and CPU model identity")
	}
	if r.Environment.StorageBackend == "" || r.Environment.DatabaseVersion == "" {
		return fmt.Errorf("official run requires storage backend and database version")
	}
	if r.Model.Name != "sentence-transformers/all-MiniLM-L6-v2" || !commitPattern.MatchString(r.Model.Revision) || !isSHA256(r.Model.SHA256) {
		return fmt.Errorf("official run requires pinned model revision and artifact checksum")
	}
	if r.Quality.Status != TrackAvailable {
		return fmt.Errorf("official run requires available quality track")
	}
	if r.Quality.FailedQueries != 0 || r.Quality.QueryCount != r.Dataset.QueryCount || len(r.Quality.Results) != r.Dataset.QueryCount {
		return fmt.Errorf("official run requires complete successful quality query coverage")
	}
	if r.Performance.Status == TrackAvailable {
		if len(r.Performance.Cases) == 0 {
			return fmt.Errorf("official performance track requires measured cases")
		}
		for _, c := range r.Performance.Cases {
			if c.Status == TrackUnavailable {
				continue
			}
			if strings.Contains(c.Name, "_fixture_setup_") {
				if c.Repetitions != 1 || len(c.Samples) != 1 {
					return fmt.Errorf("official setup case %q requires exactly one sample because setup is excluded from repeated latency measurements", c.Name)
				}
				continue
			}
			if c.Repetitions < 5 || len(c.Samples) < 5 {
				return fmt.Errorf("official case %q requires at least five measured repetitions", c.Name)
			}
		}
	}
	return nil
}

func WriteReport(w io.Writer, r Report) error {
	if err := ValidateReport(r); err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}

func LoadReport(rd io.Reader) (Report, error) {
	var r Report
	dec := json.NewDecoder(rd)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return Report{}, err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return Report{}, fmt.Errorf("trailing report data")
	}
	return r, nil
}

func validateQualityTrack(t QualityTrack) error {
	if err := validateStatus(t.Status, t.UnavailableReason); err != nil {
		return err
	}
	if t.Status == TrackUnavailable {
		if len(t.Results) > 0 {
			return fmt.Errorf("unavailable track must not contain results")
		}
		return nil
	}
	if t.QueryCount < 1 || t.FailedQueries < 0 || t.FailedQueries > t.QueryCount {
		return fmt.Errorf("query_count/failed_queries are invalid")
	}
	if len(t.Results) != t.QueryCount {
		return fmt.Errorf("results count %d does not match query_count %d", len(t.Results), t.QueryCount)
	}
	if err := validateMetrics(t.Metrics); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, result := range t.Results {
		if result.QueryID == "" || seen[result.QueryID] {
			return fmt.Errorf("query result IDs must be non-empty and unique")
		}
		seen[result.QueryID] = true
		if result.Error == "" {
			if err := validateMetrics(result.Metrics); err != nil {
				return fmt.Errorf("query %s: %w", result.QueryID, err)
			}
		}
		for _, sample := range result.LatencySamplesMS {
			if !finite(sample) || sample < 0 {
				return fmt.Errorf("query %s has invalid latency sample", result.QueryID)
			}
		}
	}
	return nil
}

func validatePerformanceTrack(t PerformanceTrack) error {
	if err := validateStatus(t.Status, t.UnavailableReason); err != nil {
		return err
	}
	if t.Status == TrackUnavailable {
		if len(t.Cases) > 0 {
			return fmt.Errorf("unavailable track must not contain cases")
		}
		return nil
	}
	if strings.TrimSpace(t.Backend) == "" || t.Concurrency < 1 {
		return fmt.Errorf("available track requires backend and positive concurrency")
	}
	if len(t.Cases) == 0 {
		return fmt.Errorf("available track requires at least one case")
	}
	seen := map[string]bool{}
	for _, c := range t.Cases {
		if c.Name == "" || seen[c.Name] {
			return fmt.Errorf("case names must be non-empty and unique")
		}
		seen[c.Name] = true
		if c.Status != TrackAvailable && c.Status != TrackUnavailable {
			return fmt.Errorf("case %s status must be available or unavailable", c.Name)
		}
		if c.Status == TrackUnavailable {
			if strings.TrimSpace(c.UnavailableReason) == "" || c.Repetitions != 0 || len(c.Samples) != 0 {
				return fmt.Errorf("unavailable case %s requires a reason and no samples", c.Name)
			}
			continue
		}
		if c.UnavailableReason != "" || c.Unit != "ms" || c.Repetitions < 1 || len(c.Samples) != c.Repetitions {
			return fmt.Errorf("case %s repetitions, samples, or unit are invalid", c.Name)
		}
		for _, sample := range c.Samples {
			if !finite(sample) || sample < 0 {
				return fmt.Errorf("case %s has invalid sample", c.Name)
			}
		}
		want := summarizeLatency(c.Samples)
		if !sameFloat(c.Summary.P50, want.P50) || !sameFloat(c.Summary.P95, want.P95) || !sameFloat(c.Summary.P99, want.P99) || !sameFloat(c.Summary.Median, want.Median) || !sameFloat(c.Summary.Spread, want.Spread) {
			return fmt.Errorf("case %s summary does not match samples", c.Name)
		}
	}
	return nil
}

func validateStatus(status, reason string) error {
	if status != TrackAvailable && status != TrackUnavailable {
		return fmt.Errorf("status must be available or unavailable")
	}
	if status == TrackUnavailable && strings.TrimSpace(reason) == "" {
		return fmt.Errorf("unavailable track requires a reason")
	}
	if status == TrackAvailable && reason != "" {
		return fmt.Errorf("available track cannot have unavailable_reason")
	}
	return nil
}

func validateMetrics(m QualityMetrics) error {
	for label, value := range map[string]float64{"recall_at_5": m.RecallAt5, "recall_at_10": m.RecallAt10, "recall_at_20": m.RecallAt20, "mrr": m.MRR, "ndcg_at_10": m.NDCGAt10} {
		if !finite(value) || value < 0 || value > 1 {
			return fmt.Errorf("%s must be finite and between 0 and 1", label)
		}
	}
	return nil
}

func summarizeLatency(samples []float64) LatencySummary {
	if len(samples) == 0 {
		return LatencySummary{}
	}
	ordered := append([]float64(nil), samples...)
	sort.Float64s(ordered)
	mean := func(p float64) float64 {
		if len(ordered) == 1 {
			return ordered[0]
		}
		index := p * float64(len(ordered)-1)
		lo := int(math.Floor(index))
		hi := int(math.Ceil(index))
		if lo == hi {
			return ordered[lo]
		}
		return ordered[lo] + (ordered[hi]-ordered[lo])*(index-float64(lo))
	}
	return LatencySummary{P50: mean(.5), P95: mean(.95), P99: mean(.99), Median: mean(.5), Spread: ordered[len(ordered)-1] - ordered[0]}
}

func sameFloat(a, b float64) bool { return finite(a) && math.Abs(a-b) < 1e-9 }
func finite(v float64) bool       { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func isSHA256(s string) bool      { return regexp.MustCompile(`^[a-fA-F0-9]{64}$`).MatchString(s) }
