package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/riccardomenegazzo/workload-abi/internal/evidence"
	"github.com/riccardomenegazzo/workload-abi/internal/report"
	"github.com/riccardomenegazzo/workload-abi/internal/snapshot"
)

func runEnrich(args []string) int {
	fs := flag.NewFlagSet("enrich", flag.ContinueOnError)
	snapshotFile := fs.String("snapshot", "", "persisted Workload ABI snapshot to enrich")
	eventsFile := fs.String("events", "", "runtime event file to normalize and merge")
	format := fs.String("format", "", "runtime event format: falco, tracee, or generic")
	output := fs.String("output", "", "write enriched snapshot to a file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *snapshotFile == "" || *eventsFile == "" || *format == "" {
		fmt.Fprintln(os.Stderr, "usage: wabi enrich --snapshot SNAPSHOT.json --events EVENTS.jsonl --format falco|tracee|generic [--output FILE]")
		return 2
	}

	s, err := snapshot.Load(*snapshotFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "enrich snapshot:", err)
		return 1
	}
	events, err := evidence.Load(*eventsFile, *format)
	if err != nil {
		fmt.Fprintln(os.Stderr, "enrich evidence:", err)
		return 1
	}
	if len(events) == 0 {
		fmt.Fprintln(os.Stderr, "enrich evidence: no normalizable runtime events found")
		return 1
	}
	enriched := evidence.Merge(s, events)

	out := os.Stdout
	if *output != "" {
		f, err := os.Create(*output)
		if err != nil {
			fmt.Fprintln(os.Stderr, "enrich output:", err)
			return 1
		}
		defer f.Close()
		out = f
	}
	if err := report.JSON(out, enriched); err != nil {
		fmt.Fprintln(os.Stderr, "enrich output:", err)
		return 1
	}
	return 0
}
