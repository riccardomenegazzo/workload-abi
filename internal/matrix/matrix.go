package matrix

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/riccardomenegazzo/workload-abi/internal/diff"
	"github.com/riccardomenegazzo/workload-abi/internal/environment"
	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

const (
	ConfigSchemaVersion   = "wabi.matrix-config/v1alpha1"
	ArtifactSchemaVersion = "wabi.matrix/v1alpha1"
)

type Config struct {
	SchemaVersion string            `json:"schema_version"`
	Environments []EnvironmentSpec `json:"environments"`
}

type EnvironmentSpec struct {
	Name string `json:"name"`
	environment.Options
}

type Artifact struct {
	SchemaVersion        string              `json:"schema_version"`
	Baseline             string              `json:"baseline"`
	Candidate            string              `json:"candidate"`
	BaselineFingerprint  string              `json:"baseline_fingerprint"`
	CandidateFingerprint string              `json:"candidate_fingerprint"`
	Scenario             string              `json:"scenario,omitempty"`
	Verdict              string              `json:"verdict"`
	Summary              Summary             `json:"summary"`
	Environments         []EnvironmentResult `json:"environments"`
	Fingerprint          string              `json:"fingerprint"`
}

type Summary struct {
	Compatible int `json:"compatible"`
	Changed    int `json:"changed"`
	Breaking   int `json:"breaking"`
}

type EnvironmentResult struct {
	Name    string         `json:"name"`
	Target  string         `json:"target,omitempty"`
	Policy  string         `json:"policy,omitempty"`
	Verdict string         `json:"verdict"`
	Changes []model.Change `json:"changes"`
}

func LoadConfig(path string) (Config, error) {
	var config Config
	data, err := os.ReadFile(path)
	if err != nil {
		return config, fmt.Errorf("read matrix config: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return config, fmt.Errorf("parse matrix config: %w", err)
	}
	if config.SchemaVersion == "" {
		config.SchemaVersion = ConfigSchemaVersion
	}
	if config.SchemaVersion != ConfigSchemaVersion {
		return config, fmt.Errorf("unsupported matrix config schema %q", config.SchemaVersion)
	}
	if len(config.Environments) == 0 {
		return config, fmt.Errorf("matrix config has no environments")
	}

	baseDir := filepath.Dir(path)
	seen := map[string]struct{}{}
	for i := range config.Environments {
		env := &config.Environments[i]
		env.Name = strings.TrimSpace(env.Name)
		if env.Name == "" {
			return config, fmt.Errorf("matrix environment %d has no name", i)
		}
		if _, exists := seen[env.Name]; exists {
			return config, fmt.Errorf("duplicate matrix environment name %q", env.Name)
		}
		seen[env.Name] = struct{}{}
		if env.TargetFile == "" && env.SeccompProfile == "" && env.PolicyFile == "" {
			return config, fmt.Errorf("matrix environment %q has no target, seccomp profile, or policy", env.Name)
		}
		env.TargetFile = resolvePath(baseDir, env.TargetFile)
		env.NetworkPolicyFile = resolvePath(baseDir, env.NetworkPolicyFile)
		env.SeccompProfile = resolvePath(baseDir, env.SeccompProfile)
		env.PolicyFile = resolvePath(baseDir, env.PolicyFile)
	}
	return config, nil
}

func Build(ctx context.Context, base, candidate model.Snapshot, config Config) (Artifact, error) {
	artifact := Artifact{
		SchemaVersion:        ArtifactSchemaVersion,
		Baseline:             base.Image,
		Candidate:            candidate.Image,
		BaselineFingerprint:  base.Fingerprint,
		CandidateFingerprint: candidate.Fingerprint,
		Scenario:             base.Scenario,
		Verdict:              "COMPATIBLE",
	}

	for _, env := range config.Environments {
		comparison := diff.Compare(base, candidate)
		comparison.Scenario = base.Scenario
		resolved, err := environment.Apply(ctx, comparison, base, candidate, env.Options)
		if err != nil {
			return artifact, fmt.Errorf("environment %q: %w", env.Name, err)
		}
		result := EnvironmentResult{
			Name:    env.Name,
			Target:  resolved.Target,
			Policy:  resolved.Policy,
			Verdict: resolved.Verdict,
			Changes: append([]model.Change(nil), resolved.Changes...),
		}
		artifact.Environments = append(artifact.Environments, result)
		switch result.Verdict {
		case "BREAKING":
			artifact.Summary.Breaking++
			artifact.Verdict = "BREAKING"
		case "CHANGED":
			artifact.Summary.Changed++
			if artifact.Verdict != "BREAKING" {
				artifact.Verdict = "CHANGED"
			}
		default:
			artifact.Summary.Compatible++
		}
	}
	artifact.Fingerprint = Fingerprint(artifact)
	return artifact, nil
}

func Fingerprint(artifact Artifact) string {
	artifact.Fingerprint = ""
	data, _ := json.Marshal(artifact)
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func Verify(artifact Artifact) error {
	if artifact.SchemaVersion != ArtifactSchemaVersion {
		return fmt.Errorf("unsupported matrix artifact schema %q", artifact.SchemaVersion)
	}
	if artifact.Fingerprint == "" {
		return fmt.Errorf("matrix artifact has no fingerprint")
	}
	expected := Fingerprint(artifact)
	if artifact.Fingerprint != expected {
		return fmt.Errorf("matrix fingerprint mismatch: stored %s computed %s", artifact.Fingerprint, expected)
	}
	return nil
}

func LoadArtifact(path string) (Artifact, error) {
	var artifact Artifact
	data, err := os.ReadFile(path)
	if err != nil {
		return artifact, fmt.Errorf("read matrix artifact: %w", err)
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		return artifact, fmt.Errorf("parse matrix artifact: %w", err)
	}
	if err := Verify(artifact); err != nil {
		return artifact, err
	}
	return artifact, nil
}

func SortedBlockingSurfaces(result EnvironmentResult) []string {
	set := map[string]struct{}{}
	for _, change := range result.Changes {
		if strings.EqualFold(change.Severity, "breaking") {
			set[change.Surface] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for surface := range set {
		out = append(out, surface)
	}
	sort.Strings(out)
	return out
}

func resolvePath(baseDir, value string) string {
	value = strings.TrimSpace(value)
	if value == "" || filepath.IsAbs(value) {
		return value
	}
	return filepath.Clean(filepath.Join(baseDir, value))
}
