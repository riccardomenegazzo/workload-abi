package deployattest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riccardomenegazzo/workload-abi/internal/graph"
	"github.com/riccardomenegazzo/workload-abi/internal/matrix"
	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

func TestDeploymentAttestationBindsMatrixSnapshotAndGraphs(t *testing.T) {
	base, candidate, matrixArtifact, baseGraph, candidateGraph := fixture()
	statement, err := New(Inputs{
		Matrix:         matrixArtifact,
		Candidate:      &candidate,
		BaselineGraph:  &baseGraph,
		CandidateGraph: &candidateGraph,
		OCISubject:     "ghcr.io/acme/app@sha256:" + strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	if statement.Type != StatementType || statement.PredicateType != PredicateType {
		t.Fatalf("unexpected statement metadata: %#v", statement)
	}
	if statement.Predicate.Matrix.Fingerprint != matrixArtifact.Fingerprint {
		t.Fatal("matrix fingerprint was not bound")
	}
	if statement.Predicate.Candidate.OperationalFingerprint != candidate.Fingerprint {
		t.Fatal("candidate operational fingerprint was not bound")
	}
	if statement.Predicate.CandidateGraph == nil || statement.Predicate.CandidateGraph.Fingerprint != candidateGraph.Fingerprint {
		t.Fatal("candidate graph fingerprint was not bound")
	}
	if statement.Predicate.BaselineGraph == nil || statement.Predicate.BaselineGraph.Fingerprint != baseGraph.Fingerprint {
		t.Fatal("baseline graph fingerprint was not bound")
	}
	if len(statement.Subject) != 5 {
		t.Fatalf("expected OCI, operational, matrix, image config and graph subjects, got %d", len(statement.Subject))
	}
	if err := Verify(statement, matrixArtifact.Fingerprint, candidateGraph.Fingerprint); err != nil {
		t.Fatal(err)
	}
	_ = base
}

func TestPredicateCanBeUsedWithoutOCIOrSnapshot(t *testing.T) {
	_, _, matrixArtifact, baseGraph, candidateGraph := fixture()
	predicate, err := NewPredicate(Inputs{
		Matrix:         matrixArtifact,
		BaselineGraph:  &baseGraph,
		CandidateGraph: &candidateGraph,
	})
	if err != nil {
		t.Fatal(err)
	}
	if predicate.Candidate.ImageID != "" {
		t.Fatalf("did not expect image ID without snapshot: %q", predicate.Candidate.ImageID)
	}
	if err := VerifyPredicate(predicate); err != nil {
		t.Fatal(err)
	}
}

func TestRejectsMismatchedCandidateSnapshot(t *testing.T) {
	_, candidate, matrixArtifact, _, _ := fixture()
	candidate.RuntimeEvents = append(candidate.RuntimeEvents, model.RuntimeEvent{Category: "syscall", Operation: "mount", Process: "/app"})
	candidate.Fingerprint = model.Fingerprint(candidate)
	if _, err := NewPredicate(Inputs{Matrix: matrixArtifact, Candidate: &candidate}); err == nil || !strings.Contains(err.Error(), "does not match matrix") {
		t.Fatalf("expected candidate binding failure, got %v", err)
	}
}

func TestRejectsMismatchedGraph(t *testing.T) {
	_, candidate, matrixArtifact, baseGraph, candidateGraph := fixture()
	wrong := candidate
	wrong.RuntimeEvents = append(wrong.RuntimeEvents, model.RuntimeEvent{Category: "syscall", Operation: "mount", Process: "/app"})
	wrong.Fingerprint = model.Fingerprint(wrong)
	candidateGraph = graph.Build(wrong)
	if _, err := NewPredicate(Inputs{Matrix: matrixArtifact, BaselineGraph: &baseGraph, CandidateGraph: &candidateGraph}); err == nil || !strings.Contains(err.Error(), "candidate graph snapshot") {
		t.Fatalf("expected graph binding failure, got %v", err)
	}
}

func TestVerifyDetectsStatementSubjectTamper(t *testing.T) {
	_, candidate, matrixArtifact, _, _ := fixture()
	statement, err := New(Inputs{Matrix: matrixArtifact, Candidate: &candidate})
	if err != nil {
		t.Fatal(err)
	}
	statement.Subject[0].Digest["sha256"] = strings.Repeat("0", 64)
	if err := Verify(statement, "", ""); err == nil || !strings.Contains(err.Error(), "wrong digest") {
		t.Fatalf("expected subject tamper failure, got %v", err)
	}
}

func TestLoadPredicateRejectsMatrixTamper(t *testing.T) {
	_, _, matrixArtifact, _, _ := fixture()
	predicate, err := NewPredicate(Inputs{Matrix: matrixArtifact})
	if err != nil {
		t.Fatal(err)
	}
	predicate.Matrix.Environments[0].Verdict = "COMPATIBLE"
	dir := t.TempDir()
	path := filepath.Join(dir, "predicate.json")
	data, _ := json.Marshal(predicate)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPredicate(path); err == nil || !strings.Contains(err.Error(), "matrix fingerprint mismatch") {
		t.Fatalf("expected matrix tamper failure, got %v", err)
	}
}

func TestRejectsInvalidOCISubject(t *testing.T) {
	_, _, matrixArtifact, _, _ := fixture()
	if _, err := New(Inputs{Matrix: matrixArtifact, OCISubject: "ghcr.io/acme/app:latest"}); err == nil || !strings.Contains(err.Error(), "NAME@sha256") {
		t.Fatalf("expected OCI subject validation failure, got %v", err)
	}
}

func fixture() (model.Snapshot, model.Snapshot, matrix.Artifact, graph.Artifact, graph.Artifact) {
	base := model.Snapshot{
		SchemaVersion: model.SchemaVersion,
		Image:         "app:v1",
		Scenario:      "smoke",
		ImageID:       "sha256:" + strings.Repeat("1", 64),
		RuntimeEvents: []model.RuntimeEvent{{Category: "process", Operation: "exec", Process: "/app"}},
	}
	base.Fingerprint = model.Fingerprint(base)
	candidate := model.Snapshot{
		SchemaVersion: model.SchemaVersion,
		Image:         "app:v2",
		Scenario:      "smoke",
		ImageID:       "sha256:" + strings.Repeat("2", 64),
		RuntimeEvents: []model.RuntimeEvent{
			{Category: "process", Operation: "exec", Process: "/app"},
			{Category: "network", Operation: "connect", Process: "/app", Target: "api.example.com:443", Protocol: "tcp", Direction: "outbound"},
		},
	}
	candidate.Fingerprint = model.Fingerprint(candidate)
	matrixArtifact := matrix.Artifact{
		SchemaVersion:        matrix.ArtifactSchemaVersion,
		Baseline:             base.Image,
		Candidate:            candidate.Image,
		BaselineFingerprint:  base.Fingerprint,
		CandidateFingerprint: candidate.Fingerprint,
		Scenario:             "smoke",
		Verdict:              "BREAKING",
		Summary:              matrix.Summary{Breaking: 1},
		Environments: []matrix.EnvironmentResult{{
			Name:    "production",
			Target:  "kubernetes:deployment#api",
			Verdict: "BREAKING",
			Changes: []model.Change{{Surface: "runtime-network", Kind: "target-conflict", Severity: "breaking", Message: "egress blocked"}},
		}},
	}
	matrixArtifact.Fingerprint = matrix.Fingerprint(matrixArtifact)
	return base, candidate, matrixArtifact, graph.Build(base), graph.Build(candidate)
}
