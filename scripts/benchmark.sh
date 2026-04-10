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

# Run basic HNSW benchmarks
echo "1. Basic HNSW Operations (Add)"
go test ./internal/adapters/vector/... -bench=^BenchmarkHNSWAdd$ -benchtime=500ms -count=3

echo ""
echo "2. Search Performance"
go test ./internal/adapters/vector/... -bench=^BenchmarkHNSWSearch$ -benchtime=500ms -count=3

echo ""
echo "3. Scalability Tests (various dataset sizes)"
echo "   - Size 100:"
go test ./internal/adapters/vector/... -bench=BenchmarkHNSWSearchScalability/size_100$ -benchtime=500ms -count=2
echo "   - Size 1000:"
go test ./internal/adapters/vector/... -bench=BenchmarkHNSWSearchScalability/size_1000$ -benchtime=500ms -count=2
echo "   - Size 10000:"
go test ./internal/adapters/vector/... -bench=BenchmarkHNSWSearchScalability/size_10000$ -benchtime=1s -count=2

echo ""
echo "4. Concurrent Access (parallel searches)"
go test ./internal/adapters/vector/... -bench=^BenchmarkHNSWConcurrentAccess$ -benchtime=1s -count=2

echo ""
echo "5. Build Time (index construction)"
go test ./internal/adapters/vector/... -bench=^BenchmarkHNSWBuildTime$ -benchtime=500ms -count=3

echo ""
echo "═══════════════════════════════════════════════════════════════"
echo "Benchmarks complete!"
echo ""
echo "To run all benchmarks with detailed output:"
echo "  go test ./internal/adapters/vector/... -bench=. -benchmem -benchtime=2s"
