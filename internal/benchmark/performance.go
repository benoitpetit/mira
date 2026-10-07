package benchmark

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/benoitpetit/mira/internal/adapters/storage"
	"github.com/benoitpetit/mira/internal/adapters/vector"
	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/util"
	mira "github.com/benoitpetit/mira/pkg/mira"
)

type PerformanceConfig struct {
	StorageRoot string
	CorpusSizes []int
	Dimension   int
	Seed        int64
	Warmups     int
	Repetitions int
	Concurrency int
	Backend     string
	DatabaseURL string
	ModelDir    string
	ModelLock   EmbeddingModelLock
}

func RunPerformanceTrack(ctx context.Context, config PerformanceConfig) (PerformanceTrack, error) {
	if len(config.CorpusSizes) == 0 {
		return PerformanceTrack{}, fmt.Errorf("at least one corpus size is required")
	}
	if config.Warmups < 0 || config.Repetitions < 1 {
		return PerformanceTrack{}, fmt.Errorf("warmups must be non-negative and repetitions must be positive")
	}
	if config.Concurrency != 0 && config.Concurrency != 1 {
		return PerformanceTrack{}, fmt.Errorf("only concurrency=1 is supported in benchmark v1")
	}
	if config.Dimension <= 0 {
		config.Dimension = 384
	}
	if config.Backend == "" {
		config.Backend = "sqlite"
	}
	if config.Backend != "sqlite" && config.Backend != "postgres" {
		return PerformanceTrack{}, fmt.Errorf("unsupported performance backend %q", config.Backend)
	}
	if config.Backend == "postgres" {
		if err := ValidateDisposableDatabase(config.DatabaseURL); err != nil {
			return PerformanceTrack{}, fmt.Errorf("validate PostgreSQL benchmark database URL: %w", err)
		}
		return runPostgresPerformanceTrack(ctx, config)
	}
	sizes := append([]int(nil), config.CorpusSizes...)
	for _, n := range sizes {
		if n < 1 {
			return PerformanceTrack{}, fmt.Errorf("corpus sizes must be positive")
		}
	}
	sort.Ints(sizes)
	for i := 1; i < len(sizes); i++ {
		if sizes[i] == sizes[i-1] {
			return PerformanceTrack{}, fmt.Errorf("corpus sizes must be unique")
		}
	}
	root := config.StorageRoot
	if root == "" {
		root = os.TempDir()
	}
	workDir, err := os.MkdirTemp(root, "mira-benchmark-performance-")
	if err != nil {
		return PerformanceTrack{}, fmt.Errorf("create isolated benchmark directory: %w", err)
	}
	defer os.RemoveAll(workDir)

	concurrency := config.Concurrency
	if concurrency == 0 {
		concurrency = 1
	}
	track := PerformanceTrack{Status: TrackAvailable, Backend: config.Backend, Concurrency: concurrency}
	var app *mira.Application
	appStorage := filepath.Join(workDir, "mira-application")
	if config.ModelDir != "" {
		if err := VerifyEmbeddingModel(config.ModelDir, config.ModelLock); err != nil {
			return PerformanceTrack{}, fmt.Errorf("verify quality model before full recall measurement: %w", err)
		}
		cfg := mira.DefaultConfig()
		cfg.Storage.Type = "sqlite"
		cfg.Storage.Path = appStorage
		cfg.Embeddings.CurrentModel = config.ModelLock.ModelID
		cfg.Embeddings.UseSimpleEmbedder = false
		cfg.Embeddings.Dimension = 384
		cfg.Extraction.LLM.Enabled = false
		cfg.Metrics.Enabled = false
		cfg.Webhooks.Enabled = false
		cfg.API.Enabled = false
		modelTarget := filepath.Join(appStorage, "models", config.ModelLock.ModelID)
		if err := os.MkdirAll(filepath.Dir(modelTarget), 0o700); err != nil {
			return PerformanceTrack{}, err
		}
		if err := os.Symlink(config.ModelDir, modelTarget); err != nil {
			return PerformanceTrack{}, fmt.Errorf("isolate model cache: %w", err)
		}
		app, err = mira.NewApplication(cfg)
		if err != nil {
			return PerformanceTrack{}, fmt.Errorf("initialize performance MIRA application: %w", err)
		}
		defer app.Close()
	}
	appIngested := 0
	for _, size := range sizes {
		caseDir := filepath.Join(workDir, fmt.Sprintf("corpus-%d", size))
		if err := os.MkdirAll(caseDir, 0o700); err != nil {
			return PerformanceTrack{}, err
		}
		dbPath := filepath.Join(caseDir, "fixture.db")
		started := time.Now()
		repo, err := storage.NewSQLiteRepository(dbPath, storage.DefaultSQLiteOptions())
		if err != nil {
			return PerformanceTrack{}, fmt.Errorf("open SQLite fixture for %d memories: %w", size, err)
		}
		modelHash := util.ComputeModelHash("mira-benchmark-vector-fixture-v1")
		if err := buildVectorFixture(ctx, repo, size, config.Dimension, config.Seed, modelHash); err != nil {
			_ = repo.Close()
			return PerformanceTrack{}, fmt.Errorf("build SQLite fixture for %d memories: %w", size, err)
		}
		setupDuration := float64(time.Since(started).Nanoseconds()) / 1e6
		track.Cases = append(track.Cases, makeLatencyCase(fmt.Sprintf("sqlite_fixture_setup_%d", size), []float64{setupDuration}))

		hnsw, err := vector.NewHNSWStore(repo, config.Dimension, filepath.Join(caseDir, "vectors.bin"), vector.DefaultHNSWOptions())
		if err != nil {
			track.Cases = append(track.Cases, unavailableLatencyCase(fmt.Sprintf("hnsw_rebuild_%d", size), fmt.Sprintf("HNSW is unavailable on %s/%s: %v", runtime.GOOS, runtime.GOARCH, err)), unavailableLatencyCase(fmt.Sprintf("hnsw_warm_search_%d", size), fmt.Sprintf("HNSW is unavailable on %s/%s", runtime.GOOS, runtime.GOARCH)))
		} else {
			hnsw.SetModelHash(modelHash)
			rebuildSamples := make([]float64, 0, config.Repetitions)
			for rep := 0; rep < config.Repetitions; rep++ {
				start := time.Now()
				if err := hnsw.BuildFromStore(ctx); err != nil {
					_ = repo.Close()
					return PerformanceTrack{}, fmt.Errorf("build HNSW index for %d memories: %w", size, err)
				}
				rebuildSamples = append(rebuildSamples, float64(time.Since(start).Nanoseconds())/1e6)
				if !hnsw.IsReady() || hnsw.Stats() != size {
					_ = repo.Close()
					return PerformanceTrack{}, fmt.Errorf("HNSW index is not ready with %d vectors (stats=%d)", size, hnsw.Stats())
				}
			}
			track.Cases = append(track.Cases, makeLatencyCase(fmt.Sprintf("hnsw_rebuild_%d", size), rebuildSamples))
			queryVector := fixtureVector(config.Seed, 0, config.Dimension)
			for i := 0; i < config.Warmups; i++ {
				if _, err := hnsw.Search(ctx, queryVector, 10, nil, nil); err != nil {
					_ = repo.Close()
					return PerformanceTrack{}, fmt.Errorf("HNSW warm-up search: %w", err)
				}
			}
			searchSamples := make([]float64, 0, config.Repetitions)
			for rep := 0; rep < config.Repetitions; rep++ {
				start := time.Now()
				found, err := hnsw.Search(ctx, queryVector, 10, nil, nil)
				elapsed := time.Since(start)
				if err != nil {
					_ = repo.Close()
					return PerformanceTrack{}, fmt.Errorf("HNSW search: %w", err)
				}
				if len(found) == 0 {
					_ = repo.Close()
					return PerformanceTrack{}, fmt.Errorf("HNSW returned no candidates for populated %d-vector index", size)
				}
				searchSamples = append(searchSamples, float64(elapsed.Nanoseconds())/1e6)
			}
			track.Cases = append(track.Cases, makeLatencyCase(fmt.Sprintf("hnsw_warm_search_%d", size), searchSamples))
		}

		sqlStore := vector.NewSQLiteVectorStore(repo.DB())
		queryVector := fixtureVector(config.Seed, 0, config.Dimension)
		for i := 0; i < config.Warmups; i++ {
			if _, err := sqlStore.Search(ctx, queryVector, 10, nil, nil); err != nil {
				_ = repo.Close()
				return PerformanceTrack{}, fmt.Errorf("SQLite fallback warm-up search: %w", err)
			}
		}
		sqlSamples := make([]float64, 0, config.Repetitions)
		for rep := 0; rep < config.Repetitions; rep++ {
			start := time.Now()
			found, err := sqlStore.Search(ctx, queryVector, 10, nil, nil)
			elapsed := time.Since(start)
			if err != nil {
				_ = repo.Close()
				return PerformanceTrack{}, fmt.Errorf("SQLite fallback search: %w", err)
			}
			if len(found) == 0 {
				_ = repo.Close()
				return PerformanceTrack{}, fmt.Errorf("SQLite fallback returned no candidates for populated %d-memory fixture", size)
			}
			sqlSamples = append(sqlSamples, float64(elapsed.Nanoseconds())/1e6)
		}
		track.Cases = append(track.Cases, makeLatencyCase(fmt.Sprintf("sql_fallback_search_%d", size), sqlSamples))
		if app == nil {
			reason := "pinned local model directory was not supplied; full MIRA recall was not measured"
			track.Cases = append(track.Cases, unavailableLatencyCase(fmt.Sprintf("sqlite_full_recall_%d", size), reason), unavailableLatencyCase(fmt.Sprintf("mira_hnsw_rebuild_%d", size), reason))
		} else {
			start := time.Now()
			for appIngested < size {
				content := performanceMemory(appIngested)
				room := "scale"
				if _, err := app.Store(ctx, content, "benchmark", &room, nil); err != nil {
					return PerformanceTrack{}, fmt.Errorf("ingest MIRA performance fixture %d: %w", appIngested, err)
				}
				appIngested++
			}
			track.Cases = append(track.Cases, makeLatencyCase(fmt.Sprintf("mira_fixture_setup_%d", size), []float64{float64(time.Since(start).Nanoseconds()) / 1e6}))
			rebuildSamples := make([]float64, 0, config.Repetitions)
			for rep := 0; rep < config.Repetitions; rep++ {
				started := time.Now()
				if err := app.RebuildVectorIndex(ctx); err != nil {
					return PerformanceTrack{}, fmt.Errorf("rebuild MIRA HNSW index for %d memories: %w", size, err)
				}
				rebuildSamples = append(rebuildSamples, float64(time.Since(started).Nanoseconds())/1e6)
			}
			track.Cases = append(track.Cases, makeLatencyCase(fmt.Sprintf("mira_hnsw_rebuild_%d", size), rebuildSamples))
			query := "Which deterministic benchmark fixture memory describes the test corpus?"
			for i := 0; i < config.Warmups; i++ {
				result, err := app.Recall(ctx, query, 2000, "benchmark", nil, nil, nil)
				if err != nil {
					return PerformanceTrack{}, fmt.Errorf("full recall warm-up: %w", err)
				}
				if len(result.Memories) == 0 {
					return PerformanceTrack{}, fmt.Errorf("full recall warm-up returned no memories at size %d", size)
				}
			}
			recallSamples := make([]float64, 0, config.Repetitions)
			for rep := 0; rep < config.Repetitions; rep++ {
				started := time.Now()
				result, err := app.Recall(ctx, query, 2000, "benchmark", nil, nil, nil)
				elapsed := time.Since(started)
				if err != nil {
					return PerformanceTrack{}, fmt.Errorf("full recall: %w", err)
				}
				if len(result.Memories) == 0 {
					return PerformanceTrack{}, fmt.Errorf("full recall returned no memories at size %d", size)
				}
				recallSamples = append(recallSamples, float64(elapsed.Nanoseconds())/1e6)
			}
			track.Cases = append(track.Cases, makeLatencyCase(fmt.Sprintf("sqlite_full_recall_%d", size), recallSamples))
		}
		if err := repo.Close(); err != nil {
			return PerformanceTrack{}, err
		}
	}
	return track, nil
}

