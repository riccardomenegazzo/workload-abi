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

type Recorder struct {
	binary string
}

type Options struct {
	Observe     time.Duration
	Environment []string
	Command     []string
}

func NewRecorder() *Recorder { return &Recorder{binary: "docker"} }

func (r *Recorder) Check(ctx context.Context) error {
	if _, err := exec.LookPath(r.binary); err != nil {
		return errors.New("docker CLI not found in PATH")
	}
	cmd := exec.CommandContext(ctx, r.binary, "info", "--format", "{{.ServerVersion}}")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker daemon unavailable: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (r *Recorder) Pull(ctx context.Context, image string) error {
	cmd := exec.CommandContext(ctx, r.binary, "image", "inspect", image)
	if err := cmd.Run(); err == nil {
		return nil
	}
	cmd = exec.CommandContext(ctx, r.binary, "pull", image)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pull %s: %w (%s)", image, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (r *Recorder) Collect(ctx context.Context, image string, observe time.Duration) (model.Snapshot, error) {
	return r.CollectWithOptions(ctx, image, Options{Observe: observe})
}

func (r *Recorder) CollectWithOptions(ctx context.Context, image string, opts Options) (model.Snapshot, error) {
	var snap model.Snapshot
	snap.Image = image
	snap.CapturedAt = time.Now().UTC()

	if err := r.Pull(ctx, image); err != nil {
		return snap, err
	}
	if err := r.inspectImage(ctx, image, &snap); err != nil {
		return snap, fmt.Errorf("inspect image: %w", err)
	}

	name := fmt.Sprintf("wabi-%d", time.Now().UnixNano())
	createArgs := []string{"create", "--name", name}
	for _, env := range opts.Environment {
		createArgs = append(createArgs, "--env", env)
	}
	createArgs = append(createArgs, image)
	createArgs = append(createArgs, opts.Command...)

	create := exec.CommandContext(ctx, r.binary, createArgs...)
	out, err := create.CombinedOutput()
	if err != nil {
		return snap, fmt.Errorf("create container: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	containerID := strings.TrimSpace(string(out))
	defer exec.Command(r.binary, "rm", "-f", containerID).Run() //nolint:errcheck

	if err := r.inspectContainer(ctx, containerID, &snap); err != nil {
		return snap, err
	}

	started := time.Now()
	start := exec.CommandContext(ctx, r.binary, "start", containerID)
	if out, err := start.CombinedOutput(); err != nil {
		return snap, fmt.Errorf("start container: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	snap.Lifecycle.StartDuration = time.Since(started)

	if opts.Observe > 0 {
		t := time.NewTimer(opts.Observe)
		select {
		case <-ctx.Done():
			t.Stop()
			return snap, ctx.Err()
		case <-t.C:
		}
	}

	if processes, err := r.top(ctx, containerID); err == nil {
		snap.Processes = processes
	} else {
		snap.Warnings = append(snap.Warnings, "process snapshot unavailable: "+err.Error())
	}

	if changes, err := r.filesystem(ctx, containerID); err == nil {
		snap.Filesystem = changes
	} else {
		snap.Warnings = append(snap.Warnings, "filesystem diff unavailable: "+err.Error())
	}

	if stats, err := r.stats(ctx, containerID); err == nil {
		snap.Stats = stats
	} else {
		snap.Warnings = append(snap.Warnings, "runtime stats unavailable: "+err.Error())
	}

	if err := r.inspectContainer(ctx, containerID, &snap); err != nil {
		snap.Warnings = append(snap.Warnings, "final inspect unavailable: "+err.Error())
	}

	if snap.Runtime.ContainerStatus == "running" {
		stopped := time.Now()
		cmd := exec.CommandContext(ctx, r.binary, "stop", "--time", "2", containerID)
		if out, err := cmd.CombinedOutput(); err != nil {
			snap.Warnings = append(snap.Warnings, "graceful stop failed: "+strings.TrimSpace(string(out)))
		} else {
			snap.Lifecycle.StopDuration = time.Since(stopped)
		}
	}

	return snap, nil
}

type imageInspectPayload struct {
	ID     string `json:"Id"`
	Config struct {
		User         string         `json:"User"`
		Entrypoint   []string       `json:"Entrypoint"`
		Cmd          []string       `json:"Cmd"`
		WorkingDir   string         `json:"WorkingDir"`
		StopSignal   string         `json:"StopSignal"`
		ExposedPorts map[string]any `json:"ExposedPorts"`
		Healthcheck  *struct {
			Test []string `json:"Test"`
		} `json:"Healthcheck"`
	} `json:"Config"`
}

type containerInspectPayload struct {
	HostConfig struct {
		Privileged     bool     `json:"Privileged"`
		ReadonlyRootfs bool     `json:"ReadonlyRootfs"`
		CapAdd         []string `json:"CapAdd"`
		CapDrop        []string `json:"CapDrop"`
		Memory         int64    `json:"Memory"`
		NanoCPUs       int64    `json:"NanoCpus"`
		PidsLimit      *int64   `json:"PidsLimit"`
	} `json:"HostConfig"`
	State struct {
		Status   string `json:"Status"`
		ExitCode int    `json:"ExitCode"`
	} `json:"State"`
}

func (r *Recorder) inspectImage(ctx context.Context, image string, snap *model.Snapshot) error {
	cmd := exec.CommandContext(ctx, r.binary, "image", "inspect", image)
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	var payload []imageInspectPayload
	if err := json.Unmarshal(out, &payload); err != nil {
		return err
	}
	if len(payload) != 1 {
		return fmt.Errorf("unexpected image inspect payload length: %d", len(payload))
	}
	p := payload[0]
	snap.ImageID = p.ID
	snap.ImageConfig.User = p.Config.User
	snap.ImageConfig.Entrypoint = p.Config.Entrypoint
	snap.ImageConfig.Cmd = p.Config.Cmd
	snap.ImageConfig.WorkingDir = p.Config.WorkingDir
	snap.ImageConfig.StopSignal = p.Config.StopSignal
	snap.ImageConfig.ExposedPorts = snap.ImageConfig.ExposedPorts[:0]
	for port := range p.Config.ExposedPorts {
		snap.ImageConfig.ExposedPorts = append(snap.ImageConfig.ExposedPorts, port)
	}
	sort.Strings(snap.ImageConfig.ExposedPorts)
	if p.Config.Healthcheck != nil {
		snap.ImageConfig.Healthcheck = append([]string(nil), p.Config.Healthcheck.Test...)
	}
	return nil
}

func (r *Recorder) inspectContainer(ctx context.Context, id string, snap *model.Snapshot) error {
	cmd := exec.CommandContext(ctx, r.binary, "inspect", id)
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	var payload []containerInspectPayload
	if err := json.Unmarshal(out, &payload); err != nil {
		return err
	}
	if len(payload) != 1 {
		return fmt.Errorf("unexpected container inspect payload length: %d", len(payload))
	}
	p := payload[0]
	snap.Runtime.Privileged = p.HostConfig.Privileged
	snap.Runtime.ReadonlyRootfs = p.HostConfig.ReadonlyRootfs
	snap.Runtime.CapAdd = append([]string(nil), p.HostConfig.CapAdd...)
	snap.Runtime.CapDrop = append([]string(nil), p.HostConfig.CapDrop...)
	snap.Runtime.MemoryLimit = p.HostConfig.Memory
	snap.Runtime.NanoCPUs = p.HostConfig.NanoCPUs
	if p.HostConfig.PidsLimit != nil {
		snap.Runtime.PidsLimit = *p.HostConfig.PidsLimit
	}
	snap.Runtime.ContainerStatus = p.State.Status
	snap.Runtime.ExitCode = p.State.ExitCode
	return nil
}

func (r *Recorder) top(ctx context.Context, id string) ([]model.Process, error) {
	cmd := exec.CommandContext(ctx, r.binary, "top", id, "-eo", "pid,ppid,user,comm,args")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker top: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return nil, nil
	}
	seen := map[string]struct{}{}
	var result []model.Process
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		command := strings.Join(fields[3:], " ")
		if _, ok := seen[command]; ok {
			continue
		}
		seen[command] = struct{}{}
		result = append(result, model.Process{Command: command})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Command < result[j].Command })
	return result, nil
}

func (r *Recorder) filesystem(ctx context.Context, id string) ([]model.FilesystemChange, error) {
	cmd := exec.CommandContext(ctx, r.binary, "diff", id)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker diff: %w", err)
	}
	var result []model.FilesystemChange
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		result = append(result, model.FilesystemChange{Kind: parts[0], Path: parts[1]})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path == result[j].Path {
			return result[i].Kind < result[j].Kind
		}
		return result[i].Path < result[j].Path
	})
	return result, nil
}

func (r *Recorder) stats(ctx context.Context, id string) (model.Stats, error) {
	var stats model.Stats
	cmd := exec.CommandContext(ctx, r.binary, "stats", "--no-stream", "--format", "{{json .}}", id)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return stats, err
	}
	var raw map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &raw); err != nil {
		return stats, err
	}
	stats.CPUPercent = raw["CPUPerc"]
	stats.MemUsage = raw["MemUsage"]
	stats.NetIO = raw["NetIO"]
	stats.BlockIO = raw["BlockIO"]
	stats.PIDs = raw["PIDs"]
	return stats, nil
}

func ParseBytes(s string) int64 {
	s = strings.TrimSpace(strings.ToLower(s))
	units := []struct {
		suffix string
		mul    float64
	}{
		{"gib", 1024 * 1024 * 1024}, {"gb", 1000 * 1000 * 1000},
		{"mib", 1024 * 1024}, {"mb", 1000 * 1000},
		{"kib", 1024}, {"kb", 1000}, {"b", 1},
	}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, u.suffix)), 64)
			if err == nil {
				return int64(v * u.mul)
			}
		}
	}
	return 0
}
