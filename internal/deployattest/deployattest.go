package deployattest

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/riccardomenegazzo/workload-abi/internal/graph"
	"github.com/riccardomenegazzo/workload-abi/internal/matrix"
	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

const (
	StatementType = "https://in-toto.io/Statement/v1"
	PredicateType = "https://wabi.dev/attestation/deployment/v1alpha1"
	SchemaVersion = "wabi.deployment/v1alpha1"
)

type Subject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

type ImageBinding struct {
	Name                   string `json:"name"`
	ImageID                string `json:"image_id,omitempty"`
	OperationalFingerprint string `json:"operational_fingerprint"`
}

type GraphBinding struct {
	SchemaVersion       string `json:"schema_version"`
	Fingerprint         string `json:"fingerprint"`
	SnapshotFingerprint string `json:"snapshot_fingerprint"`
}

type Predicate struct {
	SchemaVersion  string          `json:"schema_version"`
	Candidate      ImageBinding    `json:"candidate"`
	Matrix         matrix.Artifact `json:"matrix"`
	BaselineGraph  *GraphBinding   `json:"baseline_graph,omitempty"`
	CandidateGraph *GraphBinding   `json:"candidate_graph,omitempty"`
}

type Statement struct {
	Type          string    `json:"_type"`
	Subject       []Subject `json:"subject"`
	PredicateType string    `json:"predicateType"`
	Predicate     Predicate `json:"predicate"`
}

type Inputs struct {
	Matrix         matrix.Artifact
	Candidate      *model.Snapshot
	BaselineGraph  *graph.Artifact
	CandidateGraph *graph.Artifact
	OCISubject     string
}

func NewPredicate(inputs Inputs) (Predicate, error) {
	if err := matrix.Verify(inputs.Matrix); err != nil {
		return Predicate{}, fmt.Errorf("matrix: %w", err)
	}
	predicate := Predicate{
		SchemaVersion: SchemaVersion,
		Candidate: ImageBinding{
			Name:                   inputs.Matrix.Candidate,
			OperationalFingerprint: inputs.Matrix.CandidateFingerprint,
		},
		Matrix: inputs.Matrix,
	}

	if inputs.Candidate != nil {
		if inputs.Candidate.Fingerprint != inputs.Matrix.CandidateFingerprint {
			return Predicate{}, fmt.Errorf("candidate snapshot fingerprint %s does not match matrix %s", inputs.Candidate.Fingerprint, inputs.Matrix.CandidateFingerprint)
		}
		if inputs.Candidate.Image != inputs.Matrix.Candidate {
			return Predicate{}, fmt.Errorf("candidate snapshot image %q does not match matrix candidate %q", inputs.Candidate.Image, inputs.Matrix.Candidate)
		}
		if inputs.Candidate.Scenario != inputs.Matrix.Scenario {
			return Predicate{}, fmt.Errorf("candidate snapshot scenario %q does not match matrix scenario %q", inputs.Candidate.Scenario, inputs.Matrix.Scenario)
		}
		predicate.Candidate.ImageID = inputs.Candidate.ImageID
	}

	if (inputs.BaselineGraph == nil) != (inputs.CandidateGraph == nil) {
		return Predicate{}, fmt.Errorf("baseline and candidate graph bindings must be supplied together")
	}
	if inputs.BaselineGraph != nil {
		if inputs.BaselineGraph.SnapshotFingerprint != inputs.Matrix.BaselineFingerprint {
			return Predicate{}, fmt.Errorf("baseline graph snapshot %s does not match matrix baseline %s", inputs.BaselineGraph.SnapshotFingerprint, inputs.Matrix.BaselineFingerprint)
		}
		if inputs.CandidateGraph.SnapshotFingerprint != inputs.Matrix.CandidateFingerprint {
			return Predicate{}, fmt.Errorf("candidate graph snapshot %s does not match matrix candidate %s", inputs.CandidateGraph.SnapshotFingerprint, inputs.Matrix.CandidateFingerprint)
		}
		if inputs.BaselineGraph.Scenario != inputs.Matrix.Scenario || inputs.CandidateGraph.Scenario != inputs.Matrix.Scenario {
			return Predicate{}, fmt.Errorf("graph scenario does not match matrix scenario")
		}
		predicate.BaselineGraph = graphBinding(*inputs.BaselineGraph)
		predicate.CandidateGraph = graphBinding(*inputs.CandidateGraph)
	}

	if err := VerifyPredicate(predicate); err != nil {
		return Predicate{}, err
	}
	return predicate, nil
}

