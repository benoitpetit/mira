#!/bin/bash
# MIRA Performance Benchmark Script
# Runs comprehensive benchmarks for HNSW vector search

set -e

echo "╔══════════════════════════════════════════════════════════════╗"
echo "║                MIRA Performance Benchmarks                   ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""

cd "$(dirname "$0")/.."

echo "Running HNSW Vector Search Benchmarks..."
echo "Note: This may take a few minutes for accurate results"
echo ""

# Run all benchmarks with single pass, longer benchtime for stability
echo "Running all benchmarks (single pass, 1s each)..."
echo ""

# Capture output to file while displaying it
OUTPUT_FILE=$(mktemp)
trap "rm -f $OUTPUT_FILE" EXIT

go test ./internal/adapters/vector/... -bench=. -benchmem -benchtime=1s -run=^$ 2>&1 | \
    tee "$OUTPUT_FILE"

echo ""
echo "═══════════════════════════════════════════════════════════════"
echo "Benchmarks complete!"
echo ""

# Convert to JSON for visualization
cat "$OUTPUT_FILE" | go run scripts/benchmark_to_json.go > benchmark_results.json 2>/dev/null || true

if [ -f benchmark_results.json ] && [ -s benchmark_results.json ]; then
    echo "✓ Results saved to: benchmark_results.json"
    echo "  Load this file in scripts/benchmark.html to visualize"
else
    echo "To save results for visualization, run:"
    echo "  go test ./internal/adapters/vector/... -bench=. -benchmem -benchtime=1s -run=^$ 2>&1 | \\"
    echo "    go run scripts/benchmark_to_json.go > benchmark_results.json"
fi

echo "═══════════════════════════════════════════════════════════════"
