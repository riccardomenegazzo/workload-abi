package native

import (
	"context"
	"errors"
	"time"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

var ErrUnsupported = errors.New("native Linux recorder is not supported on this platform")

type Config struct {
	CgroupPath string
	PID        int
	Duration   time.Duration
	RingBytes  uint32
}

type Result struct {
	Backend   string               `json:"backend"`
	StartedAt time.Time            `json:"started_at"`
	EndedAt   time.Time            `json:"ended_at"`
	Cgroup    string               `json:"cgroup"`
	Events    []model.RuntimeEvent `json:"events"`
	Dropped   uint64               `json:"dropped"`
	Warnings  []string             `json:"warnings,omitempty"`
}

func Capture(ctx context.Context, cfg Config) (Result, error) {
	return capture(ctx, cfg)
}
