package scenario

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

type Spec struct {
	Name        string            `json:"name,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	Command     []string          `json:"command,omitempty"`
}

func Load(path string) (Spec, error) {
	var s Spec
	data, err := os.ReadFile(path)
	if err != nil {
		return s, fmt.Errorf("read scenario: %w", err)
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("parse scenario: %w", err)
	}
	return s, nil
}

func (s Spec) EnvList() []string {
	keys := make([]string, 0, len(s.Environment))
	for k := range s.Environment {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+s.Environment[k])
	}
	return out
}