func New(inputs Inputs) (Statement, error) {
	predicate, err := NewPredicate(inputs)
	if err != nil {
		return Statement{}, err
	}
	subjects, err := subjects(predicate, inputs.OCISubject)
	if err != nil {
		return Statement{}, err
	}
	return Statement{
		Type:          StatementType,
		Subject:       subjects,
		PredicateType: PredicateType,
		Predicate:     predicate,
	}, nil
}

func VerifyPredicate(predicate Predicate) error {
	if predicate.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported deployment predicate schema %q", predicate.SchemaVersion)
	}
	if err := matrix.Verify(predicate.Matrix); err != nil {
		return fmt.Errorf("matrix: %w", err)
	}
	if predicate.Candidate.Name != predicate.Matrix.Candidate {
		return fmt.Errorf("candidate name does not match matrix candidate")
	}
	if predicate.Candidate.OperationalFingerprint != predicate.Matrix.CandidateFingerprint {
		return fmt.Errorf("candidate operational fingerprint does not match matrix")
	}
	if _, err := sha256Digest(predicate.Candidate.OperationalFingerprint); err != nil {
		return fmt.Errorf("candidate operational fingerprint: %w", err)
	}
	if predicate.Candidate.ImageID != "" {
		if _, err := sha256Digest(predicate.Candidate.ImageID); err != nil {
			return fmt.Errorf("candidate image_id: %w", err)
		}
	}
	if (predicate.BaselineGraph == nil) != (predicate.CandidateGraph == nil) {
		return fmt.Errorf("deployment predicate must bind baseline and candidate graphs together")
	}
	if predicate.BaselineGraph != nil {
		if err := verifyGraphBinding(*predicate.BaselineGraph, predicate.Matrix.BaselineFingerprint); err != nil {
			return fmt.Errorf("baseline graph: %w", err)
		}
		if err := verifyGraphBinding(*predicate.CandidateGraph, predicate.Matrix.CandidateFingerprint); err != nil {
			return fmt.Errorf("candidate graph: %w", err)
		}
	}
	return nil
}

func Verify(statement Statement, expectedMatrixFingerprint, expectedCandidateGraphFingerprint string) error {
	if statement.Type != StatementType {
		return fmt.Errorf("unexpected statement type %q", statement.Type)
	}
	if statement.PredicateType != PredicateType {
		return fmt.Errorf("unexpected predicate type %q", statement.PredicateType)
	}
	if err := VerifyPredicate(statement.Predicate); err != nil {
		return err
	}
	if expectedMatrixFingerprint != "" && statement.Predicate.Matrix.Fingerprint != expectedMatrixFingerprint {
		return fmt.Errorf("matrix fingerprint %s does not match expected %s", statement.Predicate.Matrix.Fingerprint, expectedMatrixFingerprint)
	}
	if expectedCandidateGraphFingerprint != "" {
		if statement.Predicate.CandidateGraph == nil {
			return fmt.Errorf("attestation has no candidate graph binding")
		}
		if statement.Predicate.CandidateGraph.Fingerprint != expectedCandidateGraphFingerprint {
			return fmt.Errorf("candidate graph fingerprint %s does not match expected %s", statement.Predicate.CandidateGraph.Fingerprint, expectedCandidateGraphFingerprint)
		}
	}
	if err := verifySubjects(statement.Subject, statement.Predicate); err != nil {
		return err
	}
	return nil
}

func LoadStatement(path string) (Statement, error) {
	var statement Statement
	data, err := os.ReadFile(path)
	if err != nil {
		return statement, fmt.Errorf("read deployment attestation: %w", err)
	}
	if err := json.Unmarshal(data, &statement); err != nil {
		return statement, fmt.Errorf("parse deployment attestation: %w", err)
	}
	return statement, nil
}

func LoadPredicate(path string) (Predicate, error) {
	var predicate Predicate
	data, err := os.ReadFile(path)
	if err != nil {
		return predicate, fmt.Errorf("read deployment predicate: %w", err)
	}
	if err := json.Unmarshal(data, &predicate); err != nil {
		return predicate, fmt.Errorf("parse deployment predicate: %w", err)
	}
	if err := VerifyPredicate(predicate); err != nil {
		return predicate, err
	}
	return predicate, nil
}

func graphBinding(artifact graph.Artifact) *GraphBinding {
	return &GraphBinding{
		SchemaVersion:       artifact.SchemaVersion,
		Fingerprint:         artifact.Fingerprint,
		SnapshotFingerprint: artifact.SnapshotFingerprint,
	}
}

