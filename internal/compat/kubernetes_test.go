package compat

import (
	"testing"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
	"github.com/riccardomenegazzo/workload-abi/internal/target"
)

func TestApplyKubernetesNetworkPolicyDeniesNewOutboundIP(t *testing.T) {
	base := model.Snapshot{Image: "app:v1"}
	candidate := model.Snapshot{
		Image: "app:v2",
		RuntimeEvents: []model.RuntimeEvent{{
			Source: "fixture", Category: "network", Operation: "connect",
			Process: "/usr/bin/app", Target: "203.0.113.10:443", Protocol: "tcp", Direction: "outbound",
		}},
	}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}
	targetEnv := target.KubernetesTarget{
		File: "deployment.json", Kind: "Deployment", Name: "api", Namespace: "payments", Container: "api",
		PodLabels: map[string]string{"app": "api"}, NetworkPolicyFile: "egress.json",
		NetworkPolicies: []target.KubernetesNetworkPolicy{{
			Name: "api-egress", Namespace: "payments", EgressIsolating: true,
			PodSelector: target.KubernetesLabelSelector{MatchLabels: map[string]string{"app": "api"}},
			Egress: []target.KubernetesEgressRule{{
				To: []target.KubernetesNetworkPolicyPeer{{IPBlock: &target.KubernetesIPBlock{CIDR: "10.0.0.0/8"}}},
				Ports: []target.KubernetesNetworkPolicyPort{{Protocol: "TCP", Port: "443"}},
			}},
		}},
	}

	got := ApplyKubernetes(comparison, base, candidate, targetEnv)
	if got.Verdict != "BREAKING" {
		t.Fatalf("verdict=%s want BREAKING; changes=%#v", got.Verdict, got.Changes)
	}
	var conflict *model.Change
	for i := range got.Changes {
		if got.Changes[i].Surface == "runtime-network" && got.Changes[i].Kind == "target-conflict" {
			conflict = &got.Changes[i]
			break
		}
	}
	if conflict == nil {
		t.Fatalf("missing runtime-network target conflict: %#v", got.Changes)
	}
	if conflict.After == "" || conflict.Severity != "breaking" {
		t.Fatalf("unexpected conflict: %#v", conflict)
	}
	if got.Target != "kubernetes:Deployment/api#api+networkpolicy:egress.json" {
		t.Fatalf("target=%q", got.Target)
	}
}

func TestApplyKubernetesNetworkPolicyAllowsCIDRAndPort(t *testing.T) {
	base := model.Snapshot{Image: "app:v1"}
	candidate := model.Snapshot{Image: "app:v2", RuntimeEvents: []model.RuntimeEvent{{
		Category: "network", Operation: "connect", Target: "10.20.30.40:443", Protocol: "tcp", Direction: "outbound",
	}}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}
	targetEnv := target.KubernetesTarget{
		Kind: "Deployment", Name: "api", Namespace: "default", Container: "api", PodLabels: map[string]string{"app": "api"},
		NetworkPolicies: []target.KubernetesNetworkPolicy{{
			Name: "allow-private", Namespace: "default", EgressIsolating: true,
			PodSelector: target.KubernetesLabelSelector{MatchLabels: map[string]string{"app": "api"}},
			Egress: []target.KubernetesEgressRule{{
				To: []target.KubernetesNetworkPolicyPeer{{IPBlock: &target.KubernetesIPBlock{CIDR: "10.0.0.0/8"}}},
				Ports: []target.KubernetesNetworkPolicyPort{{Protocol: "TCP", Port: "443"}},
			}},
		}},
	}
	got := ApplyKubernetes(comparison, base, candidate, targetEnv)
	if got.Verdict == "BREAKING" {
		t.Fatalf("allowed dependency should not break: %#v", got.Changes)
	}
}

func TestApplyKubernetesNetworkPolicyDoesNotRejudgeExistingDependency(t *testing.T) {
	event := model.RuntimeEvent{Category: "network", Operation: "connect", Target: "203.0.113.10:443", Protocol: "tcp", Direction: "outbound"}
	base := model.Snapshot{Image: "app:v1", RuntimeEvents: []model.RuntimeEvent{event}}
	candidate := model.Snapshot{Image: "app:v2", RuntimeEvents: []model.RuntimeEvent{event}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "COMPATIBLE"}
	targetEnv := target.KubernetesTarget{
		Kind: "Deployment", Name: "api", Namespace: "default", Container: "api", PodLabels: map[string]string{"app": "api"},
		NetworkPolicies: []target.KubernetesNetworkPolicy{{
			Name: "deny-all", Namespace: "default", EgressIsolating: true,
			PodSelector: target.KubernetesLabelSelector{MatchLabels: map[string]string{"app": "api"}},
		}},
	}
	got := ApplyKubernetes(comparison, base, candidate, targetEnv)
	if got.Verdict == "BREAKING" {
		t.Fatalf("pre-existing dependency should not become a release regression: %#v", got.Changes)
	}
}

