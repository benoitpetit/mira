package main

import (
	"context"
	"fmt"
	"os"

	"github.com/benoitpetit/mira/internal/benchmark"
)

func main() {
	if err := benchmark.RunBenchmarkCommand(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "mira-benchmark:", err)
		os.Exit(2)
	}
}
