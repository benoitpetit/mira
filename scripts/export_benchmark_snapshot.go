package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/benoitpetit/mira/internal/benchmark"
)

// ExportBenchmarkSnapshot is shared by repository-local tooling that needs to
// create the static site artifact from a previously validated core report.
// The supported command-line interface is `mira-benchmark export-site`.
func ExportBenchmarkSnapshot(reportPath, outputPath, sourceURL string) error {
	reportBytes, err := os.ReadFile(reportPath)
	if err != nil {
		return fmt.Errorf("read report: %w", err)
	}
	output, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open snapshot output: %w", err)
	}
	writeErr := benchmark.ExportSiteSnapshot(bytes.NewReader(reportBytes), sourceURL, output)
	closeErr := output.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return nil
}
