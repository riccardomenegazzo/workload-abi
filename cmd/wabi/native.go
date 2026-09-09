package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/riccardomenegazzo/workload-abi/internal/native"
	"github.com/riccardomenegazzo/workload-abi/internal/report"
)

func runNativeRecord(args []string) int {
	fs := flag.NewFlagSet("native-record", flag.ContinueOnError)
	cgroup := fs.String("cgroup", "", "cgroup v2 directory to observe")
	pid := fs.Int("pid", 0, "resolve and observe the cgroup v2 hierarchy containing this PID")
	duration := fs.Duration("duration", 5*time.Second, "observation duration")
	ringBytes := fs.Uint64("ring-bytes", 1<<20, "kernel ring-buffer size in bytes (rounded up to a power of two)")
	output := fs.String("output", "", "write provider-neutral RuntimeEvent JSONL to a file instead of stdout")
	summary := fs.String("summary", "", "optionally write capture metadata and drop accounting as JSON")
	failOnDrop := fs.Bool("fail-on-drop", false, "exit with status 3 when the kernel reports dropped events")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: wabi native-record [flags]")
		return 2
	}
	if *cgroup == "" && *pid <= 0 {
		fmt.Fprintln(os.Stderr, "native-record: either --cgroup or --pid is required")
		return 2
	}
	if *cgroup != "" && *pid > 0 {
		fmt.Fprintln(os.Stderr, "native-record: --cgroup and --pid are mutually exclusive")
		return 2
	}
	if *duration <= 0 {
		fmt.Fprintln(os.Stderr, "native-record: --duration must be positive")
		return 2
	}
	if *ringBytes > math.MaxUint32 {
		fmt.Fprintln(os.Stderr, "native-record: --ring-bytes is too large")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), *duration+10*time.Second)
	defer cancel()
	result, err := native.Capture(ctx, native.Config{
		CgroupPath: *cgroup,
		PID:        *pid,
		Duration:   *duration,
		RingBytes:  uint32(*ringBytes),
	})
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
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	for _, event := range result.Events {
		if err := encoder.Encode(event); err != nil {
			fmt.Fprintln(os.Stderr, "native-record:", err)
			return 1
		}
	}

	if *summary != "" {
		f, err := os.Create(*summary)
		if err != nil {
			fmt.Fprintln(os.Stderr, "native-record summary:", err)
			return 1
		}
		if err := report.JSON(f, result); err != nil {
			_ = f.Close()
			fmt.Fprintln(os.Stderr, "native-record summary:", err)
			return 1
		}
		if err := f.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "native-record summary:", err)
			return 1
		}
	}

	fmt.Fprintf(os.Stderr, "native-record: captured %d semantic event(s), dropped=%d, cgroup=%s\n", len(result.Events), result.Dropped, result.Cgroup)
	for _, warning := range result.Warnings {
		fmt.Fprintln(os.Stderr, "native-record warning:", warning)
	}
	if *failOnDrop && result.Dropped > 0 {
		return 3
	}
	return 0
}