func verifyGraphBinding(binding GraphBinding, expectedSnapshot string) error {
	if binding.SchemaVersion != graph.SchemaVersion {
		return fmt.Errorf("unsupported graph schema %q", binding.SchemaVersion)
	}
	if _, err := sha256Digest(binding.Fingerprint); err != nil {
		return fmt.Errorf("fingerprint: %w", err)
	}
	if binding.SnapshotFingerprint != expectedSnapshot {
		return fmt.Errorf("snapshot fingerprint %s does not match expected %s", binding.SnapshotFingerprint, expectedSnapshot)
	}
	return nil
}

func subjects(predicate Predicate, ociSubject string) ([]Subject, error) {
	var result []Subject
	if strings.TrimSpace(ociSubject) != "" {
		subject, err := parseOCISubject(ociSubject)
		if err != nil {
			return nil, err
		}
		result = append(result, subject)
	}
	operationalDigest, _ := sha256Digest(predicate.Candidate.OperationalFingerprint)
	result = append(result, Subject{
		Name:   "wabi:operational-abi:" + predicate.Candidate.Name,
		Digest: map[string]string{"sha256": operationalDigest},
	})
	matrixDigest, _ := sha256Digest(predicate.Matrix.Fingerprint)
	result = append(result, Subject{
		Name:   "wabi:environment-matrix:" + predicate.Candidate.Name,
		Digest: map[string]string{"sha256": matrixDigest},
	})
	if predicate.Candidate.ImageID != "" {
		digest, _ := sha256Digest(predicate.Candidate.ImageID)
		result = append(result, Subject{
			Name:   "wabi:image-config:" + predicate.Candidate.Name,
			Digest: map[string]string{"sha256": digest},
		})
	}
	if predicate.CandidateGraph != nil {
		digest, _ := sha256Digest(predicate.CandidateGraph.Fingerprint)
		result = append(result, Subject{
			Name:   "wabi:causal-graph:" + predicate.Candidate.Name,
			Digest: map[string]string{"sha256": digest},
		})
	}
	return result, nil
}

func verifySubjects(subjects []Subject, predicate Predicate) error {
	if len(subjects) < 2 {
		return fmt.Errorf("deployment attestation is missing required subjects")
	}
	required := map[string]string{}
	op, _ := sha256Digest(predicate.Candidate.OperationalFingerprint)
	required["wabi:operational-abi:"+predicate.Candidate.Name] = op
	mx, _ := sha256Digest(predicate.Matrix.Fingerprint)
	required["wabi:environment-matrix:"+predicate.Candidate.Name] = mx
	if predicate.Candidate.ImageID != "" {
		id, _ := sha256Digest(predicate.Candidate.ImageID)
		required["wabi:image-config:"+predicate.Candidate.Name] = id
	}
	if predicate.CandidateGraph != nil {
		g, _ := sha256Digest(predicate.CandidateGraph.Fingerprint)
		required["wabi:causal-graph:"+predicate.Candidate.Name] = g
	}
	for name, expected := range required {
		found := false
		for _, subject := range subjects {
			if subject.Name == name && subject.Digest["sha256"] == expected {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("deployment attestation subject %q is missing or has the wrong digest", name)
		}
	}
	return nil
}

func parseOCISubject(value string) (Subject, error) {
	value = strings.TrimSpace(value)
	idx := strings.LastIndex(value, "@sha256:")
	if idx <= 0 {
		return Subject{}, fmt.Errorf("OCI subject must use NAME@sha256:<64 hex digest>")
	}
	name := value[:idx]
	fingerprint := value[idx+1:]
	digest, err := sha256Digest(fingerprint)
	if err != nil {
		return Subject{}, fmt.Errorf("OCI subject: %w", err)
	}
	return Subject{Name: name, Digest: map[string]string{"sha256": digest}}, nil
}

func sha256Digest(fingerprint string) (string, error) {
	const prefix = "sha256:"
	if !strings.HasPrefix(fingerprint, prefix) || len(fingerprint) != len(prefix)+64 {
		return "", fmt.Errorf("invalid SHA-256 fingerprint %q", fingerprint)
	}
	digest := strings.TrimPrefix(fingerprint, prefix)
	for _, c := range digest {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return "", fmt.Errorf("invalid SHA-256 fingerprint %q", fingerprint)
		}
	}
	return digest, nil
}
