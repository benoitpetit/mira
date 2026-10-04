package benchmark

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/benoitpetit/mira/internal/adapters/storage"
	mira "github.com/benoitpetit/mira/pkg/mira"
)

type RunnerConfig struct {
	ModelDir    string `json:"model_dir,omitempty"`
	ModelLock   string `json:"model_lock,omitempty"`
	CorpusSizes []int  `json:"corpus_sizes,omitempty"`
	Dimension   int    `json:"dimension,omitempty"`
	Seed        int64  `json:"seed,omitempty"`
	Backend     string `json:"backend,omitempty"`
}

type SiteSnapshot struct {
	SchemaVersion      string             `json:"schema_version"`
	SourceReportSHA256 string             `json:"source_report_sha256"`
	SourceCommit       string             `json:"source_commit"`
	SourceURL          string             `json:"source_url"`
	Benchmark          BenchmarkIdentity  `json:"benchmark"`
	Run                RunIdentity        `json:"run"`
	Dataset            DatasetIdentity    `json:"dataset"`
	Environment        RuntimeEnvironment `json:"environment"`
	Model              ModelIdentity      `json:"model"`
	Protocol           Protocol           `json:"protocol"`
	Quality            QualityTrack       `json:"quality"`
	Performance        PerformanceTrack   `json:"performance"`
}

func RunBenchmarkCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		writeBenchmarkHelp(stdout)
		return nil
	}
	switch args[0] {
	case "run":
		return runBenchmark(ctx, args[1:], stdout, stderr)
	case "validate":
		return validateBenchmarkCommand(args[1:], stdout, stderr)
	case "export-site":
		return exportSiteCommand(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown benchmark command %q (try --help)", args[0])
	}
}

func ValidateDisposableDatabase(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("database URL is required")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" {
		return fmt.Errorf("database URL must be a postgres:// or postgresql:// URL with a host")
	}
	name := strings.Trim(strings.TrimSpace(u.Path), "/")
	if !strings.HasSuffix(name, "_test") {
		return fmt.Errorf("disposable PostgreSQL database name must end in _test")
	}
	return nil
}

func ExportSiteSnapshot(reportReader io.Reader, sourceURL string, out io.Writer) error {
	reportBytes, err := io.ReadAll(reportReader)
	if err != nil {
		return err
	}
	report, err := LoadReport(strings.NewReader(string(reportBytes)))
	if err != nil {
		return fmt.Errorf("decode report: %w", err)
	}
	if err := ValidateOfficialRun(report); err != nil {
		return fmt.Errorf("report is not exportable: %w", err)
	}
	u, err := url.ParseRequestURI(sourceURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return fmt.Errorf("source URL must be an absolute HTTP(S) URL")
	}
	sum := sha256.Sum256(reportBytes)
	snapshot := SiteSnapshot{SchemaVersion: ReportSchemaVersion, SourceReportSHA256: hex.EncodeToString(sum[:]), SourceCommit: report.Source.Commit, SourceURL: sourceURL, Benchmark: report.Benchmark, Run: report.Run, Dataset: report.Dataset, Environment: report.Environment, Model: report.Model, Protocol: report.Protocol, Quality: report.Quality, Performance: report.Performance}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(snapshot)
}

