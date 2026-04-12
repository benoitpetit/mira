// Converts Go benchmark text output to JSON format for visualization
// Handles interleaved log output and multi-line benchmark results
//
// Usage:
//   go test -bench=. -benchmem ./... 2>&1 | go run scripts/benchmark_to_json.go > results.json
//   ./scripts/benchmark.sh
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// BenchmarkResult represents a single benchmark execution
type BenchmarkResult struct {
	Name              string  `json:"Name"`
	Iterations        int64   `json:"Iterations,omitempty"`
	NsPerOp           int64   `json:"NsPerOp,omitempty"`
	AllocedBytesPerOp int64   `json:"AllocedBytesPerOp,omitempty"`
	AllocsPerOp       int64   `json:"AllocsPerOp,omitempty"`
	MBPerSec          float64 `json:"MBPerSec,omitempty"`
}

// BenchmarkOutput is the root structure
type BenchmarkOutput struct {
	Benchmarks []BenchmarkResult `json:"Benchmarks"`
	Metadata   map[string]string `json:"Metadata,omitempty"`
}

func main() {
	var results []BenchmarkResult
	var metadata = make(map[string]string)
	var currentBench *BenchmarkResult
	
	scanner := bufio.NewScanner(os.Stdin)
	
	// Regex patterns
	benchStartRegex := regexp.MustCompile(`^(Benchmark[\w/-]+)(?:-\d+)?\s*`)
	dataLineRegex := regexp.MustCompile(`^\s*(\d+)\s+([\d.]+)\s+ns/op(?:\s+([\d.]+)\s+B/op)?(?:\s+([\d.]+)\s+allocs/op)?`)
	metaRegex := regexp.MustCompile(`^(goos|goarch|pkg|cpu):\s*(.+)`)
	logLineRegex := regexp.MustCompile(`^\d{4}/\d{2}/\d{2}`)
	
	for scanner.Scan() {
		line := scanner.Text()
		
		// Skip log lines (lines with timestamps)
		if logLineRegex.MatchString(line) {
			continue
		}
		
		// Parse metadata
		if matches := metaRegex.FindStringSubmatch(line); matches != nil {
			metadata[matches[1]] = matches[2]
			continue
		}
		
		// Check if this is a benchmark start line
		if matches := benchStartRegex.FindStringSubmatch(line); matches != nil {
			// Save previous benchmark if complete
			if currentBench != nil && currentBench.NsPerOp > 0 {
				results = append(results, *currentBench)
			}
			
			// Extract benchmark name (remove GOMAXPROCS suffix)
			name := strings.TrimSpace(matches[1])
			currentBench = &BenchmarkResult{Name: name}
			
			// Check if data is on the same line
			remaining := line[len(matches[0]):]
			if dataMatches := dataLineRegex.FindStringSubmatch(remaining); dataMatches != nil {
				parseData(currentBench, dataMatches)
			}
			continue
		}
		
		// Check if this is a data continuation line for current benchmark
		if currentBench != nil && currentBench.NsPerOp == 0 {
			if dataMatches := dataLineRegex.FindStringSubmatch(line); dataMatches != nil {
				parseData(currentBench, dataMatches)
			}
		}
	}
	
	// Don't forget the last benchmark
	if currentBench != nil && currentBench.NsPerOp > 0 {
		results = append(results, *currentBench)
	}
	
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}
	
	output := BenchmarkOutput{
		Benchmarks: results,
		Metadata:   metadata,
	}
	
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		os.Exit(1)
	}
}

func parseData(bench *BenchmarkResult, matches []string) {
	// Parse iterations
	if iter, err := strconv.ParseInt(matches[1], 10, 64); err == nil {
		bench.Iterations = iter
	}
	
	// Parse ns/op
	if ns, err := strconv.ParseFloat(matches[2], 64); err == nil {
		bench.NsPerOp = int64(ns)
	}
	
	// Parse B/op (optional)
	if len(matches) > 3 && matches[3] != "" {
		if bytes, err := strconv.ParseFloat(matches[3], 64); err == nil {
			bench.AllocedBytesPerOp = int64(bytes)
		}
	}
	
	// Parse allocs/op (optional)
	if len(matches) > 4 && matches[4] != "" {
		if allocs, err := strconv.ParseFloat(matches[4], 64); err == nil {
			bench.AllocsPerOp = int64(allocs)
		}
	}
}
