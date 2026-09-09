package nativeebpf

import (
	"context"
	"time"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

// Options bounds one native eBPF collection session.
type Options struct {
	Duration  time.Duration
	MaxEvents int
}

// ProbeStatus records whether a native tracepoint was attached.
type ProbeStatus struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Error     string `json:"error,omitempty"`
}

// Stats contains collection diagnostics that are intentionally not part of
// RuntimeEvent semantic identity.
type Stats struct {
	Captured    int           `json:"captured"`
	LostSamples uint64        `json:"lost_samples"`
	Duration    time.Duration `json:"duration"`
	Probes      []ProbeStatus `json:"probes"`
}

// Result is the native provider output before the CLI emits the interoperable
// RuntimeEvent array and optional diagnostics.
type Result struct {
	Events []model.RuntimeEvent `json:"events"`
	Stats  Stats                `json:"stats"`
}

// Record captures native Linux runtime evidence. The non-Linux implementation
// returns a clear unsupported-platform error while preserving cross-platform builds.
func Record(ctx context.Context, opts Options) (Result, error) {
	return record(ctx, opts)
}
