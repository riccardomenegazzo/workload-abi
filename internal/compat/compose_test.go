package compat

import (
	"testing"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

func TestEvaluateComposeDetectsReadOnlyMemoryAndGraceConflicts(t *testing.T) {
	p := ComposeProject{Services: map[string]map[string]any{
		"app": {
			"image":             "app:v2",
			"read_only":         true,
			"tmpfs":             []any{"/tmp"},
			"mem_limit":         "64m",
			"stop_grace_period": "5s",
		},
	}}
	a := model.RuntimeGraph{
		Image:     model.ImageIdentity{Reference: "app:v1"},
		Resources: model.ResourceProfile{PeakMemoryBytes: 30 << 20},
	}
	b := model.RuntimeGraph{
		Image:     model.ImageIdentity{Reference: "app:v2"},
		Resources: model.ResourceProfile{PeakMemoryBytes: 90 << 20},
		Lifecycle: model.LifecycleProfile{ShutdownMillis: 9000},
	}
	d := model.DiffResult{Changes: []model.Change{{
		Surface: "filesystem",
		Kind:    "added",
		After:   model.FileMutation{Kind: "A", Path: "/var/lib/app/cache"},
	}}}

	r, err := EvaluateComposeConfig("compose.yaml", "app", p, a, b, d)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != model.StatusBreaking {
		t.Fatalf("want breaking, got %s", r.Status)
	}
	if len(r.Conflicts) != 3 {
		t.Fatalf("want 3 conflicts, got %d: %+v", len(r.Conflicts), r.Conflicts)
	}
}
