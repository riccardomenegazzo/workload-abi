package observe

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/riccardomenegazzo/workload-abi/internal/dockercli"
	"github.com/riccardomenegazzo/workload-abi/internal/model"
	"github.com/riccardomenegazzo/workload-abi/internal/scenario"
)

type Recorder struct {
	Docker *dockercli.Client
	Out    io.Writer
}

func New(d *dockercli.Client, out io.Writer) *Recorder { return &Recorder{Docker: d, Out: out} }

func (r *Recorder) Record(ctx context.Context, image string, sc scenario.Scenario) (graph model.RuntimeGraph, err error) {
	if err := r.Docker.Available(ctx); err != nil {
		return graph, fmt.Errorf("Docker daemon is required: %w", err)
	}
	if err := r.Docker.EnsureImage(ctx, image); err != nil {
		return graph, err
	}
	id, digest, _ := r.Docker.ImageDigest(ctx, image)
	graph = model.RuntimeGraph{SchemaVersion: model.SchemaVersion, Image: model.ImageIdentity{Reference: image, ID: id, Digest: digest}, Scenario: sc.Name, CapturedAt: time.Now().UTC()}

	name := fmt.Sprintf("wabi-%d", time.Now().UnixNano())
	args := []string{"run", "-d", "--name", name, "--label", "io.wabi.run=true"}
	for k, v := range sc.Environment {
		args = append(args, "-e", k+"="+v)
	}
	ports := uniquePorts(sc)
	for _, p := range ports {
		args = append(args, "-p", fmt.Sprintf("127.0.0.1::%d", p))
	}
	args = append(args, image)
	args = append(args, sc.Command...)

	started := time.Now()
	containerID, err := r.Docker.Run(ctx, args...)
	if err != nil {
		return graph, fmt.Errorf("start workload: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = r.Docker.Run(cleanupCtx, "rm", "-f", containerID)
	}()
	if r.Out != nil {
		fmt.Fprintf(r.Out, "recording %s as %s\n", image, shortID(containerID))
	}

	if sc.SettleMillis > 0 {
		select {
		case <-ctx.Done():
			return graph, ctx.Err()
		case <-time.After(sc.SettleDuration()):
		}
	}

	ins, inspectErr := r.Docker.Inspect(ctx, containerID)
	if inspectErr != nil {
		return graph, inspectErr
	}
	graph.Image.User = ins.Config.User
	graph.Privileges.RuntimeUser = ins.Config.User
	graph.Privileges.Privileged = ins.HostConfig.Privileged
	graph.Privileges.ReadOnlyRootFS = ins.HostConfig.ReadonlyRootfs
	graph.Privileges.CapAdd = normalizedStrings(ins.HostConfig.CapAdd)
	graph.Privileges.CapDrop = normalizedStrings(ins.HostConfig.CapDrop)

	portMap := map[int]int{}
	for _, p := range ports {
		hp, e := r.Docker.HostPort(ctx, containerID, p)
		if e == nil {
			portMap[p] = hp
		} else {
			graph.Warnings = append(graph.Warnings, e.Error())
		}
	}

	readyAt := time.Now()
	if len(sc.Probes) > 0 {
		readyAt = time.Time{}
	}
	deadline := time.Now().Add(sc.ObservationDuration())
	for time.Now().Before(deadline) {
		r.sample(ctx, containerID, &graph)
		for _, p := range sc.Probes {
			ok, probeErr := runProbe(ctx, p, portMap[p.Port])
			if probeErr != nil {
				graph.Warnings = appendUnique(graph.Warnings, "probe: "+probeErr.Error())
			}
			if ok && readyAt.IsZero() {
				readyAt = time.Now()
			}
		}
		select {
		case <-ctx.Done():
			return graph, ctx.Err()
		case <-time.After(sc.SampleDuration()):
		}
	}
	if readyAt.IsZero() {
		graph.Warnings = appendUnique(graph.Warnings, "no successful readiness probe observed")
	} else {
		graph.Lifecycle.ReadyMillis = readyAt.Sub(started).Milliseconds()
	}

	if out, e := r.Docker.Run(ctx, "diff", containerID); e == nil {
		graph.Filesystem = parseDockerDiff(out)
	} else {
		graph.Warnings = append(graph.Warnings, "filesystem diff unavailable: "+e.Error())
	}

	shutdownStart := time.Now()
	timeoutSec := int(sc.ShutdownDuration().Seconds())
	if timeoutSec < 1 {
		timeoutSec = 1
	}
	_, stopErr := r.Docker.Run(ctx, "stop", "-t", strconv.Itoa(timeoutSec), containerID)
	graph.Lifecycle.ShutdownMillis = time.Since(shutdownStart).Milliseconds()
	if stopErr != nil {
		graph.Warnings = append(graph.Warnings, "graceful stop failed: "+stopErr.Error())
	}
	if final, e := r.Docker.Inspect(ctx, containerID); e == nil {
		graph.Lifecycle.ExitCode = final.State.ExitCode
		graph.Lifecycle.OOMKilled = final.State.OOMKilled
	}

	normalizeGraph(&graph)
	return graph, nil
}

func uniquePorts(sc scenario.Scenario) []int {
	seen := map[int]bool{}
	var out []int
	for _, p := range sc.PublishPorts {
		if p > 0 && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, p := range sc.Probes {
		if p.Port > 0 && !seen[p.Port] {
			seen[p.Port] = true
			out = append(out, p.Port)
		}
	}
	return out
}

func (r *Recorder) sample(ctx context.Context, id string, graph *model.RuntimeGraph) {
	if out, err := r.Docker.Run(ctx, "top", id, "-eo", "pid,ppid,comm,args"); err == nil {
		graph.Processes = append(graph.Processes, parseDockerTop(out)...)
	} else {
		graph.Warnings = appendUnique(graph.Warnings, "process sampling unavailable: "+err.Error())
	}

	for _, spec := range []struct {
		path, proto string
		ipv6        bool
	}{{"/proc/net/tcp", "tcp", false}, {"/proc/net/tcp6", "tcp", true}, {"/proc/net/udp", "udp", false}, {"/proc/net/udp6", "udp", true}} {
		out, err := r.Docker.Run(ctx, "exec", id, "cat", spec.path)
		if err != nil {
			graph.Warnings = appendUnique(graph.Warnings, "network sampling unavailable for "+spec.path)
			continue
		}
		eps, _ := ParseProcNet(out, spec.proto, spec.ipv6)
		graph.Network = append(graph.Network, eps...)
	}

	if out, err := r.Docker.Run(ctx, "exec", id, "cat", "/proc/1/status"); err == nil {
		parseProcStatus(out, &graph.Privileges)
	} else {
		graph.Warnings = appendUnique(graph.Warnings, "privilege sampling unavailable: /proc/1/status could not be read")
	}

	if out, err := r.Docker.Run(ctx, "stats", "--no-stream", "--format", "{{json .}}", id); err == nil {
		parseStats(out, &graph.Resources)
	}
}

func parseDockerTop(out string) []model.Process {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) <= 1 {
		return nil
	}
	var result []model.Process
	for _, line := range lines[1:] {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		ppid, _ := strconv.Atoi(f[1])
		cmd := ""
		if len(f) > 3 {
			cmd = strings.Join(f[3:], " ")
		}
		result = append(result, model.Process{PID: pid, PPID: ppid, Name: f[2], Command: cmd})
	}
	return result
}

func parseDockerDiff(out string) []model.FileMutation {
	var result []model.FileMutation
	s := bufio.NewScanner(strings.NewReader(out))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if len(line) < 3 {
			continue
		}
		kind := line[:1]
		path := strings.TrimSpace(line[1:])
		if kind == "A" || kind == "C" || kind == "D" {
			result = append(result, model.FileMutation{Kind: kind, Path: path})
		}
	}
	return result
}

