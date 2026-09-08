package model

import "time"

const SchemaVersion = "wabi.dev/v1alpha1"

type RuntimeGraph struct {
	SchemaVersion string            `json:"schema_version"`
	Image         ImageIdentity     `json:"image"`
	Scenario      string            `json:"scenario,omitempty"`
	CapturedAt    time.Time         `json:"captured_at"`
	Processes     []Process         `json:"processes,omitempty"`
	Network       []NetworkEndpoint `json:"network,omitempty"`
	Filesystem    []FileMutation    `json:"filesystem,omitempty"`
	Privileges    PrivilegeProfile  `json:"privileges"`
	Resources     ResourceProfile   `json:"resources"`
	Lifecycle     LifecycleProfile  `json:"lifecycle"`
	Warnings      []string          `json:"warnings,omitempty"`
}

type ImageIdentity struct {
	Reference string `json:"reference"`
	ID        string `json:"id,omitempty"`
	Digest    string `json:"digest,omitempty"`
	User      string `json:"user,omitempty"`
}

type Process struct {
	PID     int    `json:"pid,omitempty"`
	PPID    int    `json:"ppid,omitempty"`
	Name    string `json:"name"`
	Command string `json:"command,omitempty"`
}

type NetworkEndpoint struct {
	Protocol      string `json:"protocol"`
	Direction     string `json:"direction"` // listen, inbound, or outbound
	LocalAddress  string `json:"local_address,omitempty"`
	LocalPort     int    `json:"local_port,omitempty"`
	RemoteAddress string `json:"remote_address,omitempty"`
	RemotePort    int    `json:"remote_port,omitempty"`
	State         string `json:"state,omitempty"`
}

type FileMutation struct {
	Kind string `json:"kind"` // A, C, D from docker diff
	Path string `json:"path"`
}

type PrivilegeProfile struct {
	RuntimeUser      string   `json:"runtime_user,omitempty"`
	EffectiveUID     int      `json:"effective_uid,omitempty"`
	EffectiveGID     int      `json:"effective_gid,omitempty"`
	EffectiveCapsHex string   `json:"effective_caps_hex,omitempty"`
	EffectiveCaps    []string `json:"effective_caps,omitempty"`
	NoNewPrivileges bool     `json:"no_new_privileges"`
	SeccompMode      int      `json:"seccomp_mode,omitempty"`
	Privileged       bool     `json:"privileged"`
	ReadOnlyRootFS   bool     `json:"read_only_rootfs"`
	CapAdd           []string `json:"cap_add,omitempty"`
	CapDrop          []string `json:"cap_drop,omitempty"`
}

type ResourceProfile struct {
	PeakMemoryBytes int64   `json:"peak_memory_bytes,omitempty"`
	PeakCPUPercent  float64 `json:"peak_cpu_percent,omitempty"`
}

type LifecycleProfile struct {
	ReadyMillis    int64 `json:"ready_millis,omitempty"`
	ShutdownMillis int64 `json:"shutdown_millis,omitempty"`
	ExitCode       int   `json:"exit_code,omitempty"`
	OOMKilled      bool  `json:"oom_killed"`
}

type ChangeSeverity string

const (
	SeverityInfo     ChangeSeverity = "info"
	SeverityLow      ChangeSeverity = "low"
	SeverityMedium   ChangeSeverity = "medium"
	SeverityHigh     ChangeSeverity = "high"
	SeverityCritical ChangeSeverity = "critical"
)

type Change struct {
	Surface  string         `json:"surface"`
	Kind     string         `json:"kind"`
	Key      string         `json:"key"`
	Before   any            `json:"before,omitempty"`
	After    any            `json:"after,omitempty"`
	Severity ChangeSeverity `json:"severity"`
	Summary  string         `json:"summary"`
}

type DiffResult struct {
	SchemaVersion string        `json:"schema_version"`
	From          ImageIdentity `json:"from"`
	To            ImageIdentity `json:"to"`
	Scenario      string        `json:"scenario,omitempty"`
	Changes       []Change      `json:"changes"`
	Summary       DiffSummary   `json:"summary"`
}

type DiffSummary struct {
	Total    int `json:"total"`
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Info     int `json:"info"`
}

type CompatibilityStatus string

const (
	StatusCompatible CompatibilityStatus = "compatible"
	StatusDegraded   CompatibilityStatus = "degraded"
	StatusBreaking   CompatibilityStatus = "breaking"
	StatusUnknown    CompatibilityStatus = "unknown"
)

type Conflict struct {
	Surface     string         `json:"surface"`
	Constraint  string         `json:"constraint"`
	Observed    string         `json:"observed"`
	Severity    ChangeSeverity `json:"severity"`
	Explanation string         `json:"explanation"`
}

type CompatibilityResult struct {
	Status      CompatibilityStatus `json:"status"`
	Environment string              `json:"environment,omitempty"`
	Service     string              `json:"service,omitempty"`
	Conflicts   []Conflict          `json:"conflicts,omitempty"`
	Notes       []string            `json:"notes,omitempty"`
}

type CompareResult struct {
	SchemaVersion string               `json:"schema_version"`
	Diff          DiffResult           `json:"diff"`
	Compatibility *CompatibilityResult `json:"compatibility,omitempty"`
}
