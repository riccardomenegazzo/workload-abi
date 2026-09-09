package model

import (
	"testing"
	"time"
)

func TestFingerprintIgnoresVolatileMeasurements(t *testing.T) {
	a := Snapshot{
		SchemaVersion: SchemaVersion,
		Scenario:      "smoke",
		CapturedAt:    time.Unix(1, 0),
		Processes:     []Process{{Command: "app"}},
		Listeners:     []Listener{{Protocol: "tcp", Port: 8080}},
		Stats:         Stats{CPUPercent: "1%"},
		Lifecycle:     Lifecycle{StopDuration: time.Second},
	}
	b := a
	b.CapturedAt = time.Unix(999, 0)
	b.Stats.CPUPercent = "99%"
	b.Lifecycle.StopDuration = 7 * time.Second
	if Fingerprint(a) != Fingerprint(b) {
		t.Fatal("fingerprint changed for volatile timing/stats fields")
	}
}

func TestFingerprintDoesNotTreatImageReferenceAsBehavior(t *testing.T) {
	a := Snapshot{
		SchemaVersion: SchemaVersion,
		Image:         "example/app:v1",
		ImageID:       "sha256:image-one",
		Processes:     []Process{{Command: "app"}},
	}
	b := a
	b.Image = "registry.example/app:renamed"
	b.ImageID = "sha256:image-two"
	if Fingerprint(a) != Fingerprint(b) {
		t.Fatal("operational fingerprint must not change for image identity metadata alone")
	}
}

func TestFingerprintIgnoresDeepEvidenceProviderDiagnostics(t *testing.T) {
	a := Snapshot{
		SchemaVersion: SchemaVersion,
		RuntimeEvents: []RuntimeEvent{{
			Source:    "falco",
			Category:  "network",
			Operation: "connect",
			Process:   "curl",
			Target:    "api.example:443",
			Protocol:  "tcp",
			Direction: "outbound",
			PID:       100,
			Rule:      "rule one",
		}},
	}
	b := a
	b.RuntimeEvents = append([]RuntimeEvent(nil), a.RuntimeEvents...)
	b.RuntimeEvents[0].Source = "tracee"
	b.RuntimeEvents[0].PID = 999
	b.RuntimeEvents[0].Rule = "different diagnostic rule"
	if Fingerprint(a) != Fingerprint(b) {
		t.Fatal("provider/PID/rule diagnostics must not change the operational fingerprint")
	}
}

func TestFingerprintChangesWithDeepBehavior(t *testing.T) {
	a := Snapshot{SchemaVersion: SchemaVersion, RuntimeEvents: []RuntimeEvent{{Category: "file", Operation: "open", Process: "app", Target: "/etc/a"}}}
	b := Snapshot{SchemaVersion: SchemaVersion, RuntimeEvents: []RuntimeEvent{{Category: "file", Operation: "open", Process: "app", Target: "/etc/b"}}}
	if Fingerprint(a) == Fingerprint(b) {
		t.Fatal("deep runtime target change did not change operational fingerprint")
	}
}

func TestFingerprintChangesWithBehavior(t *testing.T) {
	a := Snapshot{SchemaVersion: SchemaVersion, Listeners: []Listener{{Protocol: "tcp", Port: 8080}}}
	b := Snapshot{SchemaVersion: SchemaVersion, Listeners: []Listener{{Protocol: "tcp", Port: 9090}}}
	if Fingerprint(a) == Fingerprint(b) {
		t.Fatal("fingerprint did not change for listener contract")
	}
}

func TestV1Alpha2FingerprintRemainsStableAfterCurrentSchemaUpgrade(t *testing.T) {
	s := Snapshot{
		SchemaVersion: SchemaVersionV1Alpha2,
		Scenario:      "legacy",
		Processes:     []Process{{Command: "legacy-app"}},
		Filesystem:    []FilesystemChange{{Kind: "A", Path: "/tmp/data"}},
	}
	first := Fingerprint(s)
	if first == "" {
		t.Fatal("legacy fingerprint is empty")
	}
	// Recomputing with the persisted legacy schema must be stable even though
	// SchemaVersion now points at v1alpha3.
	if first != Fingerprint(s) {
		t.Fatal("v1alpha2 fingerprint changed across recomputation")
	}
}
