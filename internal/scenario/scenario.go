package scenario

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

type Spec struct {
	Name        string            `json:"name,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	Command     []string          `json:"command,omitempty"`
	Steps       []Step            `json:"steps,omitempty"`
}

type Step struct {
	Name         string   `json:"name,omitempty"`
	After        string   `json:"after,omitempty"`
	Exec         []string `json:"exec"`
	Timeout      string   `json:"timeout,omitempty"`
	AllowFailure bool     `json:"allow_failure,omitempty"`
}

type StepPlan struct {
	Name         string
	After        time.Duration
	Command      []string
	Timeout      time.Duration
	AllowFailure bool
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
	if err := s.Validate(); err != nil {
		return s, err
	}
	return s, nil
}

func (s Spec) Validate() error {
	seen := map[string]struct{}{}
	for i, step := range s.Steps {
		if len(step.Exec) == 0 {
			return fmt.Errorf("scenario step %d has no exec command", i+1)
		}
		for _, arg := range step.Exec {
			if strings.ContainsRune(arg, '\x00') {
				return fmt.Errorf("scenario step %d contains a NUL byte", i+1)
			}
		}
		if step.Name != "" {
			if _, ok := seen[step.Name]; ok {
				return fmt.Errorf("scenario step name %q is duplicated", step.Name)
			}
			seen[step.Name] = struct{}{}
		}
		if step.After != "" {
			if _, err := time.ParseDuration(step.After); err != nil {
				return fmt.Errorf("scenario step %d invalid after duration: %w", i+1, err)
			}
		}
		if step.Timeout != "" {
			d, err := time.ParseDuration(step.Timeout)
			if err != nil {
				return fmt.Errorf("scenario step %d invalid timeout: %w", i+1, err)
			}
			if d <= 0 {
				return fmt.Errorf("scenario step %d timeout must be positive", i+1)
			}
		}
	}
	return nil
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

func (s Spec) Plans() ([]StepPlan, error) {
	out := make([]StepPlan, 0, len(s.Steps))
	for i, step := range s.Steps {
		after := time.Duration(0)
		if step.After != "" {
			d, err := time.ParseDuration(step.After)
			if err != nil {
				return nil, err
			}
			after = d
		}
		timeout := 5 * time.Second
		if step.Timeout != "" {
			d, err := time.ParseDuration(step.Timeout)
			if err != nil {
				return nil, err
			}
			timeout = d
		}
		name := step.Name
		if name == "" {
			name = fmt.Sprintf("step-%d", i+1)
		}
		out = append(out, StepPlan{
			Name:         name,
			After:        after,
			Command:      append([]string(nil), step.Exec...),
			Timeout:      timeout,
			AllowFailure: step.AllowFailure,
		})
	}
	return out, nil
}