func parseProcStatus(content string, p *model.PrivilegeProfile) {
	for _, line := range strings.Split(content, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		switch strings.TrimSuffix(f[0], ":") {
		case "Uid":
			if len(f) >= 3 {
				p.EffectiveUID, _ = strconv.Atoi(f[2])
			}
		case "Gid":
			if len(f) >= 3 {
				p.EffectiveGID, _ = strconv.Atoi(f[2])
			}
		case "CapEff":
			p.EffectiveCapsHex = f[1]
			p.EffectiveCaps = decodeCaps(f[1])
		case "NoNewPrivs":
			p.NoNewPrivileges = f[1] == "1"
		case "Seccomp":
			p.SeccompMode, _ = strconv.Atoi(f[1])
		}
	}
}

var capNames = []string{
	"CHOWN", "DAC_OVERRIDE", "DAC_READ_SEARCH", "FOWNER", "FSETID", "KILL", "SETGID", "SETUID",
	"SETPCAP", "LINUX_IMMUTABLE", "NET_BIND_SERVICE", "NET_BROADCAST", "NET_ADMIN", "NET_RAW", "IPC_LOCK",
	"IPC_OWNER", "SYS_MODULE", "SYS_RAWIO", "SYS_CHROOT", "SYS_PTRACE", "SYS_PACCT", "SYS_ADMIN", "SYS_BOOT",
	"SYS_NICE", "SYS_RESOURCE", "SYS_TIME", "SYS_TTY_CONFIG", "MKNOD", "LEASE", "AUDIT_WRITE", "AUDIT_CONTROL",
	"SETFCAP", "MAC_OVERRIDE", "MAC_ADMIN", "SYSLOG", "WAKE_ALARM", "BLOCK_SUSPEND", "AUDIT_READ", "PERFMON",
	"BPF", "CHECKPOINT_RESTORE",
}

