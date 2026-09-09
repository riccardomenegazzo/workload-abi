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
	"github.com/riccardomenegazzo/workload-abi/internal/snapshot"
	"github.com/riccardomenegazzo/workload-abi/internal/target"
)

func runCompare(args []string) int {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	observe := fs.Duration("observe", 2*time.Second, "minimum experiment observation window for each container")
	outputFormat := fs.String("format", "human", "output format: human, json, or sarif")
	jsonOut := fs.Bool("json", false, "deprecated alias for --format json")
	failOnChange := fs.Bool("fail-on-change", false, "exit with status 3 when any runtime change is found")
	targetFile := fs.String("target", "", "target environment file (Docker Compose, Kubernetes, or ECS task definition)")
	targetKind := fs.String("target-kind", "auto", "target type: auto, compose, kubernetes, or ecs")
	service := fs.String("service", "", "Compose service to evaluate")
	workload := fs.String("workload", "", "Kubernetes workload to evaluate")
	containerName := fs.String("container", "", "Kubernetes or ECS container to evaluate")
	networkPolicyFile := fs.String("network-policy", "", "Kubernetes NetworkPolicy file used to prove observed egress compatibility")
	scenarioFile := fs.String("scenario", "", "JSON scenario applied identically to both releases")
	policyFile := fs.String("policy", "", "JSON compatibility policy")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: wabi compare [flags] BASELINE_IMAGE CANDIDATE_IMAGE")
		return 2
	}
	if *networkPolicyFile != "" && *targetFile == "" {
		fmt.Fprintln(os.Stderr, "wabi: --network-policy requires a Kubernetes --target workload")
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
		comparison, err = applyTarget(ctx, comparison, base, candidate, *targetFile, *targetKind, *service, *workload, *containerName, *networkPolicyFile)
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
	snapshotValue, err := r.CollectExperiment(ctx, fs.Arg(0), opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wabi:", err)
		return 1
	}
	snapshotValue.Scenario = scenarioName
	snapshotValue.Fingerprint = model.Fingerprint(snapshotValue)
	if err := report.JSON(os.Stdout, snapshotValue); err != nil {
		fmt.Fprintln(os.Stderr, "wabi:", err)
		return 1
	}
	return 0
}

func runCompareSnapshots(args []string) int {
	fs := flag.NewFlagSet("compare-snapshots", flag.ContinueOnError)
	outputFormat := fs.String("format", "human", "output format: human, json, or sarif")
	jsonOut := fs.Bool("json", false, "deprecated alias for --format json")
	failOnChange := fs.Bool("fail-on-change", false, "exit with status 3 when any change is found")
	targetFile := fs.String("target", "", "target environment file (Docker Compose, Kubernetes, or ECS task definition)")
	targetKind := fs.String("target-kind", "auto", "target type: auto, compose, kubernetes, or ecs")
	service := fs.String("service", "", "Compose service to evaluate")
	workload := fs.String("workload", "", "Kubernetes workload to evaluate")
	containerName := fs.String("container", "", "Kubernetes or ECS container to evaluate")
	networkPolicyFile := fs.String("network-policy", "", "Kubernetes NetworkPolicy file used to prove observed egress compatibility")
	seccompProfileFile := fs.String("seccomp-profile", "", "Docker/OCI seccomp profile used to prove exact observed syscall compatibility")
	policyFile := fs.String("policy", "", "JSON compatibility policy")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: wabi compare-snapshots [flags] BASELINE.json CANDIDATE.json")
		return 2
	}
	if *networkPolicyFile != "" && *targetFile == "" {
		fmt.Fprintln(os.Stderr, "wabi: --network-policy requires a Kubernetes --target workload")
		return 2
	}
	format, err := resolveFormat(*outputFormat, *jsonOut)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wabi:", err)
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
		fmt.Fprintf(os.Stderr, "wabi: snapshots were recorded under different scenarios (%q vs %q)\n", base.Scenario, candidate.Scenario)
		return 1
	}

	comparison := diff.Compare(base, candidate)
	comparison.Scenario = base.Scenario
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if *targetFile != "" {
		comparison, err = applyTarget(ctx, comparison, base, candidate, *targetFile, *targetKind, *service, *workload, *containerName, *networkPolicyFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "target:", err)
			return 1
		}
	}
	if *seccompProfileFile != "" {
		profile, err := target.LoadSeccompProfile(*seccompProfileFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "seccomp:", err)
			return 1
		}
		comparison = compat.ApplySeccomp(comparison, base, candidate, profile)
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
	file, kind, service, workload, containerName, networkPolicyFile string,
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
		if networkPolicyFile != "" {
			return c, fmt.Errorf("--network-policy is only valid with a Kubernetes target")
		}
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
		if networkPolicyFile != "" {
			policies, err := target.LoadKubernetesNetworkPolicies(ctx, networkPolicyFile)
			if err != nil {
				return c, err
			}
			t.NetworkPolicyFile = networkPolicyFile
			t.NetworkPolicies = policies
		}
		return compat.ApplyKubernetes(c, base, candidate, t), nil
	case "ecs":
		if networkPolicyFile != "" {
			return c, fmt.Errorf("--network-policy is only valid with a Kubernetes target")
		}
		t, err := target.LoadECS(file, containerName)
		if err != nil {
			return c, err
		}
		return compat.ApplyECS(c, base, candidate, t), nil
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
