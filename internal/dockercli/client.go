package dockercli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
)

const defaultObserverImage = "busybox:1.37.0"

type Client struct {
	Binary string
}

func New() *Client { return &Client{Binary: "docker"} }

func (c *Client) Run(ctx context.Context, args ...string) (string, error) {
	out, err := c.runRaw(ctx, args...)
	if err == nil {
		return out, nil
	}

	// Minimal/distroless images frequently do not ship cat (or even a shell).
	// Proc reads are observation operations, so retry them from a tiny helper
	// that joins the target PID namespace instead of modifying the workload.
	if containerID, procPath, ok := procRead(args); ok {
		fallback, fallbackErr := c.readProcWithObserver(ctx, containerID, procPath)
		if fallbackErr == nil {
			return fallback, nil
		}
		return out, fmt.Errorf("%w; namespace observer fallback failed: %v", err, fallbackErr)
	}
	return out, err
}

func (c *Client) runRaw(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, c.Binary, args...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return strings.TrimSpace(out.String()), fmt.Errorf("docker %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(out.String()), nil
}

func procRead(args []string) (containerID, procPath string, ok bool) {
	if len(args) != 4 || args[0] != "exec" || args[2] != "cat" || !strings.HasPrefix(args[3], "/proc/") {
		return "", "", false
	}
	return args[1], args[3], true
}

func (c *Client) readProcWithObserver(ctx context.Context, containerID, procPath string) (string, error) {
	observerImage := strings.TrimSpace(os.Getenv("WABI_OBSERVER_IMAGE"))
	if observerImage == "" {
		observerImage = defaultObserverImage
	}

	// /proc/net is relative to the reader's own network namespace. Since the
	// helper joins only the target PID namespace, use /proc/1/net to address
	// the target's PID 1 network namespace explicitly.
	observerPath := procPath
	if strings.HasPrefix(procPath, "/proc/net/") {
		observerPath = path.Join("/proc/1/net", strings.TrimPrefix(procPath, "/proc/net/"))
	}

	return c.runRaw(ctx,
		"run", "--rm",
		"--pid", "container:"+containerID,
		"--label", "io.wabi.observer=true",
		observerImage,
		"cat", observerPath,
	)
}

func (c *Client) Available(ctx context.Context) error {
	_, err := c.Run(ctx, "info", "--format", "{{.ServerVersion}}")
	return err
}

func (c *Client) EnsureImage(ctx context.Context, image string) error {
	if _, err := c.Run(ctx, "image", "inspect", image, "--format", "{{.Id}}"); err == nil {
		return nil
	}
	_, err := c.Run(ctx, "pull", image)
	if err != nil {
		return fmt.Errorf("image %q is not available locally and pull failed: %w", image, err)
	}
	return nil
}

type Inspect struct {
	ID    string `json:"Id"`
	Image string `json:"Image"`
	Name  string `json:"Name"`
	State struct {
		Running   bool `json:"Running"`
		ExitCode  int  `json:"ExitCode"`
		OOMKilled bool `json:"OOMKilled"`
	} `json:"State"`
	Config struct {
		Image string `json:"Image"`
		User  string `json:"User"`
	} `json:"Config"`
	HostConfig struct {
		Privileged     bool     `json:"Privileged"`
		ReadonlyRootfs bool     `json:"ReadonlyRootfs"`
		CapAdd         []string `json:"CapAdd"`
		CapDrop        []string `json:"CapDrop"`
		NetworkMode    string   `json:"NetworkMode"`
	} `json:"HostConfig"`
	NetworkSettings struct {
		Ports map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
	} `json:"NetworkSettings"`
}

func (c *Client) Inspect(ctx context.Context, id string) (Inspect, error) {
	out, err := c.Run(ctx, "inspect", id)
	if err != nil {
		return Inspect{}, err
	}
	var v []Inspect
	if err := json.Unmarshal([]byte(out), &v); err != nil || len(v) == 0 {
		if err == nil {
			err = fmt.Errorf("empty inspect response")
		}
		return Inspect{}, fmt.Errorf("decode docker inspect: %w", err)
	}
	return v[0], nil
}

func (c *Client) ImageDigest(ctx context.Context, image string) (id, digest string, err error) {
	out, err := c.Run(ctx, "image", "inspect", image)
	if err != nil {
		return "", "", err
	}
	var v []struct {
		ID          string   `json:"Id"`
		RepoDigests []string `json:"RepoDigests"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil || len(v) == 0 {
		if err == nil {
			err = fmt.Errorf("empty image inspect response")
		}
		return "", "", fmt.Errorf("decode image inspect: %w", err)
	}
	if len(v[0].RepoDigests) > 0 {
		parts := strings.SplitN(v[0].RepoDigests[0], "@", 2)
		if len(parts) == 2 {
			digest = parts[1]
		}
	}
	return v[0].ID, digest, nil
}

func (c *Client) HostPort(ctx context.Context, id string, containerPort int) (int, error) {
	ins, err := c.Inspect(ctx, id)
	if err != nil {
		return 0, err
	}
	bindings := ins.NetworkSettings.Ports[fmt.Sprintf("%d/tcp", containerPort)]
	if len(bindings) == 0 {
		return 0, fmt.Errorf("container port %d is not published", containerPort)
	}
	p, err := strconv.Atoi(bindings[0].HostPort)
	if err != nil {
		return 0, fmt.Errorf("invalid published port %q: %w", bindings[0].HostPort, err)
	}
	return p, nil
}

func TimeoutContext(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		d = 30 * time.Second
	}
	return context.WithTimeout(parent, d)
}
