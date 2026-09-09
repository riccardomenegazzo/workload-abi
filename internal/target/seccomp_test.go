package target

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSeccompProfileAndDecisions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.json")
	data := `{
	  "defaultAction":"SCMP_ACT_ERRNO",
	  "syscalls":[
	    {"names":["read","write"],"action":"SCMP_ACT_ALLOW"},
	    {"names":["bpf"],"action":"SCMP_ACT_KILL_PROCESS"},
	    {"names":["clone"],"action":"SCMP_ACT_ALLOW","args":[{"index":0,"value":1,"op":"SCMP_CMP_EQ"}]},
	    {"names":["socket"],"action":"SCMP_ACT_NOTIFY"}
	  ]
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	profile, err := LoadSeccompProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if profile.File != path || profile.DefaultAction != "SCMP_ACT_ERRNO" {
		t.Fatalf("unexpected profile: %#v", profile)
	}

	assertSeccompDecision(t, profile, "read", SeccompAllow)
	assertSeccompDecision(t, profile, "bpf", SeccompDeny)
	assertSeccompDecision(t, profile, "mount", SeccompDeny)
	assertSeccompDecision(t, profile, "clone", SeccompUnknown)
	assertSeccompDecision(t, profile, "socket", SeccompUnknown)
}

func TestSeccompLogAllowsAndTraceRemainsUnknown(t *testing.T) {
	profile := SeccompProfile{
		DefaultAction: "SCMP_ACT_ALLOW",
		Syscalls: []SeccompSyscallRule{
			{Names: []string{"openat"}, Action: "SCMP_ACT_LOG"},
			{Names: []string{"ptrace"}, Action: "SCMP_ACT_TRACE"},
		},
	}
	assertSeccompDecision(t, profile, "openat", SeccompAllow)
	assertSeccompDecision(t, profile, "ptrace", SeccompUnknown)
	assertSeccompDecision(t, profile, "getpid", SeccompAllow)
}

func TestSeccompConflictingRulesRemainUnknown(t *testing.T) {
	profile := SeccompProfile{
		DefaultAction: "SCMP_ACT_ERRNO",
		Syscalls: []SeccompSyscallRule{
			{Names: []string{"bpf"}, Action: "SCMP_ACT_ALLOW"},
			{Names: []string{"bpf"}, Action: "SCMP_ACT_ERRNO"},
		},
	}
	assertSeccompDecision(t, profile, "bpf", SeccompUnknown)
}

func TestLoadSeccompProfileRejectsMissingDefaultAction(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.json")
	if err := os.WriteFile(path, []byte(`{"syscalls":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSeccompProfile(path); err == nil {
		t.Fatal("expected missing defaultAction to fail")
	}
}

func assertSeccompDecision(t *testing.T, profile SeccompProfile, syscall string, want SeccompDecision) {
	t.Helper()
	got, _ := profile.Decision(syscall)
	if got != want {
		t.Fatalf("Decision(%q)=%q want %q", syscall, got, want)
	}
}

var _ = context.Background