func runBenchmark(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	trackName := fs.String("track", "all", "track: all, quality, or performance")
	configPath := fs.String("config", "", "benchmark JSON configuration file")
	outputPath := fs.String("output", "", "output report JSON path")
	warmups := fs.Int("warmups", 2, "untimed warm-up iterations")
	repetitions := fs.Int("repetitions", 5, "measured repetitions")
	databaseURL := fs.String("database-url", "", "optional disposable PostgreSQL URL; v1 runner currently reports PostgreSQL unavailable")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return fmt.Errorf("--config is required")
	}
	if *outputPath == "" {
		return fmt.Errorf("--output is required")
	}
	if *trackName != "all" && *trackName != "quality" && *trackName != "performance" {
		return fmt.Errorf("unknown track %q", *trackName)
	}
	if *warmups < 0 {
		return fmt.Errorf("warmups cannot be negative")
	}
	if *repetitions < 1 {
		return fmt.Errorf("repetitions must be positive")
	}
	if *databaseURL != "" {
		if err := ValidateDisposableDatabase(*databaseURL); err != nil {
			return err
		}
		return fmt.Errorf("PostgreSQL execution is not implemented in benchmark v1; URL was validated but not used")
	}
	cfg, err := loadRunnerConfig(*configPath)
	if err != nil {
		return err
	}
	if cfg.ModelDir == "" {
		cfg.ModelDir = os.Getenv("MIRA_BENCH_MODEL_DIR")
	}
	if cfg.ModelDir != "" && !filepath.IsAbs(cfg.ModelDir) {
		cfg.ModelDir = filepath.Clean(filepath.Join(filepath.Dir(*configPath), cfg.ModelDir))
	}
	repoRoot, err := repositoryRoot()
	if err != nil {
		return err
	}
	lockPath := cfg.ModelLock
	if lockPath == "" {
		lockPath = "benchmarks/models/all-MiniLM-L6-v2.lock.json"
	}
	if !filepath.IsAbs(lockPath) {
		lockPath = filepath.Join(repoRoot, filepath.Clean(lockPath))
	}
	lockFile, err := os.Open(lockPath)
	if err != nil {
		if cfg.ModelDir != "" {
			return fmt.Errorf("open model lock: %w", err)
		}
	}
	lock := EmbeddingModelLock{}
	if lockFile != nil {
		lock, err = LoadEmbeddingModelLock(lockFile)
		closeErr := lockFile.Close()
		if err != nil {
			return fmt.Errorf("load model lock: %w", err)
		}
		if closeErr != nil {
			return closeErr
		}
	}
	datasetBytes, err := os.ReadFile(filepath.Join(repoRoot, "benchmarks/datasets/synthetic-v1.json"))
	if err != nil {
		return fmt.Errorf("read checked-in benchmark dataset: %w", err)
	}
	dataset, err := LoadDataset(strings.NewReader(string(datasetBytes)))
	if err != nil {
		return err
	}
	manifest, err := BuildManifest(dataset, 42, SyntheticV1GeneratorVersion)
	if err != nil {
		return err
	}
	tempRoot := os.Getenv("MIRA_BENCH_TMP_DIR")
	if tempRoot == "" {
		tempRoot = repoRoot
	}
	workRoot, err := newBenchmarkWorkspace(tempRoot)
	if err != nil {
		return err
	}
	defer os.RemoveAll(workRoot)
	report, err := buildLocalReport(ctx, *trackName, cfg, lock, dataset, manifest, *warmups, *repetitions, workRoot, repoRoot)
	if err != nil {
		return err
	}
	if err := ValidateReport(report); err != nil {
		return fmt.Errorf("generated report validation failed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(*outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	writeErr := WriteReport(out, report)
	closeErr := out.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	fmt.Fprintf(stdout, "Wrote %s report to %s (dirty=%t)\n", *trackName, *outputPath, report.Source.Dirty)
	return nil
}

func newBenchmarkWorkspace(root string) (string, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create benchmark temp root: %w", err)
	}
	path, err := os.MkdirTemp(root, ".mira-public-benchmark-")
	if err != nil {
		return "", fmt.Errorf("create isolated benchmark workspace: %w", err)
	}
	return path, nil
}