func TestApplyKubernetesNetworkPolicySelectorPeerRemainsUnknown(t *testing.T) {
	base := model.Snapshot{Image: "app:v1"}
	candidate := model.Snapshot{Image: "app:v2", RuntimeEvents: []model.RuntimeEvent{{
		Category: "network", Operation: "connect", Target: "10.42.0.7:8080", Protocol: "tcp", Direction: "outbound",
	}}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}
	selector := target.KubernetesLabelSelector{MatchLabels: map[string]string{"app": "backend"}}
	targetEnv := target.KubernetesTarget{
		Kind: "Deployment", Name: "api", Namespace: "default", Container: "api", PodLabels: map[string]string{"app": "api"},
		NetworkPolicies: []target.KubernetesNetworkPolicy{{
			Name: "allow-backend", Namespace: "default", EgressIsolating: true,
			PodSelector: target.KubernetesLabelSelector{MatchLabels: map[string]string{"app": "api"}},
			Egress: []target.KubernetesEgressRule{{To: []target.KubernetesNetworkPolicyPeer{{PodSelector: &selector}}}},
		}},
	}
	got := ApplyKubernetes(comparison, base, candidate, targetEnv)
	if got.Verdict == "BREAKING" {
		t.Fatalf("unresolved destination selector must stay conservative: %#v", got.Changes)
	}
}

func TestApplyKubernetesNetworkPolicyEmptyEgressDeniesAll(t *testing.T) {
	base := model.Snapshot{Image: "app:v1"}
	candidate := model.Snapshot{Image: "app:v2", RuntimeEvents: []model.RuntimeEvent{{
		Category: "network", Operation: "connect", Target: "192.0.2.5:53", Protocol: "udp", Direction: "outbound",
	}}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}
	targetEnv := target.KubernetesTarget{
		Kind: "Pod", Name: "api", Namespace: "default", Container: "api", PodLabels: map[string]string{"app": "api"},
		NetworkPolicies: []target.KubernetesNetworkPolicy{{
			Name: "default-deny-egress", Namespace: "default", EgressIsolating: true,
			PodSelector: target.KubernetesLabelSelector{},
		}},
	}
	got := ApplyKubernetes(comparison, base, candidate, targetEnv)
	if got.Verdict != "BREAKING" {
		t.Fatalf("deny-all egress should block new dependency: %#v", got.Changes)
	}
}

func TestApplyKubernetesNetworkPolicyCombinesPoliciesAdditively(t *testing.T) {
	base := model.Snapshot{Image: "app:v1"}
	candidate := model.Snapshot{Image: "app:v2", RuntimeEvents: []model.RuntimeEvent{{
		Category: "network", Operation: "connect", Target: "203.0.113.10:443", Protocol: "tcp", Direction: "outbound",
	}}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}
	selector := target.KubernetesLabelSelector{MatchLabels: map[string]string{"app": "api"}}
	targetEnv := target.KubernetesTarget{
		Kind: "Deployment", Name: "api", Namespace: "default", Container: "api", PodLabels: map[string]string{"app": "api"},
		NetworkPolicies: []target.KubernetesNetworkPolicy{
			{Name: "deny-by-omission", Namespace: "default", EgressIsolating: true, PodSelector: selector},
			{Name: "allow-public", Namespace: "default", EgressIsolating: true, PodSelector: selector, Egress: []target.KubernetesEgressRule{{
				To: []target.KubernetesNetworkPolicyPeer{{IPBlock: &target.KubernetesIPBlock{CIDR: "203.0.113.0/24"}}},
				Ports: []target.KubernetesNetworkPolicyPort{{Protocol: "TCP", Port: "443"}},
			}}},
		},
	}
	got := ApplyKubernetes(comparison, base, candidate, targetEnv)
	if got.Verdict == "BREAKING" {
		t.Fatalf("egress policies are additive; one allow must permit the dependency: %#v", got.Changes)
	}
}
