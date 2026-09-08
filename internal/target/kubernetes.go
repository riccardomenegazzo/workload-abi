package target

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type KubernetesTarget struct {
	File                     string
	Kind                     string
	Name                     string
	Container                string
	ReadOnlyRootfs           bool
	RunAsNonRoot             bool
	RunAsUser                *int64
	Privileged               bool
	AllowPrivilegeEscalation *bool
	CapDrop                  []string
	CapAdd                   []string
	MemoryLimit              int64
	StopGracePeriod          time.Duration
	WritablePaths            []string
}

func LoadKubernetes(ctx context.Context, file, workload, container string) (KubernetesTarget, error) {
	t := KubernetesTarget{File: file}
	data, err := os.ReadFile(file)
	if err != nil {
		return t, fmt.Errorf("read kubernetes target: %w", err)
	}
	data, err = kubernetesJSON(ctx, file, data)
	if err != nil {
		return t, err
	}

	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return t, fmt.Errorf("parse kubernetes target: %w", err)
	}
	obj, err := selectKubernetesObject(root, workload)
	if err != nil {
		return t, err
	}
	t.Kind = stringValue(obj["kind"])
	meta := mapValue(obj["metadata"])
	t.Name = stringValue(meta["name"])
	if t.Name == "" {
		t.Name = workload
	}

	podSpec, err := extractPodSpec(obj)
	if err != nil {
		return t, err
	}
	containers := sliceValue(podSpec["containers"])
	if len(containers) == 0 {
		return t, fmt.Errorf("kubernetes workload has no containers")
	}
	selected, err := selectContainer(containers, container)
	if err != nil {
		return t, err
	}
	t.Container = stringValue(selected["name"])
	if t.Container == "" {
		t.Container = container
	}

	podSecurity := mapValue(podSpec["securityContext"])
	containerSecurity := mapValue(selected["securityContext"])
	t.ReadOnlyRootfs = boolValue(containerSecurity["readOnlyRootFilesystem"])
	t.Privileged = boolValue(containerSecurity["privileged"])
	if v, ok := boolPointer(containerSecurity["allowPrivilegeEscalation"]); ok {
		t.AllowPrivilegeEscalation = v
	}
	if v, ok := boolPointer(containerSecurity["runAsNonRoot"]); ok {
		t.RunAsNonRoot = *v
	} else if v, ok := boolPointer(podSecurity["runAsNonRoot"]); ok {
		t.RunAsNonRoot = *v
	}
	if v, ok := intPointer(containerSecurity["runAsUser"]); ok {
		t.RunAsUser = v
	} else if v, ok := intPointer(podSecurity["runAsUser"]); ok {
		t.RunAsUser = v
	}
	caps := mapValue(containerSecurity["capabilities"])
	t.CapDrop = stringSlice(caps["drop"])
	t.CapAdd = stringSlice(caps["add"])

	resources := mapValue(selected["resources"])
	limits := mapValue(resources["limits"])
	t.MemoryLimit = parseKubernetesSize(stringValue(limits["memory"]))

	for _, raw := range sliceValue(selected["volumeMounts"]) {
		mount := mapValue(raw)
		if boolValue(mount["readOnly"]) {
			continue
		}
		path := cleanPath(stringValue(mount["mountPath"]))
		if path != "/" || stringValue(mount["mountPath"]) == "/" {
			t.WritablePaths = append(t.WritablePaths, path)
		}
	}
	if seconds, ok := intValue(podSpec["terminationGracePeriodSeconds"]); ok && seconds > 0 {
		t.StopGracePeriod = time.Duration(seconds) * time.Second
	}
	return t, nil
}

