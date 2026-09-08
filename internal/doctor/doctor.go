package doctor

import (
	"context"
	"os/exec"
	"strings"
)

type Check struct {
	Component string `json:"component"`
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
	Error     string `json:"error,omitempty"`
}

type Result struct {
	Checks []Check `json:"checks"`
}

func Run(ctx context.Context) Result {
	return Result{Checks: []Check{
		commandCheck(ctx, "docker", "docker", "version", "--format", "{{.Client.Version}}"),
		commandCheck(ctx, "docker-engine", "docker", "info", "--format", "{{.ServerVersion}}"),
		commandCheck(ctx, "docker-compose", "docker", "compose", "version", "--short"),
		commandCheck(ctx, "kubectl", "kubectl", "version", "--client", "--output=json"),
	}}
}

func commandCheck(ctx context.Context, component, binary string, args ...string) Check {
	check := Check{Component: component}
	if _, err := exec.LookPath(binary); err != nil {
		check.Error = binary + " not found in PATH"
		return check
	}
	out, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
	if err != nil {
		check.Error = strings.TrimSpace(string(out))
		if check.Error == "" {
			check.Error = err.Error()
		}
		return check
	}
	check.Available = true
	check.Version = truncate(strings.TrimSpace(string(out)), 512)
	return check
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
