// Converts Go benchmark text output to JSON format for visualization
// Usage: go test -bench=. -benchmem ./... 2>&1 | go run scripts/benchmark_to_json.go > benchmark_results.json
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

type BenchmarkResult struct {
	Name              string  `json:"Name"`
	NsPerOp           int64   `json:"NsPerOp,omitempty"`
	AllocedBytesPerOp int64   `json:"AllocedBytesPerOp,omitempty"`
	AllocsPerOp       int64   `json:"AllocsPerOp,omitempty"`
}

type BenchmarkOutput struct {
	Benchmarks []BenchmarkResult `json:"Benchmarks"`
}

func main() {
	var results []BenchmarkResult
	var currentBench *BenchmarkResult
	
	scanner := bufio.NewScanner(os.Stdin)
	
	// Regex patterns
	benchStartRegex := regexp.MustCompile(`^(Benchmark[\w/-]+)`)
	dataLineRegex := regexp.MustCompile(`^\s+(\d+)\s+([\d.]+)\s+ns/op(?:\s+([\d.]+)\s+B/op)?(?:\s+([\d.]+)\s+allocs/op)?`)
	logLineRegex := regexp.MustCompile(`^\d{4}/\d{2}/\d{2}`)
	
	for scanner.Scan() {
		line := scanner.Text()
		
		// Skip log lines (lines with timestamps)
		if logLineRegex.MatchString(line) {
			continue
		}
		
		// Check if this is a benchmark start line
		if matches := benchStartRegex.FindStringSubmatch(line); matches != nil {
			// Save previous benchmark if complete
			if currentBench != nil && currentBench.NsPerOp > 0 {
				results = append(results, *currentBench)
			}
			
			// Extract benchmark name (remove any trailing log content)
			name := strings.TrimSpace(matches[1])
			currentBench = &BenchmarkResult{Name: name}
			
			// Check if data is on the same line
			if dataMatches := dataLineRegex.FindStringSubmatch(line[len(name):]); dataMatches != nil {
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
	
	output := BenchmarkOutput{Benchmarks: results}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		os.Exit(1)
	}
}

func parseData(bench *BenchmarkResult, matches []string) {
	// matches[1] = iterations
	// matches[2] = ns/op
	// matches[3] = B/op (optional)
	// matches[4] = allocs/op (optional)
	
	if ns, err := strconv.ParseFloat(matches[2], 64); err == nil {
		bench.NsPerOp = int64(ns)
	}
	
	if len(matches) > 3 && matches[3] != "" {
		if bytes, err := strconv.ParseFloat(matches[3], 64); err == nil {
			bench.AllocedBytesPerOp = int64(bytes)
		}
	}
	
	if len(matches) > 4 && matches[4] != "" {
		if allocs, err := strconv.ParseFloat(matches[4], 64); err == nil {
			bench.AllocsPerOp = int64(allocs)
		}
	}
}
