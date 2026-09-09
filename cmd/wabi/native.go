package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/riccardomenegazzo/workload-abi/internal/nativeebpf"
	"github.com/riccardomenegazzo/workload-abi/internal/report"
)

func runNativeRecord(args []string) int {
	fs := flag.NewFlagSet("native-record", flag.ContinueOnError)
	duration := fs.Duration("duration", 3*time.Second, "bounded native eBPF observation window")
	maxEvents := fs.Int("max-events", 10000, "maximum number of unique RuntimeEvents to retain")
	output := fs.String("output", "", "write provider-neutral RuntimeEvent JSON to a file instead of stdout")
	statsOutput := fs.String("stats-output", "", "write native recorder diagnostics and drop accounting as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: wabi native-record [--duration 3s] [--max-events N] [--output events.json] [--stats-output stats.json]")
		return 2
	}
	if *duration <= 0 || *maxEvents <= 0 {
		fmt.Fprintln(os.Stderr, "native-record: duration and max-events must be positive")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), *duration+10*time.Second)
	defer cancel()
	result, err := nativeebpf.Record(ctx, nativeebpf.Options{Duration: *duration, MaxEvents: *maxEvents})
	if err != nil {
		fmt.Fprintln(os.Stderr, "native-record:", err)
		return 1
	}

	out := os.Stdout
	if *output != "" {
		f, err := os.Create(*output)
		if err != nil {
			fmt.Fprintln(os.Stderr, "native-record:", err)
			return 1
		}
		defer f.Close()
		out = f
	}
	if err := report.JSON(out, result.Events); err != nil {
		fmt.Fprintln(os.Stderr, "native-record:", err)
		return 1
	}

	if *statsOutput != "" {
		f, err := os.Create(*statsOutput)
		if err != nil {
			fmt.Fprintln(os.Stderr, "native-record stats:", err)
			return 1
		}
		defer f.Close()
		if err := report.JSON(f, result.Stats); err != nil {
			fmt.Fprintln(os.Stderr, "native-record stats:", err)
			return 1
		}
	} else {
		fmt.Fprintf(os.Stderr, "native-record: captured=%d lost_samples=%d probes=%d\n", result.Stats.Captured, result.Stats.LostSamples, len(result.Stats.Probes))
	}
	return 0
}