func runPostgresPerformanceTrack(ctx context.Context, config PerformanceConfig) (PerformanceTrack, error) {
	track := PerformanceTrack{Status: TrackAvailable, Backend: "postgres", Concurrency: 1}
	sizes := append([]int(nil), config.CorpusSizes...)
	sort.Ints(sizes)
	if config.ModelDir == "" {
		for _, size := range sizes {
			track.Cases = append(track.Cases, unavailableLatencyCase(fmt.Sprintf("postgres_full_recall_%d", size), "pinned local model directory was not supplied; full MIRA recall was not measured"))
		}
		return track, nil
	}
	if err := VerifyEmbeddingModel(config.ModelDir, config.ModelLock); err != nil {
		return PerformanceTrack{}, fmt.Errorf("verify quality model before PostgreSQL full recall measurement: %w", err)
	}

	root := config.StorageRoot
	if root == "" {
		root = os.TempDir()
	}
	workDir, err := os.MkdirTemp(root, "mira-benchmark-postgres-")
	if err != nil {
		return PerformanceTrack{}, fmt.Errorf("create isolated PostgreSQL benchmark directory: %w", err)
	}
	defer os.RemoveAll(workDir)

	for _, size := range sizes {
		app, err := newBenchmarkApplication(filepath.Join(workDir, fmt.Sprintf("corpus-%d", size)), config.ModelDir, config.ModelLock, "postgres", config.DatabaseURL)
		if err != nil {
			return PerformanceTrack{}, fmt.Errorf("initialize PostgreSQL MIRA application: %w", err)
		}
		if err := clearBenchmarkApplication(ctx, app); err != nil {
			_ = app.Close()
			return PerformanceTrack{}, err
		}

		started := time.Now()
		room := "benchmark"
		for index := 0; index < size; index++ {
			if _, err := app.Store(ctx, performanceMemory(index), "benchmark", &room, nil); err != nil {
				_ = clearBenchmarkApplication(ctx, app)
				_ = app.Close()
				return PerformanceTrack{}, fmt.Errorf("ingest PostgreSQL performance fixture %d: %w", index, err)
			}
		}
		track.Cases = append(track.Cases, makeLatencyCase(fmt.Sprintf("postgres_fixture_setup_%d", size), []float64{float64(time.Since(started).Nanoseconds()) / 1e6}))

		rebuildSamples := make([]float64, 0, config.Repetitions)
		for rep := 0; rep < config.Repetitions; rep++ {
			started := time.Now()
			if err := app.RebuildVectorIndex(ctx); err != nil {
				_ = clearBenchmarkApplication(ctx, app)
				_ = app.Close()
				return PerformanceTrack{}, fmt.Errorf("rebuild PostgreSQL MIRA HNSW index for %d memories: %w", size, err)
			}
			rebuildSamples = append(rebuildSamples, float64(time.Since(started).Nanoseconds())/1e6)
		}
		track.Cases = append(track.Cases, makeLatencyCase(fmt.Sprintf("postgres_hnsw_rebuild_%d", size), rebuildSamples))

		query := "Which deterministic benchmark fixture memory describes the test corpus?"
		for warmup := 0; warmup < config.Warmups; warmup++ {
			result, err := app.Recall(ctx, query, 2000, "benchmark", nil, nil, nil)
			if err != nil || len(result.Memories) == 0 {
				_ = clearBenchmarkApplication(ctx, app)
				_ = app.Close()
				if err != nil {
					return PerformanceTrack{}, fmt.Errorf("PostgreSQL full recall warm-up: %w", err)
				}
				return PerformanceTrack{}, fmt.Errorf("PostgreSQL full recall warm-up returned no memories at size %d", size)
			}
		}
		recallSamples := make([]float64, 0, config.Repetitions)
		for rep := 0; rep < config.Repetitions; rep++ {
			started := time.Now()
			result, err := app.Recall(ctx, query, 2000, "benchmark", nil, nil, nil)
			if err != nil || len(result.Memories) == 0 {
				_ = clearBenchmarkApplication(ctx, app)
				_ = app.Close()
				if err != nil {
					return PerformanceTrack{}, fmt.Errorf("PostgreSQL full recall: %w", err)
				}
				return PerformanceTrack{}, fmt.Errorf("PostgreSQL full recall returned no memories at size %d", size)
			}
			recallSamples = append(recallSamples, float64(time.Since(started).Nanoseconds())/1e6)
		}
		track.Cases = append(track.Cases, makeLatencyCase(fmt.Sprintf("postgres_full_recall_%d", size), recallSamples))
		if err := clearBenchmarkApplication(ctx, app); err != nil {
			_ = app.Close()
			return PerformanceTrack{}, err
		}
		if err := app.Close(); err != nil {
			return PerformanceTrack{}, err
		}
	}
	return track, nil
}

