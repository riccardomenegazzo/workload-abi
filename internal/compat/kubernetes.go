package compat

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
	"github.com/riccardomenegazzo/workload-abi/internal/target"
)

func ApplyKubernetes(c model.Comparison, base, candidate model.Snapshot, t target.KubernetesTarget) model.Comparison {
	c.Target = fmt.Sprintf("kubernetes:%s/%s#%s", t.Kind, t.Name, t.Container)
	if t.NetworkPolicyFile != "" {
		c.Target += "+networkpolicy:" + filepath.Base(t.NetworkPolicyFile)
	}

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
				"candidate introduces a new filesystem mutation outside writable Kubernetes volume mounts while readOnlyRootFilesystem is enabled")
		}
	}

	if t.MemoryLimit > 0 {
		usage := statsMemoryUsage(candidate.Stats.MemUsage)
		if usage > t.MemoryLimit {
			breaking(&c, "resource", "target-conflict", fmt.Sprintf("%d bytes", usage),
				fmt.Sprintf("observed memory usage exceeds Kubernetes memory limit (%d bytes)", t.MemoryLimit))
		}
	}

	if t.StopGracePeriod > 0 && candidate.Lifecycle.StopDuration > t.StopGracePeriod {
		breaking(&c, "lifecycle", "target-conflict", candidate.Lifecycle.StopDuration.String(),
			"observed shutdown duration exceeds Kubernetes terminationGracePeriodSeconds")
	}

	if t.RunAsNonRoot && t.RunAsUser == nil {
		user := strings.TrimSpace(candidate.ImageConfig.User)
		if user == "" || user == "0" || strings.EqualFold(user, "root") {
			breaking(&c, "identity", "target-conflict", displayUser(user),
				"candidate image user conflicts with Kubernetes runAsNonRoot")
		} else if _, err := strconv.ParseInt(user, 10, 64); err != nil {
			breaking(&c, "identity", "target-conflict", user,
				"candidate image uses a non-numeric user that Kubernetes cannot verify for runAsNonRoot without runAsUser")
		}
	}

	if dropsAll(t.CapDrop) && len(candidate.Runtime.CapAdd) > 0 {
		breaking(&c, "privilege", "target-conflict", strings.Join(candidate.Runtime.CapAdd, ","),
			"candidate runtime adds capabilities while Kubernetes drops ALL capabilities")
	}

	if t.AllowPrivilegeEscalation != nil && !*t.AllowPrivilegeEscalation && candidate.Runtime.Privileged {
		breaking(&c, "privilege", "target-conflict", "privileged=true",
			"candidate runtime requires privileged execution while Kubernetes disallows privilege escalation")
	}

	applyKubernetesNetworkPolicies(&c, base, candidate, t)
	return c
}

func displayUser(user string) string {
	if user == "" {
		return "<image-default-root>"
	}
	return user
}
