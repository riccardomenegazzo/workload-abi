package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/riccardomenegazzo/workload-abi/internal/deployattest"
	"github.com/riccardomenegazzo/workload-abi/internal/graph"
	"github.com/riccardomenegazzo/workload-abi/internal/matrix"
	"github.com/riccardomenegazzo/workload-abi/internal/report"
	"github.com/riccardomenegazzo/workload-abi/internal/snapshot"
)

func runDeploymentAttest(args []string) int {
	fs := flag.NewFlagSet("deployment-attest", flag.ContinueOnError)
	matrixFile := fs.String("matrix", "", "verified Environment Compatibility Matrix artifact")
	candidateSnapshotFile := fs.String("candidate-snapshot", "", "optional verified candidate snapshot used to bind image ID/config identity")
	baselineGraphFile := fs.String("baseline-graph", "", "optional verified baseline causal graph")
	candidateGraphFile := fs.String("candidate-graph", "", "optional verified candidate causal graph")
	ociSubject := fs.String("oci-subject", "", "optional OCI subject NAME@sha256:DIGEST to include in the standalone in-toto statement")
	predicateOnly := fs.Bool("predicate-only", false, "emit only the WABI deployment predicate for cosign or another external attester")
	output := fs.String("output", "", "write output to a file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *matrixFile == "" {
		fmt.Fprintln(os.Stderr, "usage: wabi deployment-attest --matrix MATRIX.json [--candidate-snapshot CANDIDATE.json] [--baseline-graph BASE.graph.json --candidate-graph CANDIDATE.graph.json] [--oci-subject IMAGE@sha256:DIGEST] [--predicate-only] [--output FILE]")
		return 2
	}
	if (*baselineGraphFile == "") != (*candidateGraphFile == "") {
		fmt.Fprintln(os.Stderr, "deployment-attest: --baseline-graph and --candidate-graph must be supplied together")
		return 2
	}

	matrixArtifact, err := matrix.LoadArtifact(*matrixFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "deployment-attest:", err)
		return 1
	}
	inputs := deployattest.Inputs{Matrix: matrixArtifact, OCISubject: *ociSubject}
	if *candidateSnapshotFile != "" {
		value, err := snapshot.Load(*candidateSnapshotFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "deployment-attest: candidate snapshot:", err)
			return 1
		}
		inputs.Candidate = &value
	}
	if *baselineGraphFile != "" {
		baseGraph, err := graph.Load(*baselineGraphFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "deployment-attest: baseline graph:", err)
			return 1
		}
		candidateGraph, err := graph.Load(*candidateGraphFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "deployment-attest: candidate graph:", err)
			return 1
		}
		inputs.BaselineGraph = &baseGraph
		inputs.CandidateGraph = &candidateGraph
	}

	var value any
	if *predicateOnly {
		predicate, err := deployattest.NewPredicate(inputs)
		if err != nil {
			fmt.Fprintln(os.Stderr, "deployment-attest:", err)
			return 1
		}
		value = predicate
	} else {
		statement, err := deployattest.New(inputs)
		if err != nil {
			fmt.Fprintln(os.Stderr, "deployment-attest:", err)
			return 1
		}
		value = statement
	}

	out := os.Stdout
	if *output != "" {
		file, err := os.Create(*output)
		if err != nil {
			fmt.Fprintln(os.Stderr, "deployment-attest:", err)
			return 1
		}
		defer file.Close()
		out = file
	}
	if err := report.JSON(out, value); err != nil {
		fmt.Fprintln(os.Stderr, "deployment-attest:", err)
		return 1
	}
	return 0
}

func runVerifyDeploymentAttestation(args []string) int {
	fs := flag.NewFlagSet("verify-deployment-attestation", flag.ContinueOnError)
	expectedMatrix := fs.String("matrix-fingerprint", "", "expected Environment Compatibility Matrix fingerprint")
	expectedGraph := fs.String("candidate-graph-fingerprint", "", "expected candidate causal graph fingerprint")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: wabi verify-deployment-attestation [flags] DEPLOYMENT.intoto.json")
		return 2
	}
	statement, err := deployattest.LoadStatement(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "verify-deployment-attestation:", err)
		return 1
	}
	if err := deployattest.Verify(statement, *expectedMatrix, *expectedGraph); err != nil {
		fmt.Fprintln(os.Stderr, "verify-deployment-attestation:", err)
		return 1
	}
	fmt.Printf("verified deployment evidence: candidate=%s matrix=%s verdict=%s\n", statement.Predicate.Candidate.Name, statement.Predicate.Matrix.Fingerprint, statement.Predicate.Matrix.Verdict)
	return 0
}

func runVerifyDeploymentPredicate(args []string) int {
	fs := flag.NewFlagSet("verify-deployment-predicate", flag.ContinueOnError)
	expectedMatrix := fs.String("matrix-fingerprint", "", "expected Environment Compatibility Matrix fingerprint")
	expectedGraph := fs.String("candidate-graph-fingerprint", "", "expected candidate causal graph fingerprint")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: wabi verify-deployment-predicate [flags] PREDICATE.json")
		return 2
	}
	predicate, err := deployattest.LoadPredicate(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "verify-deployment-predicate:", err)
		return 1
	}
	if *expectedMatrix != "" && predicate.Matrix.Fingerprint != *expectedMatrix {
		fmt.Fprintf(os.Stderr, "verify-deployment-predicate: matrix fingerprint %s does not match expected %s\n", predicate.Matrix.Fingerprint, *expectedMatrix)
		return 1
	}
	if *expectedGraph != "" {
		if predicate.CandidateGraph == nil {
			fmt.Fprintln(os.Stderr, "verify-deployment-predicate: predicate has no candidate graph binding")
			return 1
		}
		if predicate.CandidateGraph.Fingerprint != *expectedGraph {
			fmt.Fprintf(os.Stderr, "verify-deployment-predicate: candidate graph fingerprint %s does not match expected %s\n", predicate.CandidateGraph.Fingerprint, *expectedGraph)
			return 1
		}
	}
	fmt.Printf("verified deployment predicate: candidate=%s matrix=%s verdict=%s\n", predicate.Candidate.Name, predicate.Matrix.Fingerprint, predicate.Matrix.Verdict)
	return 0
}
