package target

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type SeccompDecision string

const (
	SeccompAllow   SeccompDecision = "allow"
	SeccompDeny    SeccompDecision = "deny"
	SeccompUnknown SeccompDecision = "unknown"
)

type SeccompProfile struct {
	File          string               `json:"-"`
	DefaultAction string               `json:"defaultAction"`
	Syscalls      []SeccompSyscallRule `json:"syscalls"`
}

type SeccompSyscallRule struct {
	Names       []string        `json:"names"`
	Action      string          `json:"action"`
	Args        []json.RawMessage `json:"args,omitempty"`
	Includes    json.RawMessage `json:"includes,omitempty"`
	Excludes    json.RawMessage `json:"excludes,omitempty"`
	Conditional bool            `json:"-"`
}

func LoadSeccompProfile(path string) (SeccompProfile, error) {
	var profile SeccompProfile
	data, err := os.ReadFile(path)
	if err != nil {
		return profile, fmt.Errorf("read seccomp profile: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&profile); err != nil {
		return profile, fmt.Errorf("parse seccomp profile: %w", err)
	}
	profile.File = path
	profile.DefaultAction = normalizeSeccompAction(profile.DefaultAction)
	if profile.DefaultAction == "" {
		return profile, fmt.Errorf("seccomp profile is missing defaultAction")
	}
	for i := range profile.Syscalls {
		rule := &profile.Syscalls[i]
		rule.Action = normalizeSeccompAction(rule.Action)
		if len(rule.Names) == 0 {
			return profile, fmt.Errorf("seccomp syscall rule %d has no names", i)
		}
		if rule.Action == "" {
			return profile, fmt.Errorf("seccomp syscall rule %d has no action", i)
		}
		for j := range rule.Names {
			rule.Names[j] = strings.TrimSpace(rule.Names[j])
		}
		rule.Conditional = len(rule.Args) > 0 || rawJSONPresent(rule.Includes) || rawJSONPresent(rule.Excludes)
	}
	return profile, nil
}

func (p SeccompProfile) Decision(syscall string) (SeccompDecision, string) {
	syscall = strings.TrimSpace(syscall)
	if syscall == "" {
		return SeccompUnknown, "empty syscall"
	}

	var decisions []SeccompDecision
	var actions []string
	for _, rule := range p.Syscalls {
		if !containsSeccompName(rule.Names, syscall) {
			continue
		}
		if rule.Conditional {
			decisions = append(decisions, SeccompUnknown)
			actions = append(actions, rule.Action+" (conditional)")
			continue
		}
		decisions = append(decisions, classifySeccompAction(rule.Action))
		actions = append(actions, rule.Action)
	}

	if len(decisions) == 0 {
		return classifySeccompAction(p.DefaultAction), p.DefaultAction
	}
	decision := decisions[0]
	for _, current := range decisions[1:] {
		if current != decision {
			return SeccompUnknown, strings.Join(actions, ", ")
		}
	}
	return decision, strings.Join(actions, ", ")
}

func classifySeccompAction(action string) SeccompDecision {
	switch normalizeSeccompAction(action) {
	case "SCMP_ACT_ALLOW", "SCMP_ACT_LOG":
		return SeccompAllow
	case "SCMP_ACT_ERRNO", "SCMP_ACT_KILL", "SCMP_ACT_KILL_PROCESS", "SCMP_ACT_KILL_THREAD", "SCMP_ACT_TRAP":
		return SeccompDeny
	case "SCMP_ACT_TRACE", "SCMP_ACT_NOTIFY":
		return SeccompUnknown
	default:
		return SeccompUnknown
	}
}

func normalizeSeccompAction(action string) string {
	return strings.ToUpper(strings.TrimSpace(action))
}

func containsSeccompName(names []string, syscall string) bool {
	for _, name := range names {
		if strings.TrimSpace(name) == syscall {
			return true
		}
	}
	return false
}

func rawJSONPresent(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && string(trimmed) != "null"
}
