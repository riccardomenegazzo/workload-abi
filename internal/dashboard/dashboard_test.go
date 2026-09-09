package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riccardomenegazzo/workload-abi/internal/graph"
	"github.com/riccardomenegazzo/workload-abi/internal/matrix"
	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

func TestLoadRenderAndHandler(t *testing.T) {
	dir := t.TempDir()
	base := model.Snapshot{SchemaVersion: model.SchemaVersion, Image: "app:v1", Scenario: "smoke"}
	base.Fingerprint = model.Fingerprint(base)
	candidate := model.Snapshot{
		SchemaVersion: model.SchemaVersion,
		Image:         "app:v2",
		Scenario:      "smoke",
		RuntimeEvents: []model.RuntimeEvent{{Category: "network", Operation: "connect", Process: "/app", Target: "api.example.com:443", Protocol: "tcp", Direction: "outbound"}},
	}
	candidate.Fingerprint = model.Fingerprint(candidate)

	artifact := matrix.Artifact{
		SchemaVersion:        matrix.ArtifactSchemaVersion,
		Baseline:             base.Image,
		Candidate:            candidate.Image,
		BaselineFingerprint:  base.Fingerprint,
		CandidateFingerprint: candidate.Fingerprint,
		Scenario:             "smoke",
		Verdict:              "BREAKING",
		Summary:              matrix.Summary{Changed: 1, Breaking: 1},
		Environments: []matrix.EnvironmentResult{
			{Name: "developer", Verdict: "CHANGED"},
			{Name: "production", Target: "kubernetes:deployment#api", Verdict: "BREAKING", Changes: []model.Change{{Surface: "runtime-network", Kind: "target-conflict", Severity: "breaking", Message: "blocked egress"}}},
		},
	}
	artifact.Fingerprint = matrix.Fingerprint(artifact)
	matrixPath := filepath.Join(dir, "matrix.json")
	writeJSON(t, matrixPath, artifact)

	baseGraph := graph.Build(base)
	candidateGraph := graph.Build(candidate)
	baseGraphPath := filepath.Join(dir, "base.graph.json")
	candidateGraphPath := filepath.Join(dir, "candidate.graph.json")
	writeJSON(t, baseGraphPath, baseGraph)
	writeJSON(t, candidateGraphPath, candidateGraph)

	data, err := Load(matrixPath, baseGraphPath, candidateGraphPath)
	if err != nil {
		t.Fatal(err)
	}
	if data.GraphDiff == nil || data.GraphDiff.Verdict != "CHANGED" {
		t.Fatalf("unexpected graph diff: %#v", data.GraphDiff)
	}
	page, err := Render(data)
	if err != nil {
		t.Fatal(err)
	}
	text := string(page)
	if strings.Contains(text, "__WABI_DASHBOARD_DATA__") || !strings.Contains(text, "app:v2") || !strings.Contains(text, "production") {
		t.Fatalf("rendered dashboard is missing embedded artifact data")
	}

	handler, err := Handler(data)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "default-src 'none'") {
		t.Fatalf("unexpected dashboard response: code=%d csp=%q", rec.Code, rec.Header().Get("Content-Security-Policy"))
	}
	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthRec := httptest.NewRecorder()
	handler.ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusOK || strings.TrimSpace(healthRec.Body.String()) != "ok" {
		t.Fatalf("unexpected health response: %d %q", healthRec.Code, healthRec.Body.String())
	}
}

func TestLoadRejectsGraphFromDifferentSnapshot(t *testing.T) {
	dir := t.TempDir()
	base := model.Snapshot{SchemaVersion: model.SchemaVersion, Image: "app:v1"}
	base.Fingerprint = model.Fingerprint(base)
	candidate := model.Snapshot{SchemaVersion: model.SchemaVersion, Image: "app:v2"}
	candidate.Fingerprint = model.Fingerprint(candidate)
	artifact := matrix.Artifact{
		SchemaVersion:        matrix.ArtifactSchemaVersion,
		Baseline:             base.Image,
		Candidate:            candidate.Image,
		BaselineFingerprint:  base.Fingerprint,
		CandidateFingerprint: candidate.Fingerprint,
		Verdict:              "COMPATIBLE",
		Environments:         []matrix.EnvironmentResult{{Name: "dev", Verdict: "COMPATIBLE"}},
		Summary:              matrix.Summary{Compatible: 1},
	}
	artifact.Fingerprint = matrix.Fingerprint(artifact)
	matrixPath := filepath.Join(dir, "matrix.json")
	writeJSON(t, matrixPath, artifact)

	wrong := model.Snapshot{SchemaVersion: model.SchemaVersion, Image: "other"}
	wrong.Fingerprint = model.Fingerprint(wrong)
	wrongGraph := graph.Build(wrong)
	candidateGraph := graph.Build(candidate)
	wrongPath := filepath.Join(dir, "wrong.graph.json")
	candidatePath := filepath.Join(dir, "candidate.graph.json")
	writeJSON(t, wrongPath, wrongGraph)
	writeJSON(t, candidatePath, candidateGraph)
	if _, err := Load(matrixPath, wrongPath, candidatePath); err == nil || !strings.Contains(err.Error(), "matrix expects") {
		t.Fatalf("expected snapshot binding error, got %v", err)
	}
}

func TestListenAddressSecurity(t *testing.T) {
	for _, address := range []string{"127.0.0.1:8787", "localhost:8787", "[::1]:8787"} {
		if err := ValidateListenAddress(address, false); err != nil {
			t.Fatalf("expected %s to be allowed: %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:8787", ":8787", "192.0.2.10:8787"} {
		if err := ValidateListenAddress(address, false); err == nil {
			t.Fatalf("expected %s to require --allow-remote", address)
		}
		if err := ValidateListenAddress(address, true); err != nil {
			t.Fatalf("allow-remote should permit %s: %v", address, err)
		}
	}
}

func TestServeStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	artifact := matrix.Artifact{SchemaVersion: matrix.ArtifactSchemaVersion, Verdict: "COMPATIBLE", Environments: []matrix.EnvironmentResult{{Name: "dev", Verdict: "COMPATIBLE"}}, Summary: matrix.Summary{Compatible: 1}}
	artifact.Fingerprint = matrix.Fingerprint(artifact)
	if err := Serve(ctx, "127.0.0.1:0", false, Data{Matrix: artifact}); err != nil {
		t.Fatal(err)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
