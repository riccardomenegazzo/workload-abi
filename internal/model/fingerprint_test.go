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

func TestFingerprintChangesWithBehavior(t *testing.T) {
	a := Snapshot{SchemaVersion: SchemaVersion, Listeners: []Listener{{Protocol: "tcp", Port: 8080}}}
	b := Snapshot{SchemaVersion: SchemaVersion, Listeners: []Listener{{Protocol: "tcp", Port: 9090}}}
	if Fingerprint(a) == Fingerprint(b) {
		t.Fatal("fingerprint did not change for listener contract")
	}
}
