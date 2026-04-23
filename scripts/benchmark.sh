#!/bin/bash
# MIRA Performance Benchmark Suite
# Comprehensive HNSW vector search benchmarking with visualization export

set -e

BENCHTIME="${BENCHTIME:-1s}"
OUTPUT_DIR="${OUTPUT_DIR:-.}"

echo "╔════════════════════════════════════════════════════════════════╗"
echo "║               MIRA Performance Benchmark Suite                 ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo ""

cd "$(dirname "$0")/.."

echo "Configuration:"
echo "  Benchmark time: $BENCHTIME per test"
echo "  Output directory: $OUTPUT_DIR"
echo ""

mkdir -p "$OUTPUT_DIR"

echo "Running HNSW Vector Search Benchmarks..."
echo "This will test search performance across various dataset sizes"
echo ""

OUTPUT_FILE=$(mktemp)
trap "rm -f $OUTPUT_FILE" EXIT

echo "Executing benchmarks (this may take a few minutes)..."
echo ""

go test ./internal/adapters/vector/... \
    -bench=. \
    -benchmem \
    -benchtime="$BENCHTIME" \
    -run=^$ 2>&1 | tee "$OUTPUT_FILE"

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "                    Benchmark Summary"
echo "════════════════════════════════════════════════════════════════"

# Generate JSON
JSON_FILE="$OUTPUT_DIR/benchmark_results.json"
go run scripts/benchmark_to_json.go < "$OUTPUT_FILE" > "$JSON_FILE" 2>/dev/null || true

echo ""
echo "Key Metrics:"
echo ""

# Extract metrics from JSON using grep/sed (portable, no jq dependency)
if [ -f "$JSON_FILE" ] && [ -s "$JSON_FILE" ]; then
    # Search latency (basic search, not scalability)
    SEARCH_NS=$(grep -A5 '"Name": "BenchmarkHNSWSearch-' "$JSON_FILE" | grep '"NsPerOp"' | head -1 | sed 's/[^0-9]//g')
    
    # Add operation
    ADD_NS=$(grep -A5 '"Name": "BenchmarkHNSWAdd-' "$JSON_FILE" | grep '"NsPerOp"' | head -1 | sed 's/[^0-9]//g')
    
    # Scalability tests
    SIZE_100=$(grep -A5 'size_100-' "$JSON_FILE" | grep '"NsPerOp"' | head -1 | sed 's/[^0-9]//g')
    SIZE_10K=$(grep -A5 'size_10000-' "$JSON_FILE" | grep '"NsPerOp"' | head -1 | sed 's/[^0-9]//g')
    
    # Display metrics
    if [ -n "$SEARCH_NS" ]; then
        SEARCH_QPS=$((1000000000 / SEARCH_NS))
        echo "  • Search Latency:    ${SEARCH_NS} ns/op (~${SEARCH_QPS} qps)"
    fi
    
    if [ -n "$ADD_NS" ]; then
        echo "  • Add Operation:     ${ADD_NS} ns/op"
    fi
    
    if [ -n "$SIZE_100" ] && [ -n "$SIZE_10K" ] && [ "$SIZE_100" -gt 0 ]; then
        # Calculate scale factor using awk
        SCALE_FACTOR=$(awk "BEGIN {printf \"%.2f\", $SIZE_10K / $SIZE_100}")
        echo "  • Scalability:       ${SCALE_FACTOR}x slower at 10K vs 100 vectors"
        echo "    (Linear O(n) scan would be ~100x)"
    fi
    
    BENCH_COUNT=$(grep -c '"Name"' "$JSON_FILE" 2>/dev/null || echo "0")
else
    echo "  Error: Could not parse benchmark results"
fi

echo ""
echo "════════════════════════════════════════════════════════════════"

if [ -f "$JSON_FILE" ] && [ -s "$JSON_FILE" ]; then
    echo ""
    echo "✓ Results exported:    $JSON_FILE"
    echo "  Benchmarks captured: $BENCH_COUNT"
    echo ""
else
    echo ""
    echo "To export results for visualization:"
    echo "  go test ./internal/adapters/vector/... -bench=. -benchmem -benchtime=1s -run=^$ 2>&1 | \\"
    echo "    go run scripts/benchmark_to_json.go > benchmark_results.json"
fi

echo ""
echo "════════════════════════════════════════════════════════════════"
