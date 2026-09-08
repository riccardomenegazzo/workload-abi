package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/riccardomenegazzo/workload-abi/internal/compat"
	"github.com/riccardomenegazzo/workload-abi/internal/diff"
	"github.com/riccardomenegazzo/workload-abi/internal/docker"
	"github.com/riccardomenegazzo/workload-abi/internal/report"
	"github.com/riccardomenegazzo/workload-abi/internal/scenario"
	"github.com/riccardomenegazzo/workload-abi/internal/target"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "compare":
		os.Exit(runCompare(os.Args[2:]))
	case "record":
		os.Exit(runRecord(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Println("wabi", version)
	default:
		usage()
		os.Exit(2)
	}
}

func runCompare(args []string) int {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	observe := fs.Duration("observe", 2*time.Second, "observation window for each container")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	failOnChange := fs.Bool("fail-on-change", false, "exit with status 3 when any runtime change is found")
	targetFile := fs.String("target", "", "Docker Compose file used as the target environment")
	service := fs.String("service", "", "Compose service to evaluate (required when the file contains multiple services)")
	scenarioFile := fs.String("scenario", "", "JSON scenario applied identically to both releases")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: wabi compare [flags] BASELINE_IMAGE CANDIDATE_IMAGE")
		return 2
	}

	opts, scenarioName, err := recorderOptions(*observe, *scenarioFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "scenario:", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), *observe*2+2*time.Minute)
	defer cancel()

	r := docker.NewRecorder()
	if err := r.Check(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "wabi:", err)
		return 1
	}

	base, err := r.CollectWithOptions(ctx, fs.Arg(0), opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "baseline:", err)
		return 1
	}
	candidate, err := r.CollectWithOptions(ctx, fs.Arg(1), opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "candidate:", err)
		return 1
	}
	base.Scenario = scenarioName
	candidate.Scenario = scenarioName

	comparison := diff.Compare(base, candidate)
	comparison.Scenario = scenarioName
	if *targetFile != "" {
		t, err := target.LoadCompose(ctx, *targetFile, *service)
		if err != nil {
			fmt.Fprintln(os.Stderr, "target:", err)
			return 1
		}
		comparison = compat.ApplyCompose(comparison, base, candidate, t)
	}

	if *jsonOut {
		_ = report.JSON(os.Stdout, comparison)
	} else {
		_ = report.Human(os.Stdout, comparison)
	}

	if comparison.Verdict == "BREAKING" {
		return 4
	}
	if *failOnChange && len(comparison.Changes) > 0 {
		return 3
	}
	return 0
}

func runRecord(args []string) int {
	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	observe := fs.Duration("observe", 2*time.Second, "observation window")
	scenarioFile := fs.String("scenario", "", "JSON scenario applied to the workload")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: wabi record [flags] IMAGE")
		return 2
	}

	opts, scenarioName, err := recorderOptions(*observe, *scenarioFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "scenario:", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), *observe+time.Minute)
	defer cancel()
	r := docker.NewRecorder()
	if err := r.Check(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "wabi:", err)
		return 1
	}
	snapshot, err := r.CollectWithOptions(ctx, fs.Arg(0), opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wabi:", err)
		return 1
	}
	snapshot.Scenario = scenarioName
	if err := report.JSON(os.Stdout, snapshot); err != nil {
		fmt.Fprintln(os.Stderr, "wabi:", err)
		return 1
	}
	return 0
}

func recorderOptions(observe time.Duration, scenarioFile string) (docker.Options, string, error) {
	opts := docker.Options{Observe: observe}
	if scenarioFile == "" {
		return opts, "", nil
	}
	s, err := scenario.Load(scenarioFile)
	if err != nil {
		return opts, "", err
	}
	opts.Environment = s.EnvList()
	opts.Command = append([]string(nil), s.Command...)
	name := s.Name
	if name == "" {
		name = scenarioFile
	}
	return opts, name, nil
}

func usage() {
	fmt.Fprintln(os.Stderr, `Workload ABI (wabi)

Discover operational breaking changes between container releases.

Commands:
  wabi compare [flags] BASELINE_IMAGE CANDIDATE_IMAGE
  wabi record  [flags] IMAGE
  wabi version

Equivalent experiment:
  wabi compare --scenario scenario.json BASELINE CANDIDATE

Target-aware comparison:
  wabi compare --target compose.yaml --service api BASELINE CANDIDATE

Exit codes for compare:
  0  compatible/no fatal error
  3  changes found when --fail-on-change is enabled
  4  breaking runtime change detected`)
}
