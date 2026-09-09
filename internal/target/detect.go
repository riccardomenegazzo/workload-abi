package target

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func Detect(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read target: %w", err)
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return "", fmt.Errorf("target file is empty")
	}
	if strings.HasPrefix(trimmed, "{") {
		var obj map[string]any
		if err := json.Unmarshal(data, &obj); err != nil {
			return "", fmt.Errorf("parse target JSON: %w", err)
		}
		if _, ok := obj["containerDefinitions"]; ok {
			return "ecs", nil
		}
		if _, ok := obj["apiVersion"]; ok {
			if _, ok := obj["kind"]; ok {
				return "kubernetes", nil
			}
		}
		if _, ok := obj["services"]; ok {
			return "compose", nil
		}
	}
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "apiVersion:") || strings.HasPrefix(line, "kind:") {
			return "kubernetes", nil
		}
		if line == "services:" {
			return "compose", nil
		}
	}
	return "", fmt.Errorf("cannot detect target type; use --target-kind compose, kubernetes, or ecs")
}
