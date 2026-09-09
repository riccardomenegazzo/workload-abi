package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/riccardomenegazzo/workload-abi/internal/graph"
	"github.com/riccardomenegazzo/workload-abi/internal/report"
	"github.com/riccardomenegazzo/workload-abi/internal/snapshot"
)

func runGraph(args []string) int {
	fs := flag.NewFlagSet("graph", flag.ContinueOnError)
	output := fs.String("output", "", "write causal graph JSON to a file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: wabi graph [--output FILE] SNAPSHOT.json")
		return 2
	}

	s, err := snapshot.Load(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "graph:", err)
		return 1
	}
	if len(s.RuntimeEvents) == 0 {
		fmt.Fprintln(os.Stderr, "graph: snapshot contains no deep runtime events; enrich it first")
		return 1
	}
	artifact := graph.Build(s)
	out := os.Stdout
	if *output != "" {
		f, err := os.Create(*output)
		if err != nil {
			fmt.Fprintln(os.Stderr, "graph:", err)
			return 1
		}
		defer f.Close()
		out = f
	}
	if err := report.JSON(out, artifact); err != nil {
		fmt.Fprintln(os.Stderr, "graph:", err)
		return 1
	}
	return 0
}

func runGraphDiff(args []string) int {
	fs := flag.NewFlagSet("graph-diff", flag.ContinueOnError)
	format := fs.String("format", "human", "output format: human or json")
	failOnChange := fs.Bool("fail-on-change", false, "exit with status 3 when the causal graph changes")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: wabi graph-diff [flags] BASELINE.graph.json CANDIDATE.graph.json")
		return 2
	}
	base, err := graph.Load(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "baseline graph:", err)
		return 1
	}
	candidate, err := graph.Load(fs.Arg(1))
	if err != nil {
		fmt.Fprintln(os.Stderr, "candidate graph:", err)
		return 1
	}
	if base.Scenario != candidate.Scenario {
		fmt.Fprintf(os.Stderr, "graph-diff: graphs were derived from different scenarios (%q vs %q)\n", base.Scenario, candidate.Scenario)
		return 1
	}

	d := graph.Compare(base, candidate)
	switch *format {
	case "json":
		if err := report.JSON(os.Stdout, d); err != nil {
			fmt.Fprintln(os.Stderr, "graph-diff:", err)
			return 1
		}
	case "human":
		fmt.Println("CAUSAL RUNTIME GRAPH")
		fmt.Println("================================================================")
		fmt.Printf("%s -> %s\n", base.Image, candidate.Image)
		fmt.Printf("baseline graph:  %s\n", base.Fingerprint)
		fmt.Printf("candidate graph: %s\n", candidate.Fingerprint)
		if len(d.Explanations) > 0 {
			fmt.Println("\nCAUSAL EXPLANATIONS")
			for _, explanation := range d.Explanations {
				fmt.Printf("  • %s\n", explanation.Summary)
			}
		}
		if len(d.Changes) > 0 {
			fmt.Println("\nEDGE CHANGES")
			for _, change := range d.Changes {
				marker := "+"
				if change.Kind == "removed" {
					marker = "-"
				}
				fmt.Printf("  %s %s --%s--> %s\n", marker, change.Edge.From, change.Edge.Relation, change.Edge.To)
			}
		}
		fmt.Printf("\nCAUSAL GRAPH: %s\n", d.Verdict)
	default:
		fmt.Fprintf(os.Stderr, "graph-diff: unsupported output format %q\n", *format)
		return 2
	}
	if *failOnChange && len(d.Changes) > 0 {
		return 3
	}
	return 0
}
