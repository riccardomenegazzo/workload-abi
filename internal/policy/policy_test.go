package policy

import (
	"testing"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

func TestPolicyCanPromoteWarningToBreaking(t *testing.T) {
	c := model.Comparison{
		Verdict: "CHANGED",
		Changes: []model.Change{{Surface: "listener", Kind: "added", Severity: "warning", Message: "new listener"}},
	}
	got := Apply(c, Spec{Name: "strict", DenySeverities: []string{"warning"}})
	if got.Verdict != "BREAKING" || got.Policy != "strict" {
		t.Fatalf("unexpected policy result: %#v", got)
	}
}

func TestPolicyRequiresEvidenceContext(t *testing.T) {
	got := Apply(model.Comparison{Verdict: "COMPATIBLE"}, Spec{RequireScenario: true, RequireTarget: true})
	if got.Verdict != "BREAKING" || len(got.Changes) != 2 {
		t.Fatalf("unexpected policy violations: %#v", got.Changes)
	}
}
