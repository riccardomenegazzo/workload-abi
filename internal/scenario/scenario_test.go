package scenario

import "testing"

func TestDefaultScenarioHasSafeDurations(t *testing.T) {
	s := Default()
	if s.SampleMillis <= 0 || s.ObservationMillis <= 0 || s.ShutdownMillis <= 0 { t.Fatalf("invalid defaults: %+v", s) }
}