func performanceMemory(index int) string {
	return fmt.Sprintf("Benchmark fixture memory %05d. This deterministic document describes the persistent retrieval corpus and synthetic test record %d.", index, index)
}

func buildVectorFixture(ctx context.Context, repo *storage.SQLiteRepository, count, dimension int, seed int64, modelHash string) error {
	model := entities.NewEmbeddingModel("mira-benchmark-vector-fixture-v1", dimension)
	model.ModelHash = modelHash
	if err := repo.RegisterModel(ctx, model); err != nil {
		return err
	}
	tx, err := repo.Begin()
	if err != nil {
		return err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()
	room := "fixture"
	for i := 0; i < count; i++ {
		content := fmt.Sprintf("Benchmark fixture memory %05d. The deterministic vector sample belongs to the test corpus.", i)
		verbatim := entities.NewVerbatim(content, "benchmark", &room).WithTokenCount(13)
		fp := entities.NewFingerprint(verbatim.ID, valueobjects.TypeFact, modelHash).WithTokenEstimate(13)
		if err := repo.StoreVerbatimTx(ctx, tx, verbatim); err != nil {
			return err
		}
		if err := repo.StoreFingerprintTx(ctx, tx, fp); err != nil {
			return err
		}
		embedding := entities.NewEmbedding(verbatim.ID, modelHash, fixtureVector(seed, int64(i), dimension)).WithNormalization()
		if err := repo.StoreEmbeddingTx(ctx, tx, embedding); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	rollback = false
	return nil
}

func fixtureVector(seed, index int64, dimension int) []float32 {
	rng := rand.New(rand.NewSource(seed + index*7919 + 17))
	vector := make([]float32, dimension)
	for i := range vector {
		vector[i] = rng.Float32()*2 - 1
	}
	return vector
}

func makeLatencyCase(name string, samples []float64) LatencyCase {
	return LatencyCase{Name: name, Status: TrackAvailable, Unit: "ms", Repetitions: len(samples), Samples: samples, Summary: summarizeLatency(samples)}
}
func unavailableLatencyCase(name, reason string) LatencyCase {
	return LatencyCase{Name: name, Status: TrackUnavailable, UnavailableReason: reason, Unit: "ms"}
}
