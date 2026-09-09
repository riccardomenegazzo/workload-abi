package target

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadECSSelectsContainerAndHardConstraints(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "task-definition.json")
	data := `{
	  "family":"payments",
	  "memory":"512",
	  "containerDefinitions":[{
	    "name":"api",
	    "memory":256,
	    "memoryReservation":128,
	    "readonlyRootFilesystem":true,
	    "stopTimeout":15,
	    "user":"10001",
	    "privileged":false,
	    "mountPoints":[
	      {"sourceVolume":"tmp","containerPath":"/tmp","readOnly":false},
	      {"sourceVolume":"config","containerPath":"/etc/app","readOnly":true}
	    ],
	    "linuxParameters":{"capabilities":{"add":["NET_BIND_SERVICE"],"drop":["SYS_ADMIN"]}}
	  }]
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := LoadECS(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Family != "payments" || got.Container != "api" || got.Identity() != "ecs:payments#api" {
		t.Fatalf("unexpected target identity: %#v", got)
	}
	if !got.ReadOnlyRootfs || got.Privileged || got.User != "10001" {
		t.Fatalf("unexpected container constraints: %#v", got)
	}
	if got.ContainerMemory != 256*mib || got.TaskMemory != 512*mib || got.MemoryHardLimit != 256*mib {
		t.Fatalf("unexpected hard memory constraints: %#v", got)
	}
	if got.MemoryReservation != 128*mib {
		t.Fatalf("memory reservation=%d", got.MemoryReservation)
	}
	if got.StopTimeout.Seconds() != 15 {
		t.Fatalf("stop timeout=%s", got.StopTimeout)
	}
	if len(got.WritablePaths) != 1 || got.WritablePaths[0] != "/tmp" {
		t.Fatalf("writable paths=%#v", got.WritablePaths)
	}
	if len(got.CapAdd) != 1 || got.CapAdd[0] != "NET_BIND_SERVICE" || len(got.CapDrop) != 1 || got.CapDrop[0] != "SYS_ADMIN" {
		t.Fatalf("capabilities add=%#v drop=%#v", got.CapAdd, got.CapDrop)
	}
}

func TestLoadECSUsesTaskMemoryWhenItIsLower(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "task.json")
	data := `{"family":"demo","memory":"256","containerDefinitions":[{"name":"api","memory":512}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadECS(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.MemoryHardLimit != 256*mib {
		t.Fatalf("hard limit=%d want %d", got.MemoryHardLimit, 256*mib)
	}
}

func TestLoadECSRequiresContainerSelectionForMultipleContainers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "task.json")
	data := `{"containerDefinitions":[{"name":"api"},{"name":"sidecar"}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadECS(path, ""); err == nil {
		t.Fatal("expected ambiguous ECS container selection to fail")
	}
	got, err := LoadECS(path, "sidecar")
	if err != nil {
		t.Fatal(err)
	}
	if got.Container != "sidecar" {
		t.Fatalf("container=%q", got.Container)
	}
}

func TestLoadECSMemoryReservationIsNotHardLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "task.json")
	data := `{"family":"demo","containerDefinitions":[{"name":"api","memoryReservation":128}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadECS(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.MemoryReservation != 128*mib || got.MemoryHardLimit != 0 {
		t.Fatalf("reservation=%d hard=%d", got.MemoryReservation, got.MemoryHardLimit)
	}
}
