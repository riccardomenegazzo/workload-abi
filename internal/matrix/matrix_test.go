package matrix

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/riccardomenegazzo/workload-abi/internal/environment"
	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

func TestLoadConfigResolvesRelativeArtifacts(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "matrix.json")
	data := `{
	  "schema_version":"wabi.matrix-config/v1alpha1",
	  "environments":[{
	    "name":"production",
	    "target":"targets/task.json",
	    "target_kind":"ecs",
	    "container":"api",
	    "seccomp_profile":"policies/seccomp.json"
	  }]
	}`
	if err := os.WriteFile(configPath, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Environments) != 1 {
		t.Fatalf("environments=%d", len(config.Environments))
	}
	env := config.Environments[0]
	if env.TargetFile != filepath.Join(dir, "targets", "task.json") {
		t.Fatalf("target=%q", env.TargetFile)
	}
	if env.SeccompProfile != filepath.Join(dir, "policies", "seccomp.json") {
		t.Fatalf("seccomp=%q", env.SeccompProfile)
	}
}

func TestBuildProducesPerEnvironmentVerdictsAndFingerprint(t *testing.T) {
	dir := t.TempDir()
	allow := filepath.Join(dir, "allow.json")
	deny := filepath.Join(dir, "deny.json")
	if err := os.WriteFile(allow, []byte(`{"defaultAction":"SCMP_ACT_ALLOW","syscalls":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deny, []byte(`{"defaultAction":"SCMP_ACT_ALLOW","syscalls":[{"names":["bpf"],"action":"SCMP_ACT_ERRNO"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	base := model.Snapshot{Image: "app:v1", Fingerprint: "sha256:base"}
	candidate := model.Snapshot{
		Image:       "app:v2",
		Fingerprint: "sha256:candidate",
		RuntimeEvents: []model.RuntimeEvent{{
			Category: "syscall", Operation: "bpf", Process: "/usr/bin/app",
		}},
	}
	config := Config{
		SchemaVersion: ConfigSchemaVersion,
		Environments: []EnvironmentSpec{
			{Name: "developer", Options: environment.Options{SeccompProfile: allow}},
			{Name: "production", Options: environment.Options{SeccompProfile: deny}},
		},
	}

	artifact, err := Build(context.Background(), base, candidate, config)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.SchemaVersion != ArtifactSchemaVersion || artifact.Fingerprint == "" {
		t.Fatalf("unexpected artifact metadata: %#v", artifact)
	}
	if artifact.Verdict != "BREAKING" || artifact.Summary.Breaking != 1 || artifact.Summary.Changed != 1 {
		t.Fatalf("unexpected summary: verdict=%s summary=%#v", artifact.Verdict, artifact.Summary)
	}
	if artifact.Environments[0].Verdict != "CHANGED" || artifact.Environments[1].Verdict != "BREAKING" {
		t.Fatalf("unexpected environment verdicts: %#v", artifact.Environments)
	}
	if err := Verify(artifact); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRejectsTamperedMatrix(t *testing.T) {
	artifact := Artifact{
		SchemaVersion: ArtifactSchemaVersion,
		Baseline:      "app:v1",
		Candidate:     "app:v2",
		Verdict:       "COMPATIBLE",
		Summary:       Summary{Compatible: 1},
		Environments:  []EnvironmentResult{{Name: "dev", Verdict: "COMPATIBLE"}},
	}
	artifact.Fingerprint = Fingerprint(artifact)
	artifact.Environments[0].Verdict = "BREAKING"
	if err := Verify(artifact); err == nil {
		t.Fatal("expected tampered matrix artifact to fail verification")
	}
}

func TestLoadConfigRejectsDuplicateNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "matrix.json")
	data := `{"schema_version":"wabi.matrix-config/v1alpha1","environments":[{"name":"prod","seccomp_profile":"a.json"},{"name":"prod","seccomp_profile":"b.json"}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected duplicate environment names to fail")
	}
}
