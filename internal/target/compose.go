package target

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type ComposeTarget struct {
	File            string
	Service         string
	ReadOnlyRootfs  bool
	CapDrop         []string
	MemoryLimit     int64
	StopGracePeriod time.Duration
	WritablePaths   []string
}

type composeConfig struct {
	Services map[string]composeService `json:"services"`
}

type composeService struct {
	ReadOnly        bool            `json:"read_only"`
	CapDrop         []string        `json:"cap_drop"`
	MemLimit        json.RawMessage `json:"mem_limit"`
	StopGracePeriod string          `json:"stop_grace_period"`
	Volumes         []composeVolume `json:"volumes"`
	Tmpfs           []string        `json:"tmpfs"`
}

type composeVolume struct {
	Type     string `json:"type"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only"`
}

func LoadCompose(ctx context.Context, file, service string) (ComposeTarget, error) {
	t := ComposeTarget{File: file, Service: service}
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", file, "config", "--format", "json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return t, fmt.Errorf("render compose config: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	var cfg composeConfig
	if err := json.Unmarshal(out, &cfg); err != nil {
		return t, fmt.Errorf("parse compose config: %w", err)
	}
	if len(cfg.Services) == 0 {
		return t, fmt.Errorf("compose file has no services")
	}
	if service == "" {
		if len(cfg.Services) != 1 {
			return t, fmt.Errorf("compose file has %d services; select one with --service", len(cfg.Services))
		}
		for name := range cfg.Services {
			service = name
		}
		t.Service = service
	}

	svc, ok := cfg.Services[service]
	if !ok {
		return t, fmt.Errorf("compose service %q not found", service)
	}
	t.ReadOnlyRootfs = svc.ReadOnly
	t.CapDrop = append([]string(nil), svc.CapDrop...)
	t.MemoryLimit = parseMemoryLimit(svc.MemLimit)
	if svc.StopGracePeriod != "" {
		if d, err := time.ParseDuration(svc.StopGracePeriod); err == nil {
			t.StopGracePeriod = d
		}
	}
	for _, v := range svc.Volumes {
		if v.Target != "" && !v.ReadOnly {
			t.WritablePaths = append(t.WritablePaths, cleanPath(v.Target))
		}
	}
	for _, v := range svc.Tmpfs {
		path := strings.SplitN(v, ":", 2)[0]
		if path != "" {
			t.WritablePaths = append(t.WritablePaths, cleanPath(path))
		}
	}
	return t, nil
}

func parseMemoryLimit(raw json.RawMessage) int64 {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return parseSize(s)
	}
	return 0
}

func parseSize(s string) int64 {
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
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	return 0
}

func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	p = strings.TrimSpace(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if p != "/" {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}
