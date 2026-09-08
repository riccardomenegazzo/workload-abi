package compat

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/riccardomenegazzo/workload-abi/internal/docker"
	"github.com/riccardomenegazzo/workload-abi/internal/model"
	"github.com/riccardomenegazzo/workload-abi/internal/target"
)

func ApplyCompose(c model.Comparison, base, candidate model.Snapshot, t target.ComposeTarget) model.Comparison {
	c.Target = fmt.Sprintf("compose:%s#%s", filepath.Base(t.File), t.Service)

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
				"candidate introduces a new filesystem mutation outside writable Compose mounts while read_only is enabled")
		}
	}

	if t.MemoryLimit > 0 {
		usage := statsMemoryUsage(candidate.Stats.MemUsage)
		if usage > t.MemoryLimit {
			breaking(&c, "resource", "target-conflict", fmt.Sprintf("%d bytes", usage),
				fmt.Sprintf("observed memory usage exceeds Compose memory limit (%d bytes)", t.MemoryLimit))
		}
	}

	if t.StopGracePeriod > 0 && candidate.Lifecycle.StopDuration > t.StopGracePeriod {
		breaking(&c, "lifecycle", "target-conflict", candidate.Lifecycle.StopDuration.String(),
			"observed shutdown duration exceeds Compose stop_grace_period")
	}

	if dropsAll(t.CapDrop) && len(candidate.Runtime.CapAdd) > 0 {
		breaking(&c, "privilege", "target-conflict", strings.Join(candidate.Runtime.CapAdd, ","),
			"candidate container configuration adds capabilities while Compose drops ALL capabilities")
	}

	return c
}

func breaking(c *model.Comparison, surface, kind, after, message string) {
	c.Changes = append(c.Changes, model.Change{
		Surface:  surface,
		Kind:     kind,
		After:    after,
		Severity: "breaking",
		Message:  message,
	})
	c.Verdict = "BREAKING"
}

func fsMap(xs []model.FilesystemChange) map[string]struct{} {
	m := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		m[x.Path] = struct{}{}
	}
	return m
}

func isWritablePath(path string, roots []string) bool {
	for _, root := range roots {
		if root == "/" || path == root || strings.HasPrefix(path, root+"/") {
			return true
		}
	}
	return false
}

func dropsAll(xs []string) bool {
	for _, x := range xs {
		if strings.EqualFold(x, "ALL") {
			return true
		}
	}
	return false
}

func statsMemoryUsage(s string) int64 {
	if s == "" {
		return 0
	}
	left := strings.TrimSpace(strings.SplitN(s, "/", 2)[0])
	return docker.ParseBytes(left)
}
