package diff

import (
	"testing"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

func TestCompareMarksScenarioRegressionBreaking(t *testing.T) {
	base := model.Snapshot{
		Image: "app:v1",
		ScenarioSteps: []model.ScenarioStepResult{
			{Name: "health", ExitCode: 0, Success: true},
		},
	}
	candidate := model.Snapshot{
		Image: "app:v2",
		ScenarioSteps: []model.ScenarioStepResult{
			{Name: "health", ExitCode: 7, Success: false},
		},
	}
	got := Compare(base, candidate)
	if got.Verdict != "BREAKING" {
		t.Fatalf("verdict=%s want BREAKING", got.Verdict)
	}
}

func TestCompareDetectsRuntimeListener(t *testing.T) {
	base := model.Snapshot{Image: "app:v1"}
	candidate := model.Snapshot{
		Image:     "app:v2",
		Listeners: []model.Listener{{Protocol: "tcp", Port: 9090}},
	}
	got := Compare(base, candidate)
	if got.Verdict != "CHANGED" {
		t.Fatalf("verdict=%s want CHANGED", got.Verdict)
	}
}
