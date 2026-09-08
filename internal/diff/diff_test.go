package diff

import (
	"testing"
	"time"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

func TestCompareDetectsOperationalChanges(t *testing.T) {
	base := model.Snapshot{
		Image: "app:v1",
		Processes: []model.Process{{Command: "app --serve"}},
		Filesystem: []model.FilesystemChange{{Kind: "C", Path: "/tmp/cache"}},
		ImageConfig: model.ImageConfig{User: "1000", ExposedPorts: []string{"8080/tcp"}},
		Lifecycle: model.Lifecycle{StopDuration: time.Second},
	}
	candidate := model.Snapshot{
		Image: "app:v2",
		Processes: []model.Process{{Command: "app --serve"}, {Command: "curl telemetry.example"}},
		Filesystem: []model.FilesystemChange{{Kind: "C", Path: "/tmp/cache"}, {Kind: "A", Path: "/var/lib/app/state"}},
		ImageConfig: model.ImageConfig{User: "0", ExposedPorts: []string{"8080/tcp", "9090/tcp"}},
		Lifecycle: model.Lifecycle{StopDuration: 3 * time.Second},
	}

	got := Compare(base, candidate)
	if got.Verdict != "CHANGED" {
		t.Fatalf("verdict=%s want CHANGED", got.Verdict)
	}
	if len(got.Changes) < 5 {
		t.Fatalf("expected multiple semantic changes, got %d", len(got.Changes))
	}
}

func TestCompareMarksFailedCandidateBreaking(t *testing.T) {
	base := model.Snapshot{Image: "app:v1", Runtime: model.RuntimeFacts{ExitCode: 0}}
	candidate := model.Snapshot{Image: "app:v2", Runtime: model.RuntimeFacts{ExitCode: 17}}
	got := Compare(base, candidate)
	if got.Verdict != "BREAKING" {
		t.Fatalf("verdict=%s want BREAKING", got.Verdict)
	}
}

func TestCompareIdenticalSnapshotsCompatible(t *testing.T) {
	s := model.Snapshot{Image: "app:v1", Processes: []model.Process{{Command: "app"}}}
	candidate := s
	candidate.Image = "app:v2"
	got := Compare(s, candidate)
	if got.Verdict != "COMPATIBLE" || len(got.Changes) != 0 {
		t.Fatalf("expected compatible empty diff, got %#v", got)
	}
}
