package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/riccardomenegazzo/workload-abi/internal/compat"
	"github.com/riccardomenegazzo/workload-abi/internal/diff"
	"github.com/riccardomenegazzo/workload-abi/internal/dockercli"
	"github.com/riccardomenegazzo/workload-abi/internal/model"
	"github.com/riccardomenegazzo/workload-abi/internal/observe"
	"github.com/riccardomenegazzo/workload-abi/internal/report"
	"github.com/riccardomenegazzo/workload-abi/internal/scenario"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "record":
		return recordCmd(args[1:])
	case "diff":
		return diffCmd(args[1:])
	case "compare", "check":
		return compareCmd(args[1:])
	case "version", "--version", "-v":
		fmt.Println("wabi", version)
		return 0
	case "help", "--help", "-h":
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
		usage()
		return 2
	}
}

func recordCmd(args []string) int {
	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	scenarioPath := fs.String("scenario", "", "scenario JSON file")
	output := fs.String("output", "", "write RuntimeGraph JSON to file")
	timeout := fs.Duration("timeout", 2*time.Minute, "overall recording timeout")
	if err := fs.Parse(interspersed(args)); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: wabi record IMAGE [--scenario file.json]")
		return 2
	}

	sc, err := scenario.Load(*scenarioPath)
	if err != nil {
		return fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	g, err := observe.New(dockercli.New(), os.Stderr).Record(ctx, fs.Arg(0), sc)
	if err != nil {
		return fail(err)
	}
	b, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return fail(fmt.Errorf("encode RuntimeGraph: %w", err))
	}
	if *output != "" {
		if err := writeFile(*output, b); err != nil {
			return fail(err)
		}
		fmt.Fprintln(os.Stderr, "wrote", *output)
	} else {
		fmt.Println(string(b))
	}
	return 0
}

func diffCmd(args []string) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.String("json", "", "write machine-readable diff JSON")
	if err := fs.Parse(interspersed(args)); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: wabi diff OLD_GRAPH.json NEW_GRAPH.json")
		return 2
	}

	a, err := loadGraph(fs.Arg(0))
	if err != nil {
		return fail(err)
	}
	b, err := loadGraph(fs.Arg(1))
	if err != nil {
		return fail(err)
	}
	d := diff.Compare(a, b)
	res := model.CompareResult{SchemaVersion: model.SchemaVersion, Diff: d}
	report.CompareText(os.Stdout, res)
	if *jsonOut != "" {
		raw, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return fail(fmt.Errorf("encode result: %w", err))
		}
		if err := writeFile(*jsonOut, raw); err != nil {
			return fail(err)
		}
	}
	return 0
}

func compareCmd(args []string) int {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	scenarioPath := fs.String("scenario", "", "scenario JSON applied identically to both images")
	target := fs.String("target", "", "Docker Compose file used as target environment")
	service := fs.String("service", "", "Compose service to evaluate")
	jsonOut := fs.String("json", "", "write complete result JSON")
	graphsDir := fs.String("graphs-dir", "", "optionally persist captured runtime graphs")
	failOn := fs.String("fail-on", "breaking", "breaking, degraded, or never")
	timeout := fs.Duration("timeout", 5*time.Minute, "overall compare timeout")
	if err := fs.Parse(interspersed(args)); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: wabi compare IMAGE_V1 IMAGE_V2 [--scenario file.json] [--target compose.yaml]")
		return 2
	}
	if !validFailOn(*failOn) {
		fmt.Fprintf(os.Stderr, "wabi: invalid --fail-on value %q; expected breaking, degraded, or never\n", *failOn)
		return 2
	}

	sc, err := scenario.Load(*scenarioPath)
	if err != nil {
		return fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	rec := observe.New(dockercli.New(), os.Stderr)

	fmt.Fprintln(os.Stderr, "[1/2] recording baseline", fs.Arg(0))
	a, err := rec.Record(ctx, fs.Arg(0), sc)
	if err != nil {
		return fail(err)
	}
	fmt.Fprintln(os.Stderr, "[2/2] recording candidate", fs.Arg(1))
	b, err := rec.Record(ctx, fs.Arg(1), sc)
	if err != nil {
		return fail(err)
	}

	if *graphsDir != "" {
		if err := writeGraphs(*graphsDir, a, b); err != nil {
			return fail(err)
		}
	}

	d := diff.Compare(a, b)
	res := model.CompareResult{SchemaVersion: model.SchemaVersion, Diff: d}
	if *target != "" {
		c, err := compat.EvaluateCompose(ctx, dockercli.New(), *target, *service, a, b, d)
		if err != nil {
			return fail(err)
		}
		res.Compatibility = &c
	}
	report.CompareText(os.Stdout, res)

	if *jsonOut != "" {
		raw, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return fail(fmt.Errorf("encode result: %w", err))
		}
		if err := writeFile(*jsonOut, raw); err != nil {
			return fail(err)
		}
	}
	return exitFor(res, *failOn)
}

func interspersed(args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-h" || a == "--help" {
			flags = append(flags, a)
			continue
		}
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			if !strings.Contains(a, "=") && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positional = append(positional, a)
	}
	return append(flags, positional...)
}

func validFailOn(mode string) bool {
	switch strings.ToLower(mode) {
	case "breaking", "degraded", "never":
		return true
	default:
		return false
	}
}

func exitFor(r model.CompareResult, mode string) int {
	mode = strings.ToLower(mode)
	if mode == "never" {
		return 0
	}
	status := model.StatusCompatible
	if r.Compatibility != nil {
		status = r.Compatibility.Status
	} else if r.Diff.Summary.Critical > 0 || r.Diff.Summary.High > 0 {
		status = model.StatusDegraded
	}
	if mode == "degraded" && (status == model.StatusDegraded || status == model.StatusBreaking) {
		return 3
	}
	if mode == "breaking" && status == model.StatusBreaking {
		return 4
	}
	return 0
}

func loadGraph(path string) (model.RuntimeGraph, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return model.RuntimeGraph{}, fmt.Errorf("read RuntimeGraph %q: %w", path, err)
	}
	var g model.RuntimeGraph
	if err := json.Unmarshal(b, &g); err != nil {
		return g, fmt.Errorf("decode RuntimeGraph %q: %w", path, err)
	}
	return g, nil
}

func writeGraphs(dir string, baseline, candidate model.RuntimeGraph) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create graphs directory: %w", err)
	}
	for name, graph := range map[string]model.RuntimeGraph{"baseline.json": baseline, "candidate.json": candidate} {
		b, err := json.MarshalIndent(graph, "", "  ")
		if err != nil {
			return fmt.Errorf("encode %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

func writeFile(path string, b []byte) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}

func fail(err error) int {
	if errors.Is(err, context.DeadlineExceeded) {
		fmt.Fprintln(os.Stderr, "wabi: operation timed out")
	} else {
		fmt.Fprintln(os.Stderr, "wabi:", err)
	}
	return 1
}

func usage() {
	fmt.Print(`Workload ABI (wabi) — detect operational breaking changes between container releases.

Usage:
  wabi compare IMAGE_V1 IMAGE_V2 [options]
  wabi record IMAGE [options]
  wabi diff OLD_GRAPH.json NEW_GRAPH.json [options]
  wabi version

Examples:
  wabi compare app:v1 app:v2 --scenario scenario.json
  wabi compare app:v1 app:v2 --scenario scenario.json --target compose.yaml
  wabi record app:v1 --output graph.json
`)
}
