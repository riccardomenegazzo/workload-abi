package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/riccardomenegazzo/workload-abi/internal/dashboard"
)

func runDashboard(args []string) int {
	fs := flag.NewFlagSet("dashboard", flag.ContinueOnError)
	matrixFile := fs.String("matrix", "", "verified environment matrix artifact")
	baselineGraph := fs.String("baseline-graph", "", "optional verified baseline causal graph")
	candidateGraph := fs.String("candidate-graph", "", "optional verified candidate causal graph")
	listen := fs.String("listen", "127.0.0.1:8787", "HTTP listen address")
	allowRemote := fs.Bool("allow-remote", false, "allow dashboard to bind beyond loopback; may expose sensitive runtime evidence")
	export := fs.String("export", "", "write a self-contained dashboard HTML file and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *matrixFile == "" {
		fmt.Fprintln(os.Stderr, "usage: wabi dashboard --matrix MATRIX.json [--baseline-graph BASE.graph.json --candidate-graph CANDIDATE.graph.json] [--export dashboard.html | --listen 127.0.0.1:8787]")
		return 2
	}

	data, err := dashboard.Load(*matrixFile, *baselineGraph, *candidateGraph)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dashboard:", err)
		return 1
	}
	if *export != "" {
		if err := dashboard.Export(*export, data); err != nil {
			fmt.Fprintln(os.Stderr, "dashboard:", err)
			return 1
		}
		fmt.Printf("dashboard exported: %s\n", *export)
		return 0
	}
	if err := dashboard.ValidateListenAddress(*listen, *allowRemote); err != nil {
		fmt.Fprintln(os.Stderr, "dashboard:", err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Printf("Workload ABI dashboard: http://%s\n", *listen)
	fmt.Println("Press Ctrl-C to stop.")
	if err := dashboard.Serve(ctx, *listen, *allowRemote, data); err != nil {
		fmt.Fprintln(os.Stderr, "dashboard:", err)
		return 1
	}
	return 0
}
