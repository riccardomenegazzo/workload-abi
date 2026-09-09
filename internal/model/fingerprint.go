package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

type fingerprintStep struct {
	Name         string   `json:"name"`
	Command      []string `json:"command"`
	ExitCode     int      `json:"exit_code"`
	Success      bool     `json:"success"`
	AllowFailure bool     `json:"allow_failure,omitempty"`
}

type fingerprintRuntimeEvent struct {
	Category      string `json:"category"`
	Operation     string `json:"operation"`
	Process       string `json:"process,omitempty"`
	ParentProcess string `json:"parent_process,omitempty"`
	Target        string `json:"target,omitempty"`
	Protocol      string `json:"protocol,omitempty"`
	Direction     string `json:"direction,omitempty"`
}

type fingerprintPayload struct {
	SchemaVersion string                    `json:"schema_version"`
	Scenario      string                    `json:"scenario,omitempty"`
	Processes     []Process                 `json:"processes,omitempty"`
	Filesystem    []FilesystemChange        `json:"filesystem,omitempty"`
	Listeners     []Listener                `json:"listeners,omitempty"`
	RuntimeEvents []fingerprintRuntimeEvent `json:"runtime_events,omitempty"`
	Steps         []fingerprintStep         `json:"steps,omitempty"`
	ImageConfig   ImageConfig               `json:"image_config"`
	Runtime       RuntimeFacts              `json:"runtime"`
}

func Fingerprint(s Snapshot) string {
	schema := s.SchemaVersion
	if schema == "" {
		// Snapshots produced before explicit schema versioning used the v1alpha2
		// fingerprint shape. Keeping this default lets old persisted evidence
		// remain verifiable after v1alpha3 becomes the current writer schema.
		schema = SchemaVersionV1Alpha2
	}
	p := fingerprintPayload{
		SchemaVersion: schema,
		Scenario:      s.Scenario,
		Processes:     append([]Process(nil), s.Processes...),
		Filesystem:    append([]FilesystemChange(nil), s.Filesystem...),
		Listeners:     append([]Listener(nil), s.Listeners...),
		ImageConfig:   s.ImageConfig,
		Runtime:       s.Runtime,
	}
	p.Runtime.ContainerStatus = ""
	p.Runtime.ExitCode = s.Runtime.ExitCode
	p.Runtime.Networks = append([]string(nil), s.Runtime.Networks...)
	sort.Strings(p.Runtime.Networks)

	sort.Slice(p.Processes, func(i, j int) bool { return p.Processes[i].Command < p.Processes[j].Command })
	sort.Slice(p.Filesystem, func(i, j int) bool {
		if p.Filesystem[i].Path == p.Filesystem[j].Path {
			return p.Filesystem[i].Kind < p.Filesystem[j].Kind
		}
		return p.Filesystem[i].Path < p.Filesystem[j].Path
	})
	sort.Slice(p.Listeners, func(i, j int) bool {
		if p.Listeners[i].Protocol == p.Listeners[j].Protocol {
			return p.Listeners[i].Port < p.Listeners[j].Port
		}
		return p.Listeners[i].Protocol < p.Listeners[j].Protocol
	})
	for _, event := range s.RuntimeEvents {
		p.RuntimeEvents = append(p.RuntimeEvents, fingerprintRuntimeEvent{
			Category:      event.Category,
			Operation:     event.Operation,
			Process:       event.Process,
			ParentProcess: event.ParentProcess,
			Target:        event.Target,
			Protocol:      event.Protocol,
			Direction:     event.Direction,
		})
	}
	sort.Slice(p.RuntimeEvents, func(i, j int) bool {
		return runtimeEventFingerprintKey(p.RuntimeEvents[i]) < runtimeEventFingerprintKey(p.RuntimeEvents[j])
	})
	for _, step := range s.ScenarioSteps {
		p.Steps = append(p.Steps, fingerprintStep{
			Name:         step.Name,
			Command:      append([]string(nil), step.Command...),
			ExitCode:     step.ExitCode,
			Success:      step.Success,
			AllowFailure: step.AllowFailure,
		})
	}

	data, _ := json.Marshal(p)
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func runtimeEventFingerprintKey(e fingerprintRuntimeEvent) string {
	return e.Category + "\x00" + e.Operation + "\x00" + e.Process + "\x00" +
		e.ParentProcess + "\x00" + e.Target + "\x00" + e.Protocol + "\x00" + e.Direction
}