func kubernetesJSON(ctx context.Context, file string, data []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return data, nil
	}
	if _, err := exec.LookPath("kubectl"); err != nil {
		return nil, fmt.Errorf("kubernetes YAML target requires kubectl in PATH; JSON manifests are parsed natively")
	}
	cmd := exec.CommandContext(ctx, "kubectl", "create", "--dry-run=client", "-f", file, "-o", "json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("render kubernetes YAML with kubectl: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func selectKubernetesObject(root any, workload string) (map[string]any, error) {
	obj := mapValue(root)
	if strings.EqualFold(stringValue(obj["kind"]), "List") {
		items := sliceValue(obj["items"])
		if len(items) == 0 {
			return nil, fmt.Errorf("kubernetes List target is empty")
		}
		if workload == "" && len(items) != 1 {
			return nil, fmt.Errorf("kubernetes target contains %d workloads; select one with --workload", len(items))
		}
		for _, raw := range items {
			item := mapValue(raw)
			name := stringValue(mapValue(item["metadata"])["name"])
			if workload == "" || name == workload {
				return item, nil
			}
		}
		return nil, fmt.Errorf("kubernetes workload %q not found", workload)
	}
	name := stringValue(mapValue(obj["metadata"])["name"])
	if workload != "" && workload != name {
		return nil, fmt.Errorf("kubernetes workload %q not found (manifest contains %q)", workload, name)
	}
	return obj, nil
}

func extractPodSpec(obj map[string]any) (map[string]any, error) {
	kind := strings.ToLower(stringValue(obj["kind"]))
	spec := mapValue(obj["spec"])
	switch kind {
	case "pod":
		return spec, nil
	case "deployment", "statefulset", "daemonset", "replicaset", "job":
		return mapValue(mapValue(spec["template"])["spec"]), nil
	case "cronjob":
		jobSpec := mapValue(mapValue(spec["jobTemplate"])["spec"])
		return mapValue(mapValue(jobSpec["template"])["spec"]), nil
	default:
		return nil, fmt.Errorf("unsupported kubernetes workload kind %q", stringValue(obj["kind"]))
	}
}

func selectContainer(containers []any, name string) (map[string]any, error) {
	if name == "" {
		if len(containers) != 1 {
			return nil, fmt.Errorf("kubernetes workload has %d containers; select one with --container", len(containers))
		}
		return mapValue(containers[0]), nil
	}
	for _, raw := range containers {
		c := mapValue(raw)
		if stringValue(c["name"]) == name {
			return c, nil
		}
	}
	return nil, fmt.Errorf("kubernetes container %q not found", name)
}

func parseKubernetesSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	units := []struct {
		suffix string
		mul    float64
	}{
		{"Ti", 1024 * 1024 * 1024 * 1024},
		{"Gi", 1024 * 1024 * 1024},
		{"Mi", 1024 * 1024},
		{"Ki", 1024},
		{"T", 1000 * 1000 * 1000 * 1000},
		{"G", 1000 * 1000 * 1000},
		{"M", 1000 * 1000},
		{"K", 1000},
	}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, u.suffix)), 64)
			if err == nil {
				return int64(v * u.mul)
			}
		}
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func mapValue(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func sliceValue(v any) []any {
	if xs, ok := v.([]any); ok {
		return xs
	}
	return nil
}

func stringValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	default:
		return ""
	}
}

func boolValue(v any) bool {
	x, _ := v.(bool)
	return x
}

func boolPointer(v any) (*bool, bool) {
	x, ok := v.(bool)
	if !ok {
		return nil, false
	}
	return &x, true
}

func intPointer(v any) (*int64, bool) {
	n, ok := intValue(v)
	if !ok {
		return nil, false
	}
	return &n, true
}

func intValue(v any) (int64, bool) {
	switch x := v.(type) {
	case float64:
		return int64(x), true
	case json.Number:
		n, err := x.Int64()
		return n, err == nil
	case int64:
		return x, true
	case int:
		return int64(x), true
	default:
		return 0, false
	}
}

func stringSlice(v any) []string {
	var out []string
	for _, raw := range sliceValue(v) {
		if s := stringValue(raw); s != "" {
			out = append(out, s)
		}
	}
	return out
}
