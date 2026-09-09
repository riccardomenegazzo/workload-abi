package evidence

import (
	"strings"
	"testing"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

func TestFalcoNormalizesFileEvent(t *testing.T) {
	input := `{"rule":"Read sensitive file","output_fields":{"evt.type":"openat","proc.exepath":"/usr/bin/cat","proc.pexepath":"/bin/sh","proc.pid":42,"fd.name":"/etc/shadow"}}`
	events, err := Decode(strings.NewReader(input), "falco")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	got := events[0]
	if got.Category != "file" || got.Operation != "open" || got.Process != "/usr/bin/cat" || got.ParentProcess != "/bin/sh" || got.Target != "/etc/shadow" {
		t.Fatalf("unexpected normalized Falco event: %#v", got)
	}
	if got.Source != "falco" || got.Rule != "Read sensitive file" || got.PID != 42 {
		t.Fatalf("diagnostic context was not preserved: %#v", got)
	}
}

func TestFalcoNormalizesOutboundNetworkEvent(t *testing.T) {
	input := `{"rule":"Outbound connection","output_fields":{"evt.type":"connect","proc.name":"curl","fd.rip.name":"api.example.com","fd.rport":443,"fd.l4proto":"tcp"}}`
	events, err := Decode(strings.NewReader(input), "falco")
	if err != nil {
		t.Fatal(err)
	}
	got := events[0]
	if got.Category != "network" || got.Operation != "connect" || got.Direction != "outbound" || got.Target != "api.example.com:443" || got.Protocol != "tcp" {
		t.Fatalf("unexpected normalized network event: %#v", got)
	}
}

func TestTraceeNormalizesFileEvent(t *testing.T) {
	input := `{"processId":2045288,"processName":"bash","eventName":"security_file_open","args":[{"name":"pathname","type":"const char*","value":"/usr/bin/exa"}]}`
	events, err := Decode(strings.NewReader(input), "tracee")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	got := events[0]
	if got.Source != "tracee" || got.Category != "file" || got.Operation != "open" || got.Process != "bash" || got.Target != "/usr/bin/exa" || got.PID != 2045288 {
		t.Fatalf("unexpected normalized Tracee event: %#v", got)
	}
}

func TestTraceeNormalizesExecEvent(t *testing.T) {
	input := `{"processId":7,"processName":"python","eventName":"execve","executable":{"path":"/usr/bin/python3"},"args":[]}`
	events, err := Decode(strings.NewReader(input), "tracee")
	if err != nil {
		t.Fatal(err)
	}
	got := events[0]
	if got.Category != "process" || got.Operation != "exec" || got.Process != "/usr/bin/python3" {
		t.Fatalf("unexpected normalized exec event: %#v", got)
	}
}

func TestGenericAcceptsArrayAndDeduplicatesBySemanticIdentity(t *testing.T) {
	input := `[
	  {"source":"sensor-a","category":"network","operation":"connect","process":"curl","target":"api.example:443","protocol":"tcp","direction":"outbound","pid":10},
	  {"source":"sensor-b","category":"network","operation":"connect","process":"curl","target":"api.example:443","protocol":"tcp","direction":"outbound","pid":99,"rule":"diagnostic"}
	]`
	events, err := Decode(strings.NewReader(input), "generic")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected semantic dedupe, got %#v", events)
	}
}

func TestMergeUpgradesSchemaAndFingerprint(t *testing.T) {
	s := model.Snapshot{
		SchemaVersion: model.SchemaVersionV1Alpha2,
		Image:         "example/app:v1",
		Processes:     []model.Process{{Command: "app"}},
	}
	s.Fingerprint = model.Fingerprint(s)
	old := s.Fingerprint
	merged := Merge(s, []model.RuntimeEvent{{Category: "file", Operation: "open", Process: "app", Target: "/etc/config"}})
	if merged.SchemaVersion != model.SchemaVersion {
		t.Fatalf("expected current schema, got %s", merged.SchemaVersion)
	}
	if merged.Fingerprint == old {
		t.Fatal("deep evidence did not change operational fingerprint")
	}
	if len(merged.RuntimeEvents) != 1 {
		t.Fatalf("expected merged runtime event, got %#v", merged.RuntimeEvents)
	}
}

func TestUnknownSyscallRemainsProviderNeutralSyscallEvidence(t *testing.T) {
	input := `{"eventName":"io_uring_setup","processName":"worker","args":[]}`
	events, err := Decode(strings.NewReader(input), "tracee")
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Category != "syscall" || events[0].Operation != "io_uring_setup" {
		t.Fatalf("unexpected syscall normalization: %#v", events[0])
	}
}