func buildLocalReport(ctx context.Context, trackName string, cfg RunnerConfig, lock EmbeddingModelLock, dataset Dataset, manifest DatasetManifest, warmups, repetitions int, workRoot, repoRoot string) (Report, error) {
	now := time.Now().UTC()
	commit, dirty := sourceState(ctx, repoRoot)
	if commit == "" {
		commit = strings.Repeat("0", 40)
	}
	model := ModelIdentity{}
	if lock.ModelID != "" {
		model = ModelIdentity{Name: lock.ModelID, Revision: lock.Revision, SHA256: modelLockChecksum(lock)}
	}
	report := Report{SchemaVersion: ReportSchemaVersion, Benchmark: BenchmarkIdentity{Name: "mira-public", Version: "1.0.0"}, Run: RunIdentity{ID: now.Format("20060102T150405.000000000Z"), StartedAtUTC: now, Track: trackName}, Source: SourceIdentity{Commit: commit, Dirty: dirty}, Dataset: DatasetIdentity{Name: dataset.Name, Version: dataset.Version, SHA256: manifest.SHA256, Seed: manifest.Seed, MemoryCount: manifest.MemoryCount, QueryCount: manifest.QueryCount, JudgmentCount: manifest.JudgmentCount}, Environment: collectEnvironment(cfg.Backend), Model: model, Protocol: Protocol{Warmups: warmups, Repetitions: repetitions, Concurrency: 1, SetupExcluded: true}}
	if report.Environment.StorageBackend == "" {
		report.Environment.StorageBackend = "sqlite"
	}
	if trackName == "all" || trackName == "quality" {
		if cfg.ModelDir == "" {
			report.Quality = QualityTrack{Status: TrackUnavailable, UnavailableReason: "set model_dir or MIRA_BENCH_MODEL_DIR to the pinned local model", Metrics: QualityMetrics{}}
		} else {
			if err := VerifyEmbeddingModel(cfg.ModelDir, lock); err != nil {
				return Report{}, err
			}
			app, err := newBenchmarkApplication(workRoot, cfg.ModelDir, lock)
			if err != nil {
				return Report{}, err
			}
			quality, runErr := RunQualityTrackWithWarmups(ctx, app, dataset, warmups, repetitions)
			closeErr := app.Close()
			if runErr != nil {
				return Report{}, runErr
			}
			if closeErr != nil {
				return Report{}, closeErr
			}
			report.Quality = quality
		}
	} else {
		report.Quality = QualityTrack{Status: TrackUnavailable, UnavailableReason: "quality track was not requested"}
	}
	if trackName == "all" || trackName == "performance" {
		sizes := cfg.CorpusSizes
		if len(sizes) == 0 {
			sizes = []int{100, 1000, 10000}
		}
		performance, err := RunPerformanceTrack(ctx, PerformanceConfig{StorageRoot: workRoot, CorpusSizes: sizes, Dimension: cfg.Dimension, Seed: cfg.Seed, Warmups: warmups, Repetitions: repetitions, Concurrency: 1, Backend: cfg.Backend, ModelDir: cfg.ModelDir, ModelLock: lock})
		if err != nil {
			return Report{}, err
		}
		report.Performance = performance
	} else {
		backend := cfg.Backend
		if backend == "" {
			backend = "sqlite"
		}
		report.Performance = PerformanceTrack{Status: TrackUnavailable, UnavailableReason: "performance track was not requested", Backend: backend, Concurrency: 1}
	}
	return report, nil
}

func newBenchmarkApplication(root, modelDir string, lock EmbeddingModelLock) (*mira.Application, error) {
	if err := VerifyEmbeddingModel(modelDir, lock); err != nil {
		return nil, err
	}
	storagePath := filepath.Join(root, "quality-storage")
	modelPath := filepath.Join(storagePath, "models", lock.ModelID)
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o700); err != nil {
		return nil, err
	}
	if err := os.Symlink(modelDir, modelPath); err != nil {
		return nil, fmt.Errorf("isolate model cache: %w", err)
	}
	cfg := mira.DefaultConfig()
	cfg.Storage.Type = "sqlite"
	cfg.Storage.Path = storagePath
	cfg.Embeddings.CurrentModel = lock.ModelID
	cfg.Embeddings.Dimension = 384
	cfg.Embeddings.UseSimpleEmbedder = false
	cfg.Extraction.LLM.Enabled = false
	cfg.Metrics.Enabled = false
	cfg.Webhooks.Enabled = false
	cfg.API.Enabled = false
	return mira.NewApplication(cfg)
}

func loadRunnerConfig(path string) (RunnerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RunnerConfig{}, fmt.Errorf("read benchmark config: %w", err)
	}
	var cfg RunnerConfig
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return RunnerConfig{}, fmt.Errorf("decode benchmark config: %w", err)
	}
	if cfg.Seed == 0 {
		cfg.Seed = 42
	}
	if cfg.Dimension == 0 {
		cfg.Dimension = 384
	}
	if cfg.Backend == "" {
		cfg.Backend = "sqlite"
	}
	return cfg, nil
}

