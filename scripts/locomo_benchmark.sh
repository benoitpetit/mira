#!/usr/bin/env bash
# Run MIRA's deterministic LoCoMo-style recall benchmark and save its JSON
# report. Optional third-party values are accepted only from a user-supplied
# baseline file; this script never invents competitor measurements.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
OUTPUT_DIR="${OUTPUT_DIR:-${PROJECT_DIR}/benchmarks/results}"
REPORT_PATH="${MIRA_LOCOMO_REPORT:-${OUTPUT_DIR}/locomo_mira.json}"
BENCHTIME="${BENCHTIME:-10x}"
BASELINES="${MIRA_COMPARE_BASELINES:-}"

mkdir -p "$(dirname "${REPORT_PATH}")"
cd "${PROJECT_DIR}"

TEST_ARGS=()
if [[ -n "${BASELINES}" ]]; then
  if [[ ! -f "${BASELINES}" ]]; then
    echo "Baseline file not found: ${BASELINES}" >&2
    exit 1
  fi
  TEST_ARGS=(-args "-mira.compare=${BASELINES}")
fi

echo "Running deterministic LoCoMo-style MIRA recall benchmark..."
echo "  report: ${REPORT_PATH}"
echo "  benchtime: ${BENCHTIME}"
if [[ -n "${BASELINES}" ]]; then
  echo "  baselines: ${BASELINES} (user-supplied, not measured by this script)"
fi

MIRA_LOCOMO_REPORT="${REPORT_PATH}" go test -tags fts5 ./internal/usecases/interactors \
  -run='^$' -bench='^BenchmarkLoCoMoRecall$' -benchmem -count=1 \
  -benchtime="${BENCHTIME}" "${TEST_ARGS[@]}"

echo "Report written to ${REPORT_PATH}"
