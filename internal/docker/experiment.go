package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

type ExperimentStep struct {
	Name         string
	After        time.Duration
	Command      []string
	Timeout      time.Duration
	AllowFailure bool
}

type ExperimentOptions struct {
	Observe     time.Duration
	Environment []string
	Command     []string
	Steps       []ExperimentStep
}

func (r *Recorder) CollectExperiment(ctx context.Context, image string, opts ExperimentOptions) (model.Snapshot, error) {
	snap := model.Snapshot{
		SchemaVersion: model.SchemaVersion,
		Image:         image,
		CapturedAt:    time.Now().UTC(),
	}
	if err := r.Pull(ctx, image); err != nil {
		return snap, err
	}
	if err := r.inspectImage(ctx, image, &snap); err != nil {
		return snap, fmt.Errorf("inspect image: %w", err)
	}

	name := fmt.Sprintf("wabi-%d", time.Now().UnixNano())
	args := []string{"create", "--name", name}
	for _, env := range opts.Environment {
		args = append(args, "--env", env)
	}
	args = append(args, image)
	args = append(args, opts.Command...)

	out, err := exec.CommandContext(ctx, r.binary, args...).CombinedOutput()
	if err != nil {
		return snap, fmt.Errorf("create container: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	id := strings.TrimSpace(string(out))
	defer exec.Command(r.binary, "rm", "-f", id).Run() //nolint:errcheck

	if err := r.inspectContainer(ctx, id, &snap); err != nil {
		return snap, err
	}
	if err := r.inspectNetwork(ctx, id, &snap); err != nil {
		snap.Warnings = append(snap.Warnings, "network metadata unavailable: "+err.Error())
	}

	experimentStarted := time.Now()
	started := time.Now()
	if out, err := exec.CommandContext(ctx, r.binary, "start", id).CombinedOutput(); err != nil {
		return snap, fmt.Errorf("start container: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	snap.Lifecycle.StartDuration = time.Since(started)

	for _, step := range opts.Steps {
		if step.After > 0 {
			if err := waitExperiment(ctx, step.After); err != nil {
				return snap, err
			}
		}
		result := r.runExperimentStep(ctx, id, step)
		snap.ScenarioSteps = append(snap.ScenarioSteps, result)
		if !result.Success && !result.AllowFailure {
			snap.Warnings = append(snap.Warnings, fmt.Sprintf(
				"required scenario step %q failed with exit code %d", result.Name, result.ExitCode))
		}
	}

	if remaining := opts.Observe - time.Since(experimentStarted); remaining > 0 {
		if err := waitExperiment(ctx, remaining); err != nil {
			return snap, err
		}
	}

	if processes, err := r.top(ctx, id); err == nil {
		snap.Processes = processes
	} else {
		snap.Warnings = append(snap.Warnings, "process snapshot unavailable: "+err.Error())
	}
	if changes, err := r.filesystem(ctx, id); err == nil {
		snap.Filesystem = changes
	} else {
		snap.Warnings = append(snap.Warnings, "filesystem diff unavailable: "+err.Error())
	}
	if listeners, err := r.listeners(ctx, id); err == nil {
		snap.Listeners = listeners
	} else {
		snap.Warnings = append(snap.Warnings, "listener snapshot unavailable: "+err.Error())
	}
	if stats, err := r.stats(ctx, id); err == nil {
		snap.Stats = stats
	} else {
		snap.Warnings = append(snap.Warnings, "runtime stats unavailable: "+err.Error())
	}

	if err := r.inspectContainer(ctx, id, &snap); err != nil {
		snap.Warnings = append(snap.Warnings, "final inspect unavailable: "+err.Error())
	}
	if err := r.inspectNetwork(ctx, id, &snap); err != nil {
		snap.Warnings = append(snap.Warnings, "final network metadata unavailable: "+err.Error())
	}

	if snap.Runtime.ContainerStatus == "running" {
		stopped := time.Now()
		if out, err := exec.CommandContext(ctx, r.binary, "stop", "--time", "2", id).CombinedOutput(); err != nil {
			snap.Warnings = append(snap.Warnings, "graceful stop failed: "+strings.TrimSpace(string(out)))
		} else {
			snap.Lifecycle.StopDuration = time.Since(stopped)
		}
	}
	return snap, nil
}

func waitExperiment(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (r *Recorder) runExperimentStep(ctx context.Context, id string, step ExperimentStep) model.ScenarioStepResult {
	result := model.ScenarioStepResult{
		Name:         step.Name,
		Command:      append([]string(nil), step.Command...),
		ExitCode:     -1,
		AllowFailure: step.AllowFailure,
	}
	timeout := step.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	stepCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := append([]string{"exec", id}, step.Command...)
	started := time.Now()
	out, err := exec.CommandContext(stepCtx, r.binary, args...).CombinedOutput()
	result.Duration = time.Since(started)
	result.Output = truncateExperiment(strings.TrimSpace(string(out)), 2048)
	if err == nil {
		result.ExitCode = 0
		result.Success = true
		return result
	}

	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		result.ExitCode = exitErr.ExitCode()
		result.Error = "command exited non-zero"
	case errors.Is(stepCtx.Err(), context.DeadlineExceeded):
		result.Error = "step timed out after " + timeout.String()
	default:
		result.Error = err.Error()
	}
	return result
}

func truncateExperiment(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (r *Recorder) inspectNetwork(ctx context.Context, id string, snap *model.Snapshot) error {
	var payload []struct {
		HostConfig struct {
			NetworkMode string `json:"NetworkMode"`
		} `json:"HostConfig"`
		NetworkSettings struct {
			Networks map[string]json.RawMessage `json:"Networks"`
		} `json:"NetworkSettings"`
	}
	out, err := exec.CommandContext(ctx, r.binary, "inspect", id).Output()
	if err != nil {
		return err
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return err
	}
	if len(payload) != 1 {
		return fmt.Errorf("unexpected network inspect payload length: %d", len(payload))
	}
	snap.Runtime.NetworkMode = payload[0].HostConfig.NetworkMode
	snap.Runtime.Networks = snap.Runtime.Networks[:0]
	for name := range payload[0].NetworkSettings.Networks {
		snap.Runtime.Networks = append(snap.Runtime.Networks, name)
	}
	sort.Strings(snap.Runtime.Networks)
	return nil
}

func (r *Recorder) listeners(ctx context.Context, id string) ([]model.Listener, error) {
	var all []model.Listener
	var failures int
	for _, item := range []struct {
		protocol string
		path     string
	}{
		{"tcp", "/proc/net/tcp"},
		{"tcp6", "/proc/net/tcp6"},
	} {
		out, err := exec.CommandContext(ctx, r.binary, "exec", id, "cat", item.path).CombinedOutput()
		if err != nil {
			failures++
			continue
		}
		all = append(all, parseTCPListeners(string(out), item.protocol)...)
	}
	if failures == 2 {
		return nil, fmt.Errorf("container cannot expose /proc/net/tcp through cat")
	}

	seen := map[string]model.Listener{}
	for _, listener := range all {
		seen[fmt.Sprintf("%s/%d", listener.Protocol, listener.Port)] = listener
	}
	all = all[:0]
	for _, listener := range seen {
		all = append(all, listener)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Protocol == all[j].Protocol {
			return all[i].Port < all[j].Port
		}
		return all[i].Protocol < all[j].Protocol
	})
	return all, nil
}

func parseTCPListeners(raw, protocol string) []model.Listener {
	var out []model.Listener
	lines := strings.Split(raw, "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[3] != "0A" {
			continue
		}
		addr := strings.SplitN(fields[1], ":", 2)
		if len(addr) != 2 {
			continue
		}
		port, err := strconv.ParseInt(addr[1], 16, 32)
		if err != nil || port <= 0 {
			continue
		}
		out = append(out, model.Listener{Protocol: protocol, Port: int(port)})
	}
	return out
}
