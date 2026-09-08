package model

import "time"

type Snapshot struct {
	Image       string             `json:"image"`
	ImageID     string             `json:"image_id,omitempty"`
	CapturedAt  time.Time          `json:"captured_at"`
	Processes   []Process          `json:"processes,omitempty"`
	Filesystem  []FilesystemChange `json:"filesystem,omitempty"`
	ImageConfig ImageConfig        `json:"image_config"`
	Runtime     RuntimeFacts       `json:"runtime"`
	Lifecycle   Lifecycle          `json:"lifecycle"`
	Stats       Stats              `json:"stats,omitempty"`
	Warnings    []string           `json:"warnings,omitempty"`
}

type Process struct {
	Command string `json:"command"`
}

type FilesystemChange struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
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
	Baseline  string   `json:"baseline"`
	Candidate string   `json:"candidate"`
	Target    string   `json:"target,omitempty"`
	Verdict   string   `json:"verdict"`
	Changes   []Change `json:"changes"`
}
