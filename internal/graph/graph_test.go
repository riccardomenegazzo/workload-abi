package graph

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

func TestBuildCreatesCausalProcessResourceChain(t *testing.T) {
	s := model.Snapshot{
		SchemaVersion: model.SchemaVersion,
		Image:         "app:v2",
		Fingerprint:   "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		RuntimeEvents: []model.RuntimeEvent{
			{Category: "process", Operation: "spawn", Process: "/usr/bin/helper", ParentProcess: "/usr/bin/app"},
			{Category: "file", Operation: "read", Process: "/usr/bin/helper", Target: "/var/run/secrets/token"},
			{Category: "network", Operation: "connect", Direction: "outbound", Process: "/usr/bin/helper", Target: "api.vendor.com:443", Protocol: "tcp"},
		},
	}
	g := Build(s)
	if g.SchemaVersion != SchemaVersion {
		t.Fatalf("schema=%q", g.SchemaVersion)
	}
	if g.Fingerprint == "" {
		t.Fatal("missing graph fingerprint")
	}
	want := map[string]bool{
		"process:/usr/bin/app\x00spawn\x00process:/usr/bin/helper":                 false,
		"process:/usr/bin/helper\x00read\x00file:/var/run/secrets/token":          false,
		"process:/usr/bin/helper\x00connect:outbound\x00endpoint:api.vendor.com:443": false,
	}
	for _, edge := range g.Edges {
		if _, ok := want[edge.Key()]; ok {
			want[edge.Key()] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Fatalf("missing edge %q in %#v", key, g.Edges)
		}
	}
}

func TestGraphFingerprintIgnoresImageReference(t *testing.T) {
	a := Build(model.Snapshot{
		SchemaVersion: model.SchemaVersion,
		Image:         "app:v1",
		Fingerprint:   "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		RuntimeEvents: []model.RuntimeEvent{{Category: "file", Operation: "read", Process: "/app", Target: "/etc/config"}},
	})
	b := a
	b.Image = "registry.example/app:renamed"
	if Fingerprint(a) != Fingerprint(b) {
		t.Fatal("graph fingerprint changed for display image metadata")
	}
}

func TestCompareExplainsNewProcessChain(t *testing.T) {
	base := Build(model.Snapshot{
		SchemaVersion: model.SchemaVersion,
		Fingerprint:   "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		RuntimeEvents: []model.RuntimeEvent{{Category: "file", Operation: "open", Process: "/usr/bin/app", Target: "/etc/app/config.yaml"}},
	})
	candidate := Build(model.Snapshot{
		SchemaVersion: model.SchemaVersion,
		Fingerprint:   "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		RuntimeEvents: []model.RuntimeEvent{
			{Category: "file", Operation: "open", Process: "/usr/bin/app", Target: "/etc/app/config.yaml"},
			{Category: "process", Operation: "spawn", Process: "/usr/bin/helper", ParentProcess: "/usr/bin/app"},
			{Category: "file", Operation: "read", Process: "/usr/bin/helper", Target: "/var/run/secrets/token"},
			{Category: "network", Operation: "connect", Direction: "outbound", Process: "/usr/bin/helper", Target: "api.vendor.com:443"},
		},
	})

	d := Compare(base, candidate)
	if d.Verdict != "CHANGED" {
		t.Fatalf("verdict=%q", d.Verdict)
	}
	if len(d.Explanations) == 0 {
		t.Fatal("missing causal explanation")
	}
	got := d.Explanations[0].Summary
	for _, fragment := range []string{"/usr/bin/helper", "/usr/bin/app", "/var/run/secrets/token", "api.vendor.com:443"} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("explanation %q missing %q", got, fragment)
		}
	}
}

func TestLoadRejectsTamperedGraph(t *testing.T) {
	g := Build(model.Snapshot{
		SchemaVersion: model.SchemaVersion,
		Fingerprint:   "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		RuntimeEvents: []model.RuntimeEvent{{Category: "file", Operation: "read", Process: "/app", Target: "/etc/config"}},
	})
	g.Edges[0].Relation = "write"
	data, _ := json.Marshal(g)
	path := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected graph fingerprint mismatch")
	}
}
