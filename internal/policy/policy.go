package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

type Spec struct {
	Name            string   `json:"name,omitempty"`
	RequireScenario bool     `json:"require_scenario,omitempty"`
	RequireTarget   bool     `json:"require_target,omitempty"`
	DenySeverities  []string `json:"deny_severities,omitempty"`
	DenySurfaces    []string `json:"deny_surfaces,omitempty"`
	DenyKinds       []string `json:"deny_kinds,omitempty"`
	MaxChanges      int      `json:"max_changes,omitempty"`
}

func Load(path string) (Spec, error) {
	var p Spec
	data, err := os.ReadFile(path)
	if err != nil {
		return p, fmt.Errorf("read policy: %w", err)
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return p, fmt.Errorf("parse policy: %w", err)
	}
	if p.MaxChanges < 0 {
		return p, fmt.Errorf("policy max_changes cannot be negative")
	}
	return p, nil
}

func Apply(c model.Comparison, p Spec) model.Comparison {
	name := p.Name
	if name == "" {
		name = "unnamed-policy"
	}
	c.Policy = name

	var violations []string
	if p.RequireScenario && c.Scenario == "" {
		violations = append(violations, "policy requires an explicit scenario")
	}
	if p.RequireTarget && c.Target == "" {
		violations = append(violations, "policy requires a target environment")
	}
	if p.MaxChanges > 0 && len(c.Changes) > p.MaxChanges {
		violations = append(violations, fmt.Sprintf("change count %d exceeds policy maximum %d", len(c.Changes), p.MaxChanges))
	}

	for _, ch := range append([]model.Change(nil), c.Changes...) {
		if containsFold(p.DenySeverities, ch.Severity) {
			violations = append(violations, fmt.Sprintf("policy denies severity %s: %s", ch.Severity, ch.Message))
		}
		if containsFold(p.DenySurfaces, ch.Surface) {
			violations = append(violations, fmt.Sprintf("policy denies surface %s: %s", ch.Surface, ch.Message))
		}
		if containsFold(p.DenyKinds, ch.Kind) {
			violations = append(violations, fmt.Sprintf("policy denies change kind %s: %s", ch.Kind, ch.Message))
		}
	}

	seen := map[string]struct{}{}
	for _, message := range violations {
		if _, ok := seen[message]; ok {
			continue
		}
		seen[message] = struct{}{}
		c.Changes = append(c.Changes, model.Change{
			Surface:  "policy",
			Kind:     "policy-violation",
			Severity: "breaking",
			Message:  message,
		})
		c.Verdict = "BREAKING"
	}
	return c
}

func containsFold(xs []string, value string) bool {
	for _, x := range xs {
		if strings.EqualFold(strings.TrimSpace(x), value) {
			return true
		}
	}
	return false
}
