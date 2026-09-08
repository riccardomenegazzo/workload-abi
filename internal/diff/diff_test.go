package diff

import (
	"testing"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

func TestCompareFindsOperationalChanges(t *testing.T) {
	a := model.RuntimeGraph{
		Image:      model.ImageIdentity{Reference: "app:v1"},
		Processes:  []model.Process{{Name: "app"}},
		Filesystem: []model.FileMutation{{Kind: "A", Path: "/tmp/cache"}},
		Resources:  model.ResourceProfile{PeakMemoryBytes: 32 << 20},
		Lifecycle:  model.LifecycleProfile{ShutdownMillis: 500},
	}
	b := model.RuntimeGraph{
		Image:      model.ImageIdentity{Reference: "app:v2"},
		Processes:  []model.Process{{Name: "app"}, {Name: "sh"}},
		Filesystem: []model.FileMutation{{Kind: "A", Path: "/tmp/cache"}, {Kind: "A", Path: "/var/lib/app/cache"}},
		Resources:  model.ResourceProfile{PeakMemoryBytes: 80 << 20},
		Lifecycle:  model.LifecycleProfile{ShutdownMillis: 7000},
	}
	r := Compare(a, b)
	if r.Summary.Total < 4 {
		t.Fatalf("expected at least 4 changes, got %+v", r.Summary)
	}
	found := false
	for _, c := range r.Changes {
		if c.Surface == "filesystem" && c.Kind == "added" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected filesystem change")
	}
}

func TestCompareIgnoresInboundEphemeralSourcePorts(t *testing.T) {
	a := model.RuntimeGraph{Network: []model.NetworkEndpoint{{Protocol: "tcp", Direction: "inbound", LocalPort: 8080, RemoteAddress: "172.17.0.1", RemotePort: 51001}}}
	b := model.RuntimeGraph{Network: []model.NetworkEndpoint{{Protocol: "tcp", Direction: "inbound", LocalPort: 8080, RemoteAddress: "172.17.0.1", RemotePort: 52777}}}

	r := Compare(a, b)
	for _, c := range r.Changes {
		if c.Surface == "network" {
			t.Fatalf("ephemeral inbound source port must not change the Operational ABI: %+v", c)
		}
	}
}

func TestCompareCompactsDockerDiffParentDirectories(t *testing.T) {
	a := model.RuntimeGraph{Filesystem: []model.FileMutation{{Kind: "A", Path: "/tmp/cache"}, {Kind: "C", Path: "/tmp"}}}
	b := model.RuntimeGraph{Filesystem: []model.FileMutation{{Kind: "C", Path: "/var"}, {Kind: "C", Path: "/var/lib"}, {Kind: "A", Path: "/var/lib/app"}, {Kind: "A", Path: "/var/lib/app/cache"}}}

	r := Compare(a, b)
	var added []string
	for _, c := range r.Changes {
		if c.Surface == "filesystem" && c.Kind == "added" {
			added = append(added, c.Key)
		}
	}
	if len(added) != 1 || added[0] != "A|/var/lib/app/cache" {
		t.Fatalf("expected only the leaf filesystem mutation, got %v", added)
	}
}
