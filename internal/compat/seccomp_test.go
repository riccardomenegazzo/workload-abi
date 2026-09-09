package compat

import (
	"testing"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
	"github.com/riccardomenegazzo/workload-abi/internal/target"
)

func TestApplySeccompDeniesNewExactSyscall(t *testing.T) {
	base := model.Snapshot{Image: "app:v1"}
	candidate := model.Snapshot{Image: "app:v2", RuntimeEvents: []model.RuntimeEvent{{
		Category: "syscall", Operation: "bpf", Process: "/usr/bin/app",
	}}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}
	profile := target.SeccompProfile{
		File: "locked-down.json", DefaultAction: "SCMP_ACT_ALLOW",
		Syscalls: []target.SeccompSyscallRule{{Names: []string{"bpf"}, Action: "SCMP_ACT_ERRNO"}},
	}

	got := ApplySeccomp(comparison, base, candidate, profile)
	if got.Verdict != "BREAKING" {
		t.Fatalf("verdict=%s want BREAKING: %#v", got.Verdict, got.Changes)
	}
	if got.Target != "seccomp:locked-down.json" {
		t.Fatalf("target=%q", got.Target)
	}
	var found bool
	for _, change := range got.Changes {
		if change.Surface == "syscall" && change.Kind == "target-conflict" && change.After == "bpf" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing syscall target conflict: %#v", got.Changes)
	}
}

func TestApplySeccompDoesNotRejudgeExistingSyscall(t *testing.T) {
	event := model.RuntimeEvent{Category: "syscall", Operation: "bpf"}
	base := model.Snapshot{Image: "app:v1", RuntimeEvents: []model.RuntimeEvent{event}}
	candidate := model.Snapshot{Image: "app:v2", RuntimeEvents: []model.RuntimeEvent{event}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "COMPATIBLE"}
	profile := target.SeccompProfile{
		File: "locked-down.json", DefaultAction: "SCMP_ACT_ALLOW",
		Syscalls: []target.SeccompSyscallRule{{Names: []string{"bpf"}, Action: "SCMP_ACT_ERRNO"}},
	}
	got := ApplySeccomp(comparison, base, candidate, profile)
	if got.Verdict == "BREAKING" {
		t.Fatalf("existing syscall should not become a release regression: %#v", got.Changes)
	}
}

func TestApplySeccompConditionalRuleDoesNotFalseBreak(t *testing.T) {
	base := model.Snapshot{Image: "app:v1"}
	candidate := model.Snapshot{Image: "app:v2", RuntimeEvents: []model.RuntimeEvent{{Category: "syscall", Operation: "clone"}}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}
	profile := target.SeccompProfile{
		File: "conditional.json", DefaultAction: "SCMP_ACT_ERRNO",
		Syscalls: []target.SeccompSyscallRule{{Names: []string{"clone"}, Action: "SCMP_ACT_ALLOW", Conditional: true}},
	}
	got := ApplySeccomp(comparison, base, candidate, profile)
	if got.Verdict == "BREAKING" {
		t.Fatalf("conditional rule must remain unresolved rather than false-break: %#v", got.Changes)
	}
}

func TestApplySeccompIgnoresNormalizedNonSyscallEvidence(t *testing.T) {
	base := model.Snapshot{Image: "app:v1"}
	candidate := model.Snapshot{Image: "app:v2", RuntimeEvents: []model.RuntimeEvent{{Category: "file", Operation: "open"}}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}
	profile := target.SeccompProfile{File: "deny.json", DefaultAction: "SCMP_ACT_ERRNO"}
	got := ApplySeccomp(comparison, base, candidate, profile)
	if got.Verdict == "BREAKING" {
		t.Fatalf("normalized behavior is not exact syscall identity: %#v", got.Changes)
	}
}
