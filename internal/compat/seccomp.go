package compat

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
	"github.com/riccardomenegazzo/workload-abi/internal/target"
)

func ApplySeccomp(c model.Comparison, base, candidate model.Snapshot, profile target.SeccompProfile) model.Comparison {
	ref := "seccomp:" + filepath.Base(profile.File)
	if c.Target == "" {
		c.Target = ref
	} else if !strings.Contains(c.Target, ref) {
		c.Target += "+" + ref
	}

	baseSyscalls := exactSyscallSet(base.RuntimeEvents)
	candidateSyscalls := exactSyscallSet(candidate.RuntimeEvents)
	var added []string
	for syscall := range candidateSyscalls {
		if _, existed := baseSyscalls[syscall]; !existed {
			added = append(added, syscall)
		}
	}
	sort.Strings(added)

	for _, syscall := range added {
		decision, action := profile.Decision(syscall)
		if decision != target.SeccompDeny {
			continue
		}
		breaking(&c, "syscall", "target-conflict", syscall,
			fmt.Sprintf("candidate introduces syscall %s, but seccomp profile %s resolves it to %s", syscall, filepath.Base(profile.File), action))
	}
	return c
}

func exactSyscallSet(events []model.RuntimeEvent) map[string]struct{} {
	out := map[string]struct{}{}
	for _, event := range events {
		if !strings.EqualFold(strings.TrimSpace(event.Category), "syscall") {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(event.Operation))
		if name != "" {
			out[name] = struct{}{}
		}
	}
	return out
}