func modelLockChecksum(lock EmbeddingModelLock) string {
	files := append([]ModelArtifact(nil), lock.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	h := sha256.New()
	for _, f := range files {
		fmt.Fprintf(h, "%s:%s\n", f.Path, strings.ToLower(f.SHA256))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func collectEnvironment(backend string) RuntimeEnvironment {
	host, _ := os.Hostname()
	version := mira.DefaultConfig().System.Version
	if version == "" {
		version = "0.8.4"
	}
	if backend == "" {
		backend = "sqlite"
	}
	dbVersion := "not exposed"
	root, err := os.MkdirTemp("", "mira-bench-env-")
	if err == nil {
		if repo, e := storage.NewSQLiteRepository(filepath.Join(root, "version.db"), storage.DefaultSQLiteOptions()); e == nil {
			var v string
			if e = repo.DB().QueryRow("SELECT sqlite_version()").Scan(&v); e == nil {
				dbVersion = v
			}
			_ = repo.Close()
		}
		_ = os.RemoveAll(root)
	}
	buildTags := []string{}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "-tags" {
				buildTags = append(buildTags, strings.Fields(setting.Value)...)
			}
		}
	}
	return RuntimeEnvironment{MiraVersion: version, GoVersion: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH, Host: host, CPUModel: cpuModel(), LogicalCPUs: runtime.NumCPU(), GOMAXPROCS: runtime.GOMAXPROCS(0), BuildTags: buildTags, StorageBackend: backend, DatabaseVersion: dbVersion}
}

func cpuModel() string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.ToLower(line), "model name") {
				if _, value, ok := strings.Cut(line, ":"); ok {
					return strings.TrimSpace(value)
				}
			}
		}
	}
	return "not exposed"
}

func sourceState(ctx context.Context, repoRoot string) (string, bool) {
	commitCmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	commitCmd.Dir = repoRoot
	commit, err := commitCmd.Output()
	if err != nil {
		return "", true
	}
	statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain", "--untracked-files=normal")
	statusCmd.Dir = repoRoot
	status, err := statusCmd.Output()
	if err != nil {
		return strings.TrimSpace(string(commit)), true
	}
	return strings.TrimSpace(string(commit)), len(status) > 0
}

func repositoryRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	root, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("locate MIRA repository root: %w", err)
	}
	return strings.TrimSpace(string(root)), nil
}

func validateBenchmarkCommand(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	path := fs.String("report", "", "report JSON path")
	official := fs.Bool("official", false, "validate public provenance requirements")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return fmt.Errorf("--report is required")
	}
	f, err := os.Open(*path)
	if err != nil {
		return err
	}
	report, readErr := LoadReport(f)
	closeErr := f.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	if *official {
		err = ValidateOfficialRun(report)
	} else {
		err = ValidateReport(report)
	}
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Report is valid")
	return nil
}

func exportSiteCommand(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("export-site", flag.ContinueOnError)
	fs.SetOutput(stderr)
	reportPath := fs.String("report", "", "validated report JSON path")
	outPath := fs.String("out", "", "site snapshot output path")
	sourceURL := fs.String("source-url", "", "stable raw report URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *reportPath == "" || *outPath == "" {
		return fmt.Errorf("--report and --out are required")
	}
	reportBytes, err := os.ReadFile(*reportPath)
	if err != nil {
		return err
	}
	if *sourceURL == "" {
		report, err := LoadReport(bytes.NewReader(reportBytes))
		if err != nil {
			return err
		}
		*sourceURL = fmt.Sprintf("https://raw.githubusercontent.com/benoitpetit/mira/%s/benchmarks/results/%s.json", report.Source.Commit, report.Run.ID)
	}
	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(*outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	writeErr := ExportSiteSnapshot(bytes.NewReader(reportBytes), *sourceURL, out)
	closeErr := out.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	fmt.Fprintf(stdout, "Wrote validated site snapshot to %s\n", *outPath)
	return nil
}

func writeBenchmarkHelp(w io.Writer) {
	fmt.Fprintln(w, "MIRA public benchmark\n\nCommands:\n  mira-benchmark run --track all|quality|performance --config <path> --output <path> [--warmups N --repetitions N]\n  mira-benchmark validate --report <path> [--official]\n  mira-benchmark export-site --report <path> --out <path> [--source-url URL]")
}
