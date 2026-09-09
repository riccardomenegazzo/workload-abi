package model

import "time"

const (
	SchemaVersionV1Alpha2 = "wabi.dev/v1alpha2"
	SchemaVersion         = "wabi.dev/v1alpha3"
)

func IsSupportedSchema(version string) bool {
	switch version {
	case "", SchemaVersionV1Alpha2, SchemaVersion:
		return true
	default:
		return false
	}
}

type Snapshot struct {
	SchemaVersion string               `json:"schema_version"`
	Image         string               `json:"image"`
	ImageID       string               `json:"image_id,omitempty"`
	Scenario      string               `json:"scenario,omitempty"`
	Fingerprint   string               `json:"fingerprint,omitempty"`
	CapturedAt    time.Time            `json:"captured_at"`
	Processes     []Process            `json:"processes,omitempty"`
	Filesystem    []FilesystemChange   `json:"filesystem,omitempty"`
	Listeners     []Listener           `json:"listeners,omitempty"`
	RuntimeEvents []RuntimeEvent       `json:"runtime_events,omitempty"`
	ScenarioSteps []ScenarioStepResult `json:"scenario_steps,omitempty"`
	ImageConfig   ImageConfig          `json:"image_config"`
	Runtime       RuntimeFacts         `json:"runtime"`
	Lifecycle     Lifecycle            `json:"lifecycle"`
	Stats         Stats                `json:"stats,omitempty"`
	Warnings      []string             `json:"warnings,omitempty"`
}

type Process struct {
	Command string `json:"command"`
}

type FilesystemChange struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type Listener struct {
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
}

// RuntimeEvent is provider-neutral deep runtime evidence. Fields such as Source,
// PID and Rule preserve diagnostics but are deliberately excluded from the
// semantic identity used by compatibility diffing and operational fingerprints.
type RuntimeEvent struct {
	Source        string `json:"source,omitempty"`
	Category      string `json:"category"`
	Operation     string `json:"operation"`
	Process       string `json:"process,omitempty"`
	ParentProcess string `json:"parent_process,omitempty"`
	Target        string `json:"target,omitempty"`
	Protocol      string `json:"protocol,omitempty"`
	Direction     string `json:"direction,omitempty"`
	PID           int    `json:"pid,omitempty"`
	Rule          string `json:"rule,omitempty"`
}

func (e RuntimeEvent) SemanticKey() string {
	return e.Category + "\x00" + e.Operation + "\x00" + e.Process + "\x00" +
		e.ParentProcess + "\x00" + e.Target + "\x00" + e.Protocol + "\x00" + e.Direction
}

type ScenarioStepResult struct {
	Name         string        `json:"name"`
	Command      []string      `json:"command"`
	ExitCode     int           `json:"exit_code"`
	Success      bool          `json:"success"`
	AllowFailure bool          `json:"allow_failure,omitempty"`
	Duration     time.Duration `json:"duration"`
	Output       string        `json:"output,omitempty"`
	Error        string        `json:"error,omitempty"`
}

type ImageConfig struct {
	User         string   `json:"user,omitempty"`
	Entrypoint   []string `json:"entrypoint,omitempty"`
	Cmd          []string `json:"cmd,omitempty"`
	WorkingDir   string   `json:"working_dir,omitempty"`
	StopSignal   string   `json:"stop_signal,omitempty"`
	ExposedPorts []string `json:"exposed_ports,omitempty"`
	Healthcheck  []string `json:"healthcheck,omitempty"`
}

type RuntimeFacts struct {
	Privileged      bool     `json:"privileged"`
	ReadonlyRootfs  bool     `json:"readonly_rootfs"`
	CapAdd          []string `json:"cap_add,omitempty"`
	CapDrop         []string `json:"cap_drop,omitempty"`
	MemoryLimit     int64    `json:"memory_limit_bytes,omitempty"`
	NanoCPUs        int64    `json:"nano_cpus,omitempty"`
	PidsLimit       int64    `json:"pids_limit,omitempty"`
	ContainerStatus string   `json:"container_status,omitempty"`
	ExitCode        int      `json:"exit_code,omitempty"`
	NetworkMode     string   `json:"network_mode,omitempty"`
	Networks        []string `json:"networks,omitempty"`
}

type Lifecycle struct {
	StartDuration time.Duration `json:"start_duration"`
	StopDuration  time.Duration `json:"stop_duration"`
}

type Stats struct {
	CPUPercent string `json:"cpu_percent,omitempty"`
	MemUsage   string `json:"mem_usage,omitempty"`
	NetIO      string `json:"net_io,omitempty"`
	BlockIO    string `json:"block_io,omitempty"`
	PIDs       string `json:"pids,omitempty"`
}

type Change struct {
	Surface  string `json:"surface"`
	Kind     string `json:"kind"`
	Before   string `json:"before,omitempty"`
	After    string `json:"after,omitempty"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type Comparison struct {
	SchemaVersion        string   `json:"schema_version"`
	Baseline             string   `json:"baseline"`
	Candidate            string   `json:"candidate"`
	BaselineFingerprint  string   `json:"baseline_fingerprint,omitempty"`
	CandidateFingerprint string   `json:"candidate_fingerprint,omitempty"`
	Scenario             string   `json:"scenario,omitempty"`
	Target               string   `json:"target,omitempty"`
	Policy               string   `json:"policy,omitempty"`
	Verdict              string   `json:"verdict"`
	Changes              []Change `json:"changes"`
}
