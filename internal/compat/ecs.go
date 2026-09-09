package compat

import (
	"fmt"
	"strings"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
	"github.com/riccardomenegazzo/workload-abi/internal/target"
)

func ApplyECS(c model.Comparison, base, candidate model.Snapshot, t target.ECSTarget) model.Comparison {
	c.Target = t.Identity()

	if t.ReadOnlyRootfs {
		baseFS := fsMap(base.Filesystem)
		for _, ch := range candidate.Filesystem {
			if _, existed := baseFS[ch.Path]; existed {
				continue
			}
			if isWritablePath(ch.Path, t.WritablePaths) {
				continue
			}
			breaking(&c, "filesystem", "target-conflict", ch.Kind+" "+ch.Path,
				"candidate introduces a new filesystem mutation outside writable ECS mountPoints while readonlyRootFilesystem is enabled")
		}
	}

	if t.MemoryHardLimit > 0 {
		usage := statsMemoryUsage(candidate.Stats.MemUsage)
		if usage > t.MemoryHardLimit {
			breaking(&c, "resource", "target-conflict", fmt.Sprintf("%d bytes", usage),
				fmt.Sprintf("observed memory usage exceeds ECS hard memory limit (%d bytes)", t.MemoryHardLimit))
		}
	}

	if t.StopTimeout > 0 && candidate.Lifecycle.StopDuration > t.StopTimeout {
		breaking(&c, "lifecycle", "target-conflict", candidate.Lifecycle.StopDuration.String(),
			"observed shutdown duration exceeds explicit ECS stopTimeout")
	}

	if dropsAll(t.CapDrop) && len(candidate.Runtime.CapAdd) > 0 {
		breaking(&c, "privilege", "target-conflict", strings.Join(candidate.Runtime.CapAdd, ","),
			"candidate container configuration adds capabilities while ECS drops ALL capabilities")
	}

	return c
}
