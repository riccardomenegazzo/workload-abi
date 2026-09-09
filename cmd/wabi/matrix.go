package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/riccardomenegazzo/workload-abi/internal/matrix"
	"github.com/riccardomenegazzo/workload-abi/internal/report"
	"github.com/riccardomenegazzo/workload-abi/internal/snapshot"
)

func runMatrix(args []string) int {
	fs := flag.NewFlagSet("matrix", flag.ContinueOnError)
	configFile := fs.String("config", "", "environment matrix configuration JSON")
	output := fs.String("output", "", "persist the versioned matrix artifact as JSON")
	format := fs.String("format", "human", "stdout format: human or json")
	failOnChange := fs.Bool("fail-on-change", false, "exit with status 3 when the matrix is CHANGED but not BREAKING")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configFile == "" || fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: wabi matrix --config ENVIRONMENTS.json [flags] BASELINE.json CANDIDATE.json")
		return 2
	}
	stdoutFormat := strings.ToLower(strings.TrimSpace(*format))
	if stdoutFormat != "human" && stdoutFormat != "json" {
		fmt.Fprintf(os.Stderr, "matrix: unsupported format %q\n", *format)
		return 2
	}

	base, err := snapshot.Load(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "baseline snapshot:", err)
		return 1
	}
	candidate, err := snapshot.Load(fs.Arg(1))
	if err != nil {
		fmt.Fprintln(os.Stderr, "candidate snapshot:", err)
		return 1
	}
	if base.Scenario != candidate.Scenario {
		fmt.Fprintf(os.Stderr, "matrix: snapshots were recorded under different scenarios (%q vs %q)\n", base.Scenario, candidate.Scenario)
		return 1
	}
	config, err := matrix.LoadConfig(*configFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "matrix:", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	artifact, err := matrix.Build(ctx, base, candidate, config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "matrix:", err)
		return 1
	}

	if *output != "" {
		f, err := os.Create(*output)
		if err != nil {
			fmt.Fprintln(os.Stderr, "matrix:", err)
			return 1
		}
		if err := report.JSON(f, artifact); err != nil {
			_ = f.Close()
			fmt.Fprintln(os.Stderr, "matrix:", err)
			return 1
		}
		if err := f.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "matrix:", err)
			return 1
		}
	}

	if stdoutFormat == "json" {
		if err := report.JSON(os.Stdout, artifact); err != nil {
			fmt.Fprintln(os.Stderr, "matrix:", err)
			return 1
		}
	} else if err := matrix.WriteHuman(os.Stdout, artifact); err != nil {
		fmt.Fprintln(os.Stderr, "matrix:", err)
		return 1
	}

	if artifact.Verdict == "BREAKING" {
		return 4
	}
	if *failOnChange && artifact.Verdict == "CHANGED" {
		return 3
	}
	return 0
}

func runVerifyMatrix(args []string) int {
	fs := flag.NewFlagSet("verify-matrix", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: wabi verify-matrix MATRIX.json")
		return 2
	}
	artifact, err := matrix.LoadArtifact(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "verify-matrix:", err)
		return 1
	}
	fmt.Printf("verified: %s -> %s %s %s\n", artifact.Baseline, artifact.Candidate, artifact.Verdict, artifact.Fingerprint)
	return 0
}
