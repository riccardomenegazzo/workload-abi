package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/riccardomenegazzo/workload-abi/internal/attest"
	"github.com/riccardomenegazzo/workload-abi/internal/report"
)

func runAttest(args []string) int {
	fs := flag.NewFlagSet("attest", flag.ContinueOnError)
	output := fs.String("output", "", "write attestation to a file instead of stdout")
	predicateOnly := fs.Bool("predicate-only", false, "emit only the Workload ABI predicate for external signing tools")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: wabi attest [--output FILE] COMPARISON.json")
		return 2
	}
	comparison, err := attest.LoadComparison(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "attest:", err)
		return 1
	}
	statement, err := attest.New(comparison)
	if err != nil {
		fmt.Fprintln(os.Stderr, "attest:", err)
		return 1
	}
	out := os.Stdout
	if *output != "" {
		f, err := os.Create(*output)
		if err != nil {
			fmt.Fprintln(os.Stderr, "attest:", err)
			return 1
		}
		defer f.Close()
		out = f
	}
	var value any = statement
	if *predicateOnly {
		value = statement.Predicate
	}
	if err := report.JSON(out, value); err != nil {
		fmt.Fprintln(os.Stderr, "attest:", err)
		return 1
	}
	return 0
}

func runVerifyAttestation(args []string) int {
	fs := flag.NewFlagSet("verify-attestation", flag.ContinueOnError)
	expected := fs.String("candidate-fingerprint", "", "expected candidate SHA-256 operational fingerprint")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: wabi verify-attestation [flags] ATTESTATION.json")
		return 2
	}
	statement, err := attest.LoadStatement(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "verify-attestation:", err)
		return 1
	}
	if err := attest.Verify(statement, *expected); err != nil {
		fmt.Fprintln(os.Stderr, "verify-attestation:", err)
		return 1
	}
	fmt.Printf("verified: %s %s\n", statement.Predicate.Comparison.Candidate, statement.Predicate.Comparison.CandidateFingerprint)
	return 0
}
