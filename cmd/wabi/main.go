package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/riccardomenegazzo/workload-abi/internal/compat"
	"github.com/riccardomenegazzo/workload-abi/internal/diff"
	"github.com/riccardomenegazzo/workload-abi/internal/docker"
	"github.com/riccardomenegazzo/workload-abi/internal/model"
	"github.com/riccardomenegazzo/workload-abi/internal/policy"
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
	observe := fs.Duration("observe", 2*time.Second, "minimum experiment observation window for each container")
	outputFormat := fs.String("format", "human", "output format: human, json, or sarif")
	jsonOut := fs.Bool("json", false, "deprecated alias for --format json")
	failOnChange := fs.Bool("fail-on-change", false, "exit with status 3 when any runtime change is found")
	targetFile := fs.String("target", "", "target environment file (Docker Compose or Kubernetes)")
	targetKind := fs.String("target-kind", "auto", "target type: auto, compose, or kubernetes")
	service := fs.String("service", "", "Compose service to evaluate")
	workload := fs.String("workload", "", "Kubernetes workload to evaluate")
	containerName := fs.String("container", "", "Kubernetes container to evaluate")
	scenarioFile := fs.String("scenario", "", "JSON scenario applied identically to both releases")
	policyFile := fs.String("policy", "", "JSON compatibility policy")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: wabi compare [flags] BASELINE_IMAGE CANDIDATE_IMAGE")
		return 2
	}
	format, err := resolveFormat(*outputFormat, *jsonOut)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wabi:", err)
		return 2
	}

	opts, scenarioName, scenarioBudget, err := recorderOptions(*observe, *scenarioFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "scenario:", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), *observe*2+scenarioBudget*2+2*time.Minute)
	defer cancel()

	r := docker.NewRecorder()
	if err := r.Check(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "wabi:", err)
		return 1
	}

	base, err := r.CollectExperiment(ctx, fs.Arg(0), opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "baseline:", err)
		return 1
	}
	candidate, err := r.CollectExperiment(ctx, fs.Arg(1), opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "candidate:", err)
		return 1
	}
	base.Scenario = scenarioName
	candidate.Scenario = scenarioName
	base.Fingerprint = model.Fingerprint(base)
	candidate.Fingerprint = model.Fingerprint(candidate)

	comparison := diff.Compare(base, candidate)
	comparison.Scenario = scenarioName

	if *targetFile != "" {
		comparison, err = applyTarget(ctx, comparison, base, candidate, *targetFile, *targetKind, *service, *workload, *containerName)
		if err != nil {
			fmt.Fprintln(os.Stderr, "target:", err)
			return 1
		}
	}

	if *policyFile != "" {
		p, err := policy.Load(*policyFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "policy:", err)
			return 1
		}
		comparison = policy.Apply(comparison, p)
	}

	if err := writeComparison(os.Stdout, format, comparison); err != nil {
		fmt.Fprintln(os.Stderr, "wabi:", err)
		return 1
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
	observe := fs.Duration("observe", 2*time.Second, "minimum experiment observation window")
	scenarioFile := fs.String("scenario", "", "JSON scenario applied to the workload")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: wabi record [flags] IMAGE")
		return 2
	}

	opts, scenarioName, scenarioBudget, err := recorderOptions(*observe, *scenarioFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "scenario:", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), *observe+scenarioBudget+time.Minute)
	defer cancel()
	r := docker.NewRecorder()
	if err := r.Check(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "wabi:", err)
		return 1
	}
	snapshot, err := r.CollectExperiment(ctx, fs.Arg(0), opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wabi:", err)
		return 1
	}
	snapshot.Scenario = scenarioName
	snapshot.Fingerprint = model.Fingerprint(snapshot)
	if err := report.JSON(os.Stdout, snapshot); err != nil {
		fmt.Fprintln(os.Stderr, "wabi:", err)
		return 1
	}
	return 0
}

func recorderOptions(observe time.Duration, scenarioFile string) (docker.ExperimentOptions, string, time.Duration, error) {
	opts := docker.ExperimentOptions{Observe: observe}
	if scenarioFile == "" {
		return opts, "", 0, nil
	}
	s, err := scenario.Load(scenarioFile)
	if err != nil {
		return opts, "", 0, err
	}
	plans, err := s.Plans()
	if err != nil {
		return opts, "", 0, err
	}
	opts.Environment = s.EnvList()
	opts.Command = append([]string(nil), s.Command...)
	var budget time.Duration
	for _, plan := range plans {
		opts.Steps = append(opts.Steps, docker.ExperimentStep{
			Name:         plan.Name,
			After:        plan.After,
			Command:      append([]string(nil), plan.Command...),
			Timeout:      plan.Timeout,
			AllowFailure: plan.AllowFailure,
		})
		budget += plan.After + plan.Timeout
	}
	name := s.Name
	if name == "" {
		name = scenarioFile
	}
	return opts, name, budget, nil
}

func applyTarget(
	ctx context.Context,
	c model.Comparison,
	base, candidate model.Snapshot,
	file, kind, service, workload, containerName string,
) (model.Comparison, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" || kind == "auto" {
		detected, err := target.Detect(file)
		if err != nil {
			return c, err
		}
		kind = detected
	}
	switch kind {
	case "compose":
		t, err := target.LoadCompose(ctx, file, service)
		if err != nil {
			return c, err
		}
		return compat.ApplyCompose(c, base, candidate, t), nil
	case "kubernetes", "k8s":
		t, err := target.LoadKubernetes(ctx, file, workload, containerName)
		if err != nil {
			return c, err
		}
		return compat.ApplyKubernetes(c, base, candidate, t), nil
	default:
		return c, fmt.Errorf("unsupported target kind %q", kind)
	}
}

func resolveFormat(format string, jsonAlias bool) (string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if jsonAlias {
		if format != "" && format != "human" && format != "json" {
			return "", fmt.Errorf("--json cannot be combined with --format %s", format)
		}
		format = "json"
	}
	switch format {
	case "", "human":
		return "human", nil
	case "json", "sarif":
		return format, nil
	default:
		return "", fmt.Errorf("unsupported output format %q", format)
	}
}

func writeComparison(out *os.File, format string, comparison model.Comparison) error {
	switch format {
	case "json":
		return report.JSON(out, comparison)
	case "sarif":
		return report.SARIF(out, comparison)
	default:
		return report.Human(out, comparison)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Workload ABI (wabi)

Discover operational breaking changes between container releases.

Commands:
  wabi compare [flags] BASELINE_IMAGE CANDIDATE_IMAGE
  wabi record  [flags] IMAGE
  wabi version

Equivalent multi-phase experiment:
  wabi compare --scenario scenario.json BASELINE CANDIDATE

Target-aware comparison:
  wabi compare --target compose.yaml --service api BASELINE CANDIDATE
  wabi compare --target deployment.json --target-kind kubernetes --container api BASELINE CANDIDATE

Policy gate:
  wabi compare --scenario scenario.json --target compose.yaml --policy policy.json BASELINE CANDIDATE

Machine-readable output:
  wabi compare --format json BASELINE CANDIDATE
  wabi compare --format sarif BASELINE CANDIDATE

Exit codes for compare:
  0  compatible/no fatal error
  3  changes found when --fail-on-change is enabled
  4  breaking runtime, target, scenario, or policy regression detected`)
}
