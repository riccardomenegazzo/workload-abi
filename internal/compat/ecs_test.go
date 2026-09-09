package compat

import (
	"testing"
	"time"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
	"github.com/riccardomenegazzo/workload-abi/internal/target"
)

func TestApplyECSDetectsReadonlyRootfsConflict(t *testing.T) {
	base := model.Snapshot{
		Image:      "app:v1",
		Filesystem: []model.FilesystemChange{{Kind: "A", Path: "/tmp/version"}},
	}
	candidate := model.Snapshot{
		Image: "app:v2",
		Filesystem: []model.FilesystemChange{
			{Kind: "A", Path: "/tmp/version"},
			{Kind: "A", Path: "/var/lib/app/state"},
		},
	}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}
	targetEnv := target.ECSTarget{Family: "payments", Container: "api", ReadOnlyRootfs: true, WritablePaths: []string{"/tmp"}}

	got := ApplyECS(comparison, base, candidate, targetEnv)
	if got.Verdict != "BREAKING" {
		t.Fatalf("verdict=%s want BREAKING", got.Verdict)
	}
	if got.Target != "ecs:payments#api" {
		t.Fatalf("target=%q", got.Target)
	}
	if !hasECSChange(got.Changes, "filesystem", "target-conflict") {
		t.Fatalf("missing filesystem target conflict: %#v", got.Changes)
	}
}

func TestApplyECSAllowsWritableMount(t *testing.T) {
	base := model.Snapshot{Image: "app:v1"}
	candidate := model.Snapshot{Image: "app:v2", Filesystem: []model.FilesystemChange{{Kind: "A", Path: "/var/lib/app/state"}}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}
	targetEnv := target.ECSTarget{Family: "payments", Container: "api", ReadOnlyRootfs: true, WritablePaths: []string{"/var/lib/app"}}

	got := ApplyECS(comparison, base, candidate, targetEnv)
	if got.Verdict == "BREAKING" {
		t.Fatalf("writable ECS mount should avoid conflict: %#v", got.Changes)
	}
}

func TestApplyECSDetectsHardMemoryLimitConflict(t *testing.T) {
	base := model.Snapshot{Image: "app:v1"}
	candidate := model.Snapshot{Image: "app:v2", Stats: model.Stats{MemUsage: "300MiB / 512MiB"}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}
	targetEnv := target.ECSTarget{Family: "payments", Container: "api", MemoryHardLimit: 256 * 1024 * 1024}

	got := ApplyECS(comparison, base, candidate, targetEnv)
	if got.Verdict != "BREAKING" || !hasECSChange(got.Changes, "resource", "target-conflict") {
		t.Fatalf("expected resource conflict: %#v", got.Changes)
	}
}

func TestApplyECSMemoryReservationAloneDoesNotBreak(t *testing.T) {
	base := model.Snapshot{Image: "app:v1"}
	candidate := model.Snapshot{Image: "app:v2", Stats: model.Stats{MemUsage: "300MiB / 512MiB"}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}
	targetEnv := target.ECSTarget{Family: "payments", Container: "api", MemoryReservation: 128 * 1024 * 1024}

	got := ApplyECS(comparison, base, candidate, targetEnv)
	if got.Verdict == "BREAKING" {
		t.Fatalf("soft memoryReservation must not create hard conflict: %#v", got.Changes)
	}
}

func TestApplyECSDetectsExplicitStopTimeoutConflict(t *testing.T) {
	base := model.Snapshot{Image: "app:v1"}
	candidate := model.Snapshot{Image: "app:v2", Lifecycle: model.Lifecycle{StopDuration: 12 * time.Second}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}
	targetEnv := target.ECSTarget{Family: "payments", Container: "api", StopTimeout: 10 * time.Second}

	got := ApplyECS(comparison, base, candidate, targetEnv)
	if got.Verdict != "BREAKING" || !hasECSChange(got.Changes, "lifecycle", "target-conflict") {
		t.Fatalf("expected lifecycle conflict: %#v", got.Changes)
	}
}

func TestApplyECSAbsentStopTimeoutDoesNotAssumeDefault(t *testing.T) {
	base := model.Snapshot{Image: "app:v1"}
	candidate := model.Snapshot{Image: "app:v2", Lifecycle: model.Lifecycle{StopDuration: 2 * time.Minute}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}
	targetEnv := target.ECSTarget{Family: "payments", Container: "api"}

	got := ApplyECS(comparison, base, candidate, targetEnv)
	if got.Verdict == "BREAKING" {
		t.Fatalf("absent stopTimeout must remain unknown: %#v", got.Changes)
	}
}

func hasECSChange(changes []model.Change, surface, kind string) bool {
	for _, change := range changes {
		if change.Surface == surface && change.Kind == kind {
			return true
		}
	}
	return false
}
