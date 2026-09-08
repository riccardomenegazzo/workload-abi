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

type fingerprintPayload struct {
	SchemaVersion string             `json:"schema_version"`
	Scenario      string             `json:"scenario,omitempty"`
	Processes     []Process          `json:"processes,omitempty"`
	Filesystem    []FilesystemChange `json:"filesystem,omitempty"`
	Listeners     []Listener         `json:"listeners,omitempty"`
	Steps         []fingerprintStep  `json:"steps,omitempty"`
	ImageConfig   ImageConfig        `json:"image_config"`
	Runtime       RuntimeFacts       `json:"runtime"`
}

func Fingerprint(s Snapshot) string {
	p := fingerprintPayload{
		SchemaVersion: SchemaVersion,
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
