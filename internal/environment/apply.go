package environment

import (
	"context"
	"fmt"
	"strings"

	"github.com/riccardomenegazzo/workload-abi/internal/compat"
	"github.com/riccardomenegazzo/workload-abi/internal/model"
	"github.com/riccardomenegazzo/workload-abi/internal/policy"
	"github.com/riccardomenegazzo/workload-abi/internal/target"
)

type Options struct {
	TargetFile        string `json:"target,omitempty"`
	TargetKind        string `json:"target_kind,omitempty"`
	Service           string `json:"service,omitempty"`
	Workload          string `json:"workload,omitempty"`
	Container         string `json:"container,omitempty"`
	NetworkPolicyFile string `json:"network_policy,omitempty"`
	SeccompProfile    string `json:"seccomp_profile,omitempty"`
	PolicyFile        string `json:"policy,omitempty"`
}

func Apply(ctx context.Context, comparison model.Comparison, base, candidate model.Snapshot, opts Options) (model.Comparison, error) {
	var err error
	if opts.NetworkPolicyFile != "" && opts.TargetFile == "" {
		return comparison, fmt.Errorf("network policy requires a Kubernetes target workload")
	}
	if opts.TargetFile != "" {
		comparison, err = applyTarget(ctx, comparison, base, candidate, opts)
		if err != nil {
			return comparison, err
		}
	}
	if opts.SeccompProfile != "" {
		profile, err := target.LoadSeccompProfile(opts.SeccompProfile)
		if err != nil {
			return comparison, fmt.Errorf("load seccomp profile: %w", err)
		}
		comparison = compat.ApplySeccomp(comparison, base, candidate, profile)
	}
	if opts.PolicyFile != "" {
		p, err := policy.Load(opts.PolicyFile)
		if err != nil {
			return comparison, fmt.Errorf("load policy: %w", err)
		}
		comparison = policy.Apply(comparison, p)
	}
	return comparison, nil
}

func applyTarget(ctx context.Context, c model.Comparison, base, candidate model.Snapshot, opts Options) (model.Comparison, error) {
	kind := strings.ToLower(strings.TrimSpace(opts.TargetKind))
	if kind == "" || kind == "auto" {
		detected, err := target.Detect(opts.TargetFile)
		if err != nil {
			return c, err
		}
		kind = detected
	}

	switch kind {
	case "compose":
		if opts.NetworkPolicyFile != "" {
			return c, fmt.Errorf("network policy is only valid with a Kubernetes target")
		}
		t, err := target.LoadCompose(ctx, opts.TargetFile, opts.Service)
		if err != nil {
			return c, err
		}
		return compat.ApplyCompose(c, base, candidate, t), nil
	case "kubernetes", "k8s":
		t, err := target.LoadKubernetes(ctx, opts.TargetFile, opts.Workload, opts.Container)
		if err != nil {
			return c, err
		}
		if opts.NetworkPolicyFile != "" {
			policies, err := target.LoadKubernetesNetworkPolicies(ctx, opts.NetworkPolicyFile)
			if err != nil {
				return c, err
			}
			t.NetworkPolicyFile = opts.NetworkPolicyFile
			t.NetworkPolicies = policies
		}
		return compat.ApplyKubernetes(c, base, candidate, t), nil
	case "ecs":
		if opts.NetworkPolicyFile != "" {
			return c, fmt.Errorf("network policy is only valid with a Kubernetes target")
		}
		t, err := target.LoadECS(opts.TargetFile, opts.Container)
		if err != nil {
			return c, err
		}
		return compat.ApplyECS(c, base, candidate, t), nil
	default:
		return c, fmt.Errorf("unsupported target kind %q", kind)
	}
}
