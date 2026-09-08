package diff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

func Compare(base, candidate model.Snapshot) model.Comparison {
	c := model.Comparison{
		SchemaVersion:        model.SchemaVersion,
		Baseline:             base.Image,
		Candidate:            candidate.Image,
		BaselineFingerprint:  base.Fingerprint,
		CandidateFingerprint: candidate.Fingerprint,
		Verdict:              "COMPATIBLE",
	}

	compareSet(&c, "process", processSet(base.Processes), processSet(candidate.Processes), "warning")
	compareFS(&c, base.Filesystem, candidate.Filesystem)
	compareSet(&c, "port", stringSet(base.ImageConfig.ExposedPorts), stringSet(candidate.ImageConfig.ExposedPorts), "warning")
	compareSet(&c, "listener", listenerSet(base.Listeners), listenerSet(candidate.Listeners), "warning")
	compareScalar(&c, "network", "network-mode", base.Runtime.NetworkMode, candidate.Runtime.NetworkMode, "warning")
	compareSet(&c, "network", stringSet(base.Runtime.Networks), stringSet(candidate.Runtime.Networks), "warning")
	compareScalar(&c, "image-config", "user", base.ImageConfig.User, candidate.ImageConfig.User, "warning")
	compareScalar(&c, "image-config", "working-dir", base.ImageConfig.WorkingDir, candidate.ImageConfig.WorkingDir, "info")
	compareScalar(&c, "lifecycle", "stop-signal", base.ImageConfig.StopSignal, candidate.ImageConfig.StopSignal, "warning")
	compareSlice(&c, "image-config", "entrypoint", base.ImageConfig.Entrypoint, candidate.ImageConfig.Entrypoint, "warning")
	compareSlice(&c, "image-config", "cmd", base.ImageConfig.Cmd, candidate.ImageConfig.Cmd, "info")
	compareSlice(&c, "healthcheck", "test", base.ImageConfig.Healthcheck, candidate.ImageConfig.Healthcheck, "warning")
	compareScenarioSteps(&c, base.ScenarioSteps, candidate.ScenarioSteps)

	if !base.Runtime.Privileged && candidate.Runtime.Privileged {
		add(&c, model.Change{Surface: "privilege", Kind: "expanded", Before: "false", After: "true", Severity: "breaking", Message: "candidate requires privileged container execution"})
	}
	if base.Runtime.ReadonlyRootfs && !candidate.Runtime.ReadonlyRootfs {
		add(&c, model.Change{Surface: "filesystem", Kind: "expanded", Before: "read-only", After: "writable", Severity: "warning", Message: "candidate runtime root filesystem is writable"})
	}
	compareSet(&c, "capability", stringSet(base.Runtime.CapAdd), stringSet(candidate.Runtime.CapAdd), "breaking")

	if base.Lifecycle.StopDuration > 0 && candidate.Lifecycle.StopDuration > base.Lifecycle.StopDuration*2 && candidate.Lifecycle.StopDuration-base.Lifecycle.StopDuration > 500_000_000 {
		add(&c, model.Change{Surface: "lifecycle", Kind: "regression", Before: base.Lifecycle.StopDuration.String(), After: candidate.Lifecycle.StopDuration.String(), Severity: "warning", Message: "candidate shutdown duration increased materially"})
	}

	if base.Runtime.ExitCode == 0 && candidate.Runtime.ExitCode != 0 {
		add(&c, model.Change{Surface: "lifecycle", Kind: "regression", Before: "exit=0", After: fmt.Sprintf("exit=%d", candidate.Runtime.ExitCode), Severity: "breaking", Message: "candidate exits unsuccessfully under the same observation workflow"})
	}

	sortChanges(c.Changes)
	return c
}

func sortChanges(changes []model.Change) {
	sort.SliceStable(changes, func(i, j int) bool {
		if rank(changes[i].Severity) == rank(changes[j].Severity) {
			if changes[i].Surface == changes[j].Surface {
				return changes[i].Message < changes[j].Message
			}
			return changes[i].Surface < changes[j].Surface
		}
		return rank(changes[i].Severity) > rank(changes[j].Severity)
	})
}

func add(c *model.Comparison, ch model.Change) {
	c.Changes = append(c.Changes, ch)
	switch ch.Severity {
	case "breaking":
		c.Verdict = "BREAKING"
	case "warning", "info":
		if c.Verdict == "COMPATIBLE" {
			c.Verdict = "CHANGED"
		}
	}
}

func rank(s string) int {
	switch s {
	case "breaking":
		return 3
	case "warning":
		return 2
	default:
		return 1
	}
}

