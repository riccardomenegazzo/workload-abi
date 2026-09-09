package attest

import (
	"testing"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

func TestStatementRoundTripVerification(t *testing.T) {
	c := model.Comparison{
		SchemaVersion:        model.SchemaVersion,
		Candidate:            "app:v2",
		CandidateFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:              "COMPATIBLE",
	}
	s, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(s, c.CandidateFingerprint); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyV1Alpha2StatementStillVerifies(t *testing.T) {
	c := model.Comparison{
		SchemaVersion:        model.SchemaVersionV1Alpha2,
		Candidate:            "legacy:v2",
		CandidateFingerprint: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		Verdict:              "CHANGED",
	}
	s, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	if s.Predicate.SchemaVersion != model.SchemaVersionV1Alpha2 {
		t.Fatalf("legacy predicate schema changed: %s", s.Predicate.SchemaVersion)
	}
	if err := Verify(s, c.CandidateFingerprint); err != nil {
		t.Fatalf("legacy attestation should remain verifiable: %v", err)
	}
}

func TestVerifyRejectsTamperedSubject(t *testing.T) {
	c := model.Comparison{
		SchemaVersion:        model.SchemaVersion,
		Candidate:            "app:v2",
		CandidateFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	s, _ := New(c)
	s.Subject[0].Digest["sha256"] = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := Verify(s, ""); err == nil {
		t.Fatal("expected tampered subject failure")
	}
}
