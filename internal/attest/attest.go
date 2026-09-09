package attest

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

const (
	StatementType = "https://in-toto.io/Statement/v1"
	PredicateType = "https://wabi.dev/attestation/compatibility/v1alpha1"
)

type Subject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

type Predicate struct {
	SchemaVersion string           `json:"schema_version"`
	Comparison    model.Comparison `json:"comparison"`
}

type Statement struct {
	Type          string    `json:"_type"`
	Subject       []Subject `json:"subject"`
	PredicateType string    `json:"predicateType"`
	Predicate     Predicate `json:"predicate"`
}

func New(c model.Comparison) (Statement, error) {
	if c.Candidate == "" {
		return Statement{}, fmt.Errorf("comparison candidate is required")
	}
	if c.CandidateFingerprint == "" {
		return Statement{}, fmt.Errorf("candidate fingerprint is required for attestation")
	}
	digest, err := sha256Digest(c.CandidateFingerprint)
	if err != nil {
		return Statement{}, err
	}
	if c.SchemaVersion == "" {
		c.SchemaVersion = model.SchemaVersion
	}
	return Statement{
		Type: StatementType,
		Subject: []Subject{{
			Name:   c.Candidate,
			Digest: map[string]string{"sha256": digest},
		}},
		PredicateType: PredicateType,
		Predicate: Predicate{
			SchemaVersion: model.SchemaVersion,
			Comparison:    c,
		},
	}, nil
}

func Verify(s Statement, expectedFingerprint string) error {
	if s.Type != StatementType {
		return fmt.Errorf("unexpected statement type %q", s.Type)
	}
	if s.PredicateType != PredicateType {
		return fmt.Errorf("unexpected predicate type %q", s.PredicateType)
	}
	if s.Predicate.SchemaVersion != model.SchemaVersion {
		return fmt.Errorf("unsupported predicate schema %q", s.Predicate.SchemaVersion)
	}
	if len(s.Subject) != 1 {
		return fmt.Errorf("expected exactly one attestation subject")
	}
	comparison := s.Predicate.Comparison
	if comparison.Candidate == "" || comparison.CandidateFingerprint == "" {
		return fmt.Errorf("attestation comparison is missing candidate evidence")
	}
	if s.Subject[0].Name != comparison.Candidate {
		return fmt.Errorf("attestation subject name does not match comparison candidate")
	}
	digest, err := sha256Digest(comparison.CandidateFingerprint)
	if err != nil {
		return err
	}
	if s.Subject[0].Digest["sha256"] != digest {
		return fmt.Errorf("attestation subject digest does not match candidate fingerprint")
	}
	if expectedFingerprint != "" && comparison.CandidateFingerprint != expectedFingerprint {
		return fmt.Errorf("candidate fingerprint %s does not match expected %s", comparison.CandidateFingerprint, expectedFingerprint)
	}
	return nil
}

func LoadComparison(path string) (model.Comparison, error) {
	var c model.Comparison
	data, err := os.ReadFile(path)
	if err != nil {
		return c, fmt.Errorf("read comparison: %w", err)
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("parse comparison: %w", err)
	}
	if c.SchemaVersion != "" && c.SchemaVersion != model.SchemaVersion {
		return c, fmt.Errorf("unsupported comparison schema %q", c.SchemaVersion)
	}
	if c.Candidate == "" {
		return c, fmt.Errorf("comparison candidate is required")
	}
	return c, nil
}

func LoadStatement(path string) (Statement, error) {
	var s Statement
	data, err := os.ReadFile(path)
	if err != nil {
		return s, fmt.Errorf("read attestation: %w", err)
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("parse attestation: %w", err)
	}
	return s, nil
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