func processSet(ps []model.Process) map[string]struct{} {
	m := map[string]struct{}{}
	for _, p := range ps {
		if p.Command != "" {
			m[p.Command] = struct{}{}
		}
	}
	return m
}

func listenerSet(xs []model.Listener) map[string]struct{} {
	m := map[string]struct{}{}
	for _, x := range xs {
		if x.Port > 0 {
			m[fmt.Sprintf("%s/%d", x.Protocol, x.Port)] = struct{}{}
		}
	}
	return m
}

func stringSet(xs []string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, x := range xs {
		if x != "" {
			m[x] = struct{}{}
		}
	}
	return m
}

func compareSet(c *model.Comparison, surface string, before, after map[string]struct{}, addedSeverity string) {
	for x := range after {
		if _, ok := before[x]; !ok {
			add(c, model.Change{Surface: surface, Kind: "added", After: x, Severity: addedSeverity, Message: fmt.Sprintf("new %s observed: %s", surface, x)})
		}
	}
	for x := range before {
		if _, ok := after[x]; !ok {
			add(c, model.Change{Surface: surface, Kind: "removed", Before: x, Severity: "info", Message: fmt.Sprintf("%s no longer observed: %s", surface, x)})
		}
	}
}

func compareFS(c *model.Comparison, before, after []model.FilesystemChange) {
	b := map[string]string{}
	a := map[string]string{}
	for _, x := range before {
		b[x.Path] = x.Kind
	}
	for _, x := range after {
		a[x.Path] = x.Kind
	}
	for path, kind := range a {
		if old, ok := b[path]; !ok {
			add(c, model.Change{Surface: "filesystem", Kind: "added", After: kind + " " + path, Severity: "warning", Message: "new filesystem mutation observed: " + kind + " " + path})
		} else if old != kind {
			add(c, model.Change{Surface: "filesystem", Kind: "changed", Before: old + " " + path, After: kind + " " + path, Severity: "warning", Message: "filesystem mutation changed for " + path})
		}
	}
	for path, kind := range b {
		if _, ok := a[path]; !ok {
			add(c, model.Change{Surface: "filesystem", Kind: "removed", Before: kind + " " + path, Severity: "info", Message: "filesystem mutation no longer observed: " + kind + " " + path})
		}
	}
}

func compareScenarioSteps(c *model.Comparison, before, after []model.ScenarioStepResult) {
	count := len(before)
	if len(after) > count {
		count = len(after)
	}
	for i := 0; i < count; i++ {
		if i >= len(before) {
			add(c, model.Change{Surface: "scenario", Kind: "added", After: after[i].Name, Severity: "warning", Message: "candidate executed an additional scenario step: " + after[i].Name})
			continue
		}
		if i >= len(after) {
			add(c, model.Change{Surface: "scenario", Kind: "removed", Before: before[i].Name, Severity: "breaking", Message: "candidate did not execute scenario step: " + before[i].Name})
			continue
		}
		b, a := before[i], after[i]
		name := b.Name
		if name == "" {
			name = fmt.Sprintf("step-%d", i+1)
		}
		if b.Success && !a.Success && !a.AllowFailure {
			add(c, model.Change{
				Surface:  "scenario",
				Kind:     "regression",
				Before:   fmt.Sprintf("%s exit=%d", name, b.ExitCode),
				After:    fmt.Sprintf("%s exit=%d", a.Name, a.ExitCode),
				Severity: "breaking",
				Message:  "required scenario step regressed: " + name,
			})
		} else if b.ExitCode != a.ExitCode {
			add(c, model.Change{
				Surface:  "scenario",
				Kind:     "changed",
				Before:   fmt.Sprintf("%s exit=%d", name, b.ExitCode),
				After:    fmt.Sprintf("%s exit=%d", a.Name, a.ExitCode),
				Severity: "warning",
				Message:  "scenario step exit code changed: " + name,
			})
		}
	}
}

func compareScalar(c *model.Comparison, surface, key, before, after, severity string) {
	if before == after {
		return
	}
	add(c, model.Change{Surface: surface, Kind: "changed", Before: before, After: after, Severity: severity, Message: key + " changed"})
}

func compareSlice(c *model.Comparison, surface, key string, before, after []string, severity string) {
	if strings.Join(before, "\x00") == strings.Join(after, "\x00") {
		return
	}
	add(c, model.Change{Surface: surface, Kind: "changed", Before: strings.Join(before, " "), After: strings.Join(after, " "), Severity: severity, Message: key + " changed"})
}
