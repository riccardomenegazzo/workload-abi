package scenario

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Scenario struct {
	Name              string            `json:"name"`
	Command           []string          `json:"command,omitempty"`
	Environment       map[string]string `json:"environment,omitempty"`
	PublishPorts      []int             `json:"publish_ports,omitempty"`
	SettleMillis      int               `json:"settle_ms,omitempty"`
	ObservationMillis int               `json:"observation_ms,omitempty"`
	SampleMillis      int               `json:"sample_interval_ms,omitempty"`
	ShutdownMillis    int               `json:"shutdown_timeout_ms,omitempty"`
	Probes            []Probe           `json:"probes,omitempty"`
}

type Probe struct {
	Port         int               `json:"port"`
	Method       string            `json:"method,omitempty"`
	Path         string            `json:"path,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Body         string            `json:"body,omitempty"`
	ExpectStatus int               `json:"expect_status,omitempty"`
	Repeat       int               `json:"repeat,omitempty"`
	IntervalMS   int               `json:"interval_ms,omitempty"`
}

func Default() Scenario {
	return Scenario{Name: "default", SettleMillis: 500, ObservationMillis: 2500, SampleMillis: 300, ShutdownMillis: 15000}
}

func Load(path string) (Scenario, error) {
	if path == "" {
		return Default(), nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Scenario{}, fmt.Errorf("read scenario: %w", err)
	}
	s := Default()
	if err := json.Unmarshal(b, &s); err != nil {
		return Scenario{}, fmt.Errorf("decode scenario: %w", err)
	}
	if s.Name == "" {
		s.Name = "unnamed"
	}
	if s.SampleMillis <= 0 {
		s.SampleMillis = 300
	}
	if s.ObservationMillis <= 0 {
		s.ObservationMillis = 2500
	}
	if s.ShutdownMillis <= 0 {
		s.ShutdownMillis = 15000
	}
	return s, nil
}

func (s Scenario) SettleDuration() time.Duration      { return time.Duration(s.SettleMillis) * time.Millisecond }
func (s Scenario) ObservationDuration() time.Duration { return time.Duration(s.ObservationMillis) * time.Millisecond }
func (s Scenario) SampleDuration() time.Duration      { return time.Duration(s.SampleMillis) * time.Millisecond }
func (s Scenario) ShutdownDuration() time.Duration    { return time.Duration(s.ShutdownMillis) * time.Millisecond }