func decodeCaps(hexValue string) []string {
	v, err := strconv.ParseUint(hexValue, 16, 64)
	if err != nil {
		return nil
	}
	var out []string
	for i, name := range capNames {
		if v&(uint64(1)<<i) != 0 {
			out = append(out, name)
		}
	}
	return out
}

func parseStats(out string, r *model.ResourceProfile) {
	var v struct {
		MemUsage string `json:"MemUsage"`
		CPUPerc  string `json:"CPUPerc"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		return
	}
	if parts := strings.SplitN(v.MemUsage, " / ", 2); len(parts) > 0 {
		if b, err := parseByteSize(parts[0]); err == nil && b > r.PeakMemoryBytes {
			r.PeakMemoryBytes = b
		}
	}
	cpu := strings.TrimSpace(strings.TrimSuffix(v.CPUPerc, "%"))
	if f, err := strconv.ParseFloat(cpu, 64); err == nil && f > r.PeakCPUPercent {
		r.PeakCPUPercent = f
	}
}

func parseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	units := []struct {
		suffix string
		mul    float64
	}{{"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10}, {"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"B", 1}}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			n, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, u.suffix)), 64)
			if err != nil {
				return 0, err
			}
			return int64(n * u.mul), nil
		}
	}
	return 0, fmt.Errorf("unknown byte size %q", s)
}

func runProbe(ctx context.Context, p scenario.Probe, hostPort int) (bool, error) {
	if hostPort <= 0 {
		return false, fmt.Errorf("port %d is not published", p.Port)
	}
	method := p.Method
	if method == "" {
		method = http.MethodGet
	}
	path := p.Path
	if path == "" {
		path = "/"
	}
	repeat := p.Repeat
	if repeat <= 0 {
		repeat = 1
	}
	interval := time.Duration(p.IntervalMS) * time.Millisecond
	client := &http.Client{Timeout: 2 * time.Second}
	ok := false
	for i := 0; i < repeat; i++ {
		req, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("http://127.0.0.1:%d%s", hostPort, path), strings.NewReader(p.Body))
		if err != nil {
			return false, err
		}
		for k, v := range p.Headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			expected := p.ExpectStatus
			if expected == 0 {
				expected = 200
			}
			if resp.StatusCode == expected {
				ok = true
			}
		}
		if i+1 < repeat && interval > 0 {
			time.Sleep(interval)
		}
	}
	if !ok {
		return false, fmt.Errorf("%s %s did not return expected status", method, path)
	}
	return true, nil
}

func normalizeGraph(g *model.RuntimeGraph) {
	g.Processes = dedupeProcesses(g.Processes)
	g.Network = classifyInbound(dedupeNetwork(g.Network))
	g.Filesystem = dedupeFiles(g.Filesystem)
	g.Warnings = normalizedStrings(g.Warnings)
	g.Privileges.EffectiveCaps = normalizedStrings(g.Privileges.EffectiveCaps)
	g.Privileges.CapAdd = normalizedStrings(g.Privileges.CapAdd)
	g.Privileges.CapDrop = normalizedStrings(g.Privileges.CapDrop)
}

func classifyInbound(in []model.NetworkEndpoint) []model.NetworkEndpoint {
	listeners := map[string]bool{}
	for _, n := range in {
		if n.Direction == "listen" {
			listeners[fmt.Sprintf("%s|%d", n.Protocol, n.LocalPort)] = true
		}
	}
	for i := range in {
		if in[i].Direction == "outbound" && listeners[fmt.Sprintf("%s|%d", in[i].Protocol, in[i].LocalPort)] {
			in[i].Direction = "inbound"
		}
	}
	return in
}

func dedupeProcesses(in []model.Process) []model.Process {
	seen := map[string]model.Process{}
	for _, p := range in {
		key := p.Name + "\x00" + p.Command
		p.PID, p.PPID = 0, 0
		seen[key] = p
	}
	out := make([]model.Process, 0, len(seen))
	for _, p := range seen {
		out = append(out, p)
	}
	sortProcesses(out)
	return out
}

func dedupeNetwork(in []model.NetworkEndpoint) []model.NetworkEndpoint {
	seen := map[string]model.NetworkEndpoint{}
	for _, n := range in {
		key := fmt.Sprintf("%s|%s|%s|%d|%s|%d", n.Protocol, n.Direction, n.LocalAddress, n.LocalPort, n.RemoteAddress, n.RemotePort)
		seen[key] = n
	}
	out := make([]model.NetworkEndpoint, 0, len(seen))
	for _, n := range seen {
		out = append(out, n)
	}
	sortNetwork(out)
	return out
}

func dedupeFiles(in []model.FileMutation) []model.FileMutation {
	seen := map[string]model.FileMutation{}
	for _, f := range in {
		seen[f.Kind+"|"+f.Path] = f
	}
	out := make([]model.FileMutation, 0, len(seen))
	for _, f := range seen {
		out = append(out, f)
	}
	sortFiles(out)
	return out
}

func normalizedStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func appendUnique(in []string, s string) []string {
	for _, x := range in {
		if x == s {
			return in
		}
	}
	return append(in, s)
}

func sortProcesses(v []model.Process) {
	for i := 0; i < len(v); i++ {
		for j := i + 1; j < len(v); j++ {
			if v[j].Name+v[j].Command < v[i].Name+v[i].Command {
				v[i], v[j] = v[j], v[i]
			}
		}
	}
}

func sortNetwork(v []model.NetworkEndpoint) {
	for i := 0; i < len(v); i++ {
		for j := i + 1; j < len(v); j++ {
			a := fmt.Sprintf("%s%s%05d%s%05d", v[i].Protocol, v[i].Direction, v[i].LocalPort, v[i].RemoteAddress, v[i].RemotePort)
			b := fmt.Sprintf("%s%s%05d%s%05d", v[j].Protocol, v[j].Direction, v[j].LocalPort, v[j].RemoteAddress, v[j].RemotePort)
			if b < a {
				v[i], v[j] = v[j], v[i]
			}
		}
	}
}

func sortFiles(v []model.FileMutation) {
	for i := 0; i < len(v); i++ {
		for j := i + 1; j < len(v); j++ {
			if v[j].Path+v[j].Kind < v[i].Path+v[i].Kind {
				v[i], v[j] = v[j], v[i]
			}
		}
	}
}

func shortID(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
