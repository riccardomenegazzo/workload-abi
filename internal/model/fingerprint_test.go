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

func TestFingerprintChangesWithBehavior(t *testing.T) {
	a := Snapshot{SchemaVersion: SchemaVersion, Listeners: []Listener{{Protocol: "tcp", Port: 8080}}}
	b := Snapshot{SchemaVersion: SchemaVersion, Listeners: []Listener{{Protocol: "tcp", Port: 9090}}}
	if Fingerprint(a) == Fingerprint(b) {
		t.Fatal("fingerprint did not change for listener contract")
	}
}
