package compat

import (
	"testing"
	"time"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
	"github.com/riccardomenegazzo/workload-abi/internal/target"
)

func TestApplyComposeDetectsReadonlyConflict(t *testing.T) {
	base := model.Snapshot{
		Image:      "app:v1",
		Filesystem: []model.FilesystemChange{{Kind: "C", Path: "/tmp/cache"}},
	}
	candidate := model.Snapshot{
		Image: "app:v2",
		Filesystem: []model.FilesystemChange{
			{Kind: "C", Path: "/tmp/cache"},
			{Kind: "A", Path: "/var/lib/app/state"},
		},
	}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}
	targetEnv := target.ComposeTarget{File: "compose.yaml", Service: "api", ReadOnlyRootfs: true, WritablePaths: []string{"/tmp"}}

	got := ApplyCompose(comparison, base, candidate, targetEnv)
	if got.Verdict != "BREAKING" {
		t.Fatalf("verdict=%s want BREAKING", got.Verdict)
	}
	if len(got.Changes) != 1 || got.Changes[0].Surface != "filesystem" {
		t.Fatalf("unexpected changes: %#v", got.Changes)
	}
}

func TestApplyComposeAllowsWritableMount(t *testing.T) {
	base := model.Snapshot{Image: "app:v1"}
	candidate := model.Snapshot{
		Image:      "app:v2",
		Filesystem: []model.FilesystemChange{{Kind: "A", Path: "/var/lib/app/state"}},
	}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}
	targetEnv := target.ComposeTarget{File: "compose.yaml", Service: "api", ReadOnlyRootfs: true, WritablePaths: []string{"/var/lib/app"}}

	got := ApplyCompose(comparison, base, candidate, targetEnv)
	if got.Verdict == "BREAKING" {
		t.Fatalf("writable mount should avoid conflict: %#v", got.Changes)
	}
}

func TestApplyComposeDetectsShutdownConflict(t *testing.T) {
	base := model.Snapshot{Image: "app:v1"}
	candidate := model.Snapshot{Image: "app:v2", Lifecycle: model.Lifecycle{StopDuration: 12 * time.Second}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "COMPATIBLE"}
	targetEnv := target.ComposeTarget{File: "compose.yaml", Service: "api", StopGracePeriod: 10 * time.Second}

	got := ApplyCompose(comparison, base, candidate, targetEnv)
	if got.Verdict != "BREAKING" {
		t.Fatalf("verdict=%s want BREAKING", got.Verdict)
	}
}
