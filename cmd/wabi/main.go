package main

import (
	"fmt"
	"os"
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
	case "enrich":
		os.Exit(runEnrich(os.Args[2:]))
	case "graph":
		os.Exit(runGraph(os.Args[2:]))
	case "graph-diff":
		os.Exit(runGraphDiff(os.Args[2:]))
	case "compare-snapshots":
		os.Exit(runCompareSnapshots(os.Args[2:]))
	case "matrix":
		os.Exit(runMatrix(os.Args[2:]))
	case "verify-matrix":
		os.Exit(runVerifyMatrix(os.Args[2:]))
	case "dashboard":
		os.Exit(runDashboard(os.Args[2:]))
	case "attest":
		os.Exit(runAttest(os.Args[2:]))
	case "verify-attestation":
		os.Exit(runVerifyAttestation(os.Args[2:]))
	case "deployment-attest":
		os.Exit(runDeploymentAttest(os.Args[2:]))
	case "verify-deployment-attestation":
		os.Exit(runVerifyDeploymentAttestation(os.Args[2:]))
	case "verify-deployment-predicate":
		os.Exit(runVerifyDeploymentPredicate(os.Args[2:]))
	case "doctor":
		os.Exit(runDoctor(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Println("wabi", version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Workload ABI (wabi)

Discover operational breaking changes between container releases.

Commands:
  wabi compare [flags] BASELINE_IMAGE CANDIDATE_IMAGE
  wabi record            [flags] IMAGE
  wabi enrich            --snapshot SNAPSHOT.json --events EVENTS.jsonl --format falco|tracee|generic
  wabi graph             [--output FILE] SNAPSHOT.json
  wabi graph-diff        [--format human|json] BASELINE.graph.json CANDIDATE.graph.json
  wabi compare-snapshots [flags] BASELINE.json CANDIDATE.json
  wabi matrix            --config ENVIRONMENTS.json [flags] BASELINE.json CANDIDATE.json
  wabi verify-matrix     MATRIX.json
  wabi dashboard         --matrix MATRIX.json [flags]
  wabi attest            [flags] COMPARISON.json
  wabi verify-attestation [flags] ATTESTATION.json
  wabi deployment-attest --matrix MATRIX.json [flags]
  wabi verify-deployment-attestation [flags] DEPLOYMENT.intoto.json
  wabi verify-deployment-predicate [flags] PREDICATE.json
  wabi doctor            [--json]
  wabi version

Equivalent multi-phase experiment:
  wabi compare --scenario scenario.json BASELINE CANDIDATE

Deep runtime evidence:
  wabi enrich --snapshot snapshot.json --events falco.jsonl --format falco --output enriched.json

Causal runtime graph:
  wabi graph --output candidate.graph.json candidate.enriched.json
  wabi graph-diff baseline.graph.json candidate.graph.json

Target-aware comparison:
  wabi compare --target compose.yaml --service api BASELINE CANDIDATE
  wabi compare --target deployment.json --target-kind kubernetes --container api BASELINE CANDIDATE
  wabi compare-snapshots --target deployment.json --target-kind kubernetes --network-policy egress.json BASELINE.json CANDIDATE.json
  wabi compare-snapshots --target task-definition.json --target-kind ecs --container api BASELINE.json CANDIDATE.json

Environment compatibility matrix:
  wabi matrix --config environments.json --output matrix.json BASELINE.json CANDIDATE.json
  wabi verify-matrix matrix.json

Deployability dashboard:
  wabi dashboard --matrix matrix.json
  wabi dashboard --matrix matrix.json --baseline-graph baseline.graph.json --candidate-graph candidate.graph.json
  wabi dashboard --matrix matrix.json --export dashboard.html

Supply-chain deployment evidence:
  wabi deployment-attest --matrix matrix.json --candidate-snapshot candidate.json --output deployment.intoto.json
  wabi deployment-attest --matrix matrix.json --predicate-only --output deployment.predicate.json
  wabi verify-deployment-attestation deployment.intoto.json
  wabi verify-deployment-predicate deployment.predicate.json

Seccomp proof from exact syscall evidence:
  wabi compare-snapshots --seccomp-profile seccomp.json BASELINE.json CANDIDATE.json

Policy gate:
  wabi compare --scenario scenario.json --target compose.yaml --policy policy.json BASELINE CANDIDATE

Machine-readable output:
  wabi compare --format json BASELINE CANDIDATE
  wabi compare --format sarif BASELINE CANDIDATE
  wabi matrix --config environments.json --format json BASELINE.json CANDIDATE.json

Exit codes for compare/matrix:
  0  compatible/no fatal error
  3  changes found when --fail-on-change is enabled
  4  breaking runtime, target, scenario, policy, or environment regression detected`)
}
