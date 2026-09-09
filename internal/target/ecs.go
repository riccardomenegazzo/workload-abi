package target

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const mib = int64(1024 * 1024)

type ECSTarget struct {
	File              string
	Family            string
	Container         string
	ReadOnlyRootfs    bool
	WritablePaths     []string
	ContainerMemory   int64
	TaskMemory        int64
	MemoryHardLimit   int64
	MemoryReservation int64
	StopTimeout       time.Duration
	Privileged        bool
	User              string
	CapAdd            []string
	CapDrop           []string
}

func LoadECS(file, container string) (ECSTarget, error) {
	t := ECSTarget{File: file}
	data, err := os.ReadFile(file)
	if err != nil {
		return t, fmt.Errorf("read ECS task definition: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return t, fmt.Errorf("parse ECS task definition: %w", err)
	}

	t.Family = strings.TrimSpace(stringValue(root["family"]))
	t.TaskMemory = parseECSMemory(root["memory"])

	containers := sliceValue(root["containerDefinitions"])
	if len(containers) == 0 {
		return t, fmt.Errorf("ECS task definition has no containerDefinitions")
	}
	selected, err := selectECSContainer(containers, container)
	if err != nil {
		return t, err
	}
	t.Container = strings.TrimSpace(stringValue(selected["name"]))
	if t.Container == "" {
		return t, fmt.Errorf("selected ECS container is missing name")
	}

	t.ReadOnlyRootfs = boolValue(selected["readonlyRootFilesystem"])
	t.ContainerMemory = parseECSMemory(selected["memory"])
	t.MemoryReservation = parseECSMemory(selected["memoryReservation"])
	t.MemoryHardLimit = minPositive(t.ContainerMemory, t.TaskMemory)
	t.Privileged = boolValue(selected["privileged"])
	t.User = strings.TrimSpace(stringValue(selected["user"]))

	if seconds, ok := intValue(selected["stopTimeout"]); ok && seconds > 0 {
		t.StopTimeout = time.Duration(seconds) * time.Second
	}

	for _, raw := range sliceValue(selected["mountPoints"]) {
		mount := mapValue(raw)
		if boolValue(mount["readOnly"]) {
			continue
		}
		path := cleanPath(stringValue(mount["containerPath"]))
		if path != "/" || strings.TrimSpace(stringValue(mount["containerPath"])) == "/" {
			t.WritablePaths = append(t.WritablePaths, path)
		}
	}

	linuxParameters := mapValue(selected["linuxParameters"])
	capabilities := mapValue(linuxParameters["capabilities"])
	t.CapAdd = stringSlice(capabilities["add"])
	t.CapDrop = stringSlice(capabilities["drop"])

	return t, nil
}

func (t ECSTarget) Identity() string {
	family := strings.TrimSpace(t.Family)
	if family == "" {
		family = filepath.Base(t.File)
	}
	return fmt.Sprintf("ecs:%s#%s", family, t.Container)
}

func selectECSContainer(containers []any, name string) (map[string]any, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		if len(containers) != 1 {
			return nil, fmt.Errorf("ECS task definition has %d containers; select one with --container", len(containers))
		}
		return mapValue(containers[0]), nil
	}
	for _, raw := range containers {
		container := mapValue(raw)
		if stringValue(container["name"]) == name {
			return container, nil
		}
	}
	return nil, fmt.Errorf("ECS container %q not found", name)
}

func parseECSMemory(v any) int64 {
	s := strings.TrimSpace(scalarString(v))
	if s == "" {
		return 0
	}
	var value int64
	if n, ok := intValue(v); ok {
		value = n
	} else {
		decoder := json.NewDecoder(strings.NewReader(s))
		decoder.UseNumber()
		var number json.Number
		if err := decoder.Decode(&number); err == nil {
			value, _ = number.Int64()
		}
	}
	if value <= 0 {
		return 0
	}
	return value * mib
}

func minPositive(a, b int64) int64 {
	switch {
	case a > 0 && b > 0 && a < b:
		return a
	case a > 0 && b > 0:
		return b
	case a > 0:
		return a
	default:
		return b
	}
}
