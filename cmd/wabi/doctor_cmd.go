package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/riccardomenegazzo/workload-abi/internal/doctor"
	"github.com/riccardomenegazzo/workload-abi/internal/report"
)

func runDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: wabi doctor [--json]")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result := doctor.Run(ctx)
	if *jsonOut {
		if err := report.JSON(os.Stdout, result); err != nil {
			return 1
		}
		return 0
	}
	for _, check := range result.Checks {
		status := "MISSING"
		if check.Available {
			status = "OK"
		}
		fmt.Printf("%-16s %-7s %s\n", check.Component, status, firstNonEmpty(check.Version, check.Error))
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
