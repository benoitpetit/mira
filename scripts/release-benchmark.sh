#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-}"
SITE_DIR="${2:-../mira-landing}"
if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Usage: make bench-release VERSION=x.y.z [MIRA_SITE_DIR=../mira-landing]" >&2
  exit 2
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
TAG="v${VERSION}"
EXPECTED_VERSION="$(sed -n 's/.*CurrentVersion = "\([0-9.]*\)".*/\1/p' internal/config/config.go | head -1)"
HEAD_TAG="$(git describe --tags --exact-match HEAD 2>/dev/null || true)"
REPORT="benchmarks/results/mira-benchmark-${TAG}.json"
POSTGRES_REPORT="benchmarks/results/mira-benchmark-${TAG}-postgres.json"
SITE_ROOT="$(cd "$ROOT" && realpath -m "$SITE_DIR")"

if [[ "$EXPECTED_VERSION" != "$VERSION" ]]; then
  echo "Core version is ${EXPECTED_VERSION:-unknown}; expected $VERSION" >&2
  exit 1
fi
if [[ "$HEAD_TAG" != "$TAG" ]]; then
  echo "HEAD must be tagged $TAG before generating the release benchmark" >&2
  exit 1
fi
if [[ -n "$(git status --porcelain)" ]]; then
  echo "Core worktree must be clean before generating an official benchmark" >&2
  exit 1
fi
if [[ ! -d "$SITE_ROOT" || ! -f "$SITE_ROOT/package.json" ]]; then
  echo "Site repository not found at $SITE_ROOT; set MIRA_SITE_DIR" >&2
  exit 1
fi
if ! grep -q "version: \"$VERSION\"" "$SITE_ROOT/components/versions/releases.ts"; then
  echo "Add $VERSION and its release date to the site's version index before benchmarking" >&2
  exit 1
fi
if [[ -z "${MIRA_BENCH_MODEL_DIR:-}" ]]; then
  echo "Set MIRA_BENCH_MODEL_DIR to the directory containing the pinned model files" >&2
  exit 1
fi

"${GO:-go}" run -tags fts5 ./cmd/mira-benchmark run \
  --track all --config benchmarks/config.release.json --output "$REPORT" \
  --warmups "${BENCH_WARMUPS:-2}" --repetitions "${BENCH_REPETITIONS:-5}"
"${GO:-go}" run -tags fts5 ./cmd/mira-benchmark validate --report "$REPORT" --official

gh release view "$TAG" >/dev/null
gh release upload "$TAG" "$REPORT" --clobber

SNAPSHOT_URL="https://github.com/benoitpetit/mira/releases/download/${TAG}/mira-benchmark-${TAG}.json"
mkdir -p "$SITE_ROOT/public/data/benchmarks"
"${GO:-go}" run -tags fts5 ./cmd/mira-benchmark export-site \
  --report "$REPORT" --out "$SITE_ROOT/public/data/benchmarks/current.json" \
  --source-url "$SNAPSHOT_URL"
"${GO:-go}" run -tags fts5 ./cmd/mira-benchmark export-site \
  --report "$REPORT" --out "$SITE_ROOT/public/data/benchmarks/${TAG}.json" \
  --source-url "$SNAPSHOT_URL"

if [[ -n "${MIRA_BENCH_POSTGRES_URL:-}" ]]; then
  "${GO:-go}" run -tags fts5 ./cmd/mira-benchmark run \
    --track all --config benchmarks/config.postgres.json --database-url "$MIRA_BENCH_POSTGRES_URL" --output "$POSTGRES_REPORT" \
    --warmups "${BENCH_WARMUPS:-2}" --repetitions "${BENCH_REPETITIONS:-5}"
  "${GO:-go}" run -tags fts5 ./cmd/mira-benchmark validate --report "$POSTGRES_REPORT" --official
  gh release upload "$TAG" "$POSTGRES_REPORT" --clobber
  POSTGRES_SNAPSHOT_URL="https://github.com/benoitpetit/mira/releases/download/${TAG}/mira-benchmark-${TAG}-postgres.json"
  "${GO:-go}" run -tags fts5 ./cmd/mira-benchmark export-site \
    --report "$POSTGRES_REPORT" --out "$SITE_ROOT/public/data/benchmarks/${TAG}-postgres.json" \
    --source-url "$POSTGRES_SNAPSHOT_URL"
fi

npm --prefix "$SITE_ROOT" test
npm --prefix "$SITE_ROOT" run lint
npm --prefix "$SITE_ROOT" run build

echo "Benchmark reports for $TAG are attached to the GitHub release. Site snapshots are ready in $SITE_ROOT/public/data/benchmarks."
