# MIRA benchmark guide

This directory separates the existing CBA selector microbenchmark from the
versioned public benchmark protocol. They answer different questions and their
numbers must not be compared as if they measured the same system.

## Existing CBA selector microbenchmark

`make bench-locomo` runs `BenchmarkLoCoMoRecall`. Despite the historical target
name, it does not run the LoCoMo dataset. It builds synthetic candidates with
preassigned relevance scores and a test interactor, then measures the CBA
selection path for a fixed query. It does not measure persistent storage,
embeddings, HNSW, SQL fallback, full application retrieval, or answer accuracy.
Treat its output as a local selector microbenchmark only.

```bash
make bench-locomo
```

The optional JSON output from this older command is diagnostic and is not a
public result. Do not cite its latency as a full MIRA recall measurement.

## Versioned public benchmark

The public protocol uses the authored and redistributable fixture at
[`datasets/synthetic-v1.json`](datasets/synthetic-v1.json). Its manifest records
the canonical SHA-256, generator version, seed, and record counts. Relevance
labels are fixed in that fixture and are never generated from MIRA's own scores.
The quality track uses `Recall@5/10/20`, MRR, and `nDCG@10`. The performance
track is independent and records named component timings, setup policy, raw
samples, and runtime provenance.

Reports follow [`schema/v1.schema.json`](schema/v1.schema.json). A local run may
be dirty or have an unavailable track, but it must state those facts. An
official website snapshot requires a clean source revision, the checked-in
dataset hash, a pinned model identity, host/backend provenance, complete quality
coverage, and at least five measured samples for each repeated latency case.
Fixture setup is reported as one standalone observation because setup is
excluded from repeated latency measurements; it must not be interpreted as a
p50/p95 estimate. Timing results describe the recorded host and protocol; they
are not product SLOs. Synthetic quality results are not LoCoMo scores and do
not establish general answer accuracy.

## Run locally

Requirements: Go 1.25+, the checked-in fixture and model lock, and a local copy
of the locked model for the quality and full-recall cases. SQLite runs create
temporary storage and remove it when the command exits. PostgreSQL runs require
an explicit disposable URL whose database name ends in `_test`; MIRA clears its
benchmark records before and after the run. Model files remain in the supplied
local model directory and are verified by SHA-256 before use.

Set `MIRA_BENCH_MODEL_DIR` to the directory containing the locked model files,
or set `model_dir` in a copy of
[`config.example.json`](config.example.json). Then run from the core repository
root:

```bash
MIRA_BENCH_MODEL_DIR=/path/to/all-MiniLM-L6-v2 \
  go run -tags fts5 ./cmd/mira-benchmark run \
  --track all \
  --config benchmarks/config.example.json \
  --output benchmarks/results/local.json \
  --warmups 2 \
  --repetitions 5

go run -tags fts5 ./cmd/mira-benchmark validate \
  --report benchmarks/results/local.json
```

The example profile uses 100 and 1,000 records so a local smoke run finishes
quickly. A reference run should include 10,000 records and at least five
measured repetitions. Local reports can be dirty and are never exportable as
the public snapshot. SQLite and PostgreSQL use separate reports and never reuse
one another's timings.

For PostgreSQL with the local pgvector service:

```bash
docker compose up -d postgres
MIRA_BENCH_MODEL_DIR=/path/to/all-MiniLM-L6-v2 \
  go run -tags fts5 ./cmd/mira-benchmark run \
  --track all \
  --config benchmarks/config.postgres.json \
  --database-url 'postgres://mira:mira@localhost:5433/mira_benchmark_test?sslmode=disable' \
  --output benchmarks/results/local-postgres.json \
  --warmups 2 \
  --repetitions 5
```

Temporary databases and indexes are created under the core checkout and
removed on exit. Set `MIRA_BENCH_TMP_DIR` to use another writable filesystem
when the checkout volume has limited space.

`make bench-public` runs the same local protocol. Override `BENCH_CONFIG`,
`MIRA_BENCH_REPORT`, `BENCH_WARMUPS`, and `BENCH_REPETITIONS` to select a profile
and output path. `make bench-public-validate` validates that local report.

## Attach a benchmark to a release

After the release tag exists on GitHub, run this from a clean core checkout with
the pinned model available locally. The target uses the release profile
(100/1,000/10,000 records), validates the report as official, uploads the raw
JSON to the matching GitHub Release, exports versioned and current website
snapshots, then tests and builds the site:

```bash
MIRA_BENCH_MODEL_DIR=/path/to/all-MiniLM-L6-v2 \
MIRA_BENCH_POSTGRES_URL='postgres://mira:mira@localhost:5433/mira_benchmark_test?sslmode=disable' \
  make bench-release VERSION=0.8.6 MIRA_SITE_DIR=../mira-landing
```

When `MIRA_BENCH_POSTGRES_URL` is set, the target validates and attaches a
second, PostgreSQL-specific report and exports a versioned companion snapshot.

The release workflow first builds binaries and tests the tagged core. The
benchmark remains on the release host so future releases can be measured with
the same pinned model and documented hardware. Keep the host and protocol
consistent for comparisons; the site records provenance and does not compare
timings from different environments as if they were equivalent. The version
history lists releases from 0.8.0, but versions before this protocol have no
retroactive measurements.

## Official site export

An official export requires a clean core commit, complete quality queries,
matching dataset checksum, the pinned model files, host/backend provenance,
and at least five samples per repeated timing case (fixture setup is a single
observation):

```bash
go run -tags fts5 ./cmd/mira-benchmark validate \
  --report benchmarks/results/<run-id>.json --official

go run -tags fts5 ./cmd/mira-benchmark export-site \
  --report benchmarks/results/<run-id>.json \
  --out ../mira-landing/public/data/benchmarks/current.json
```

The exporter refuses dirty, incomplete, or mismatched reports. It records the
raw report checksum, source core commit, and stable report URL in the static
site snapshot. Review the report and page before publishing; this workflow
does not deploy the website. The public page must not generalize host-specific
latencies into an SLO.

`scripts/benchmark.html` is a legacy visualization demo with hard-coded sample
values. It is not connected to the report validator and must not be used as
benchmark evidence.

## Comparisons

The first release intentionally has no competitor leaderboard. A meaningful
comparison needs matching data, model, hardware, storage backend, and protocol;
market figures are context, not head-to-head evidence. See
[`docs/MARKET_REFERENCES.md`](../docs/MARKET_REFERENCES.md).
