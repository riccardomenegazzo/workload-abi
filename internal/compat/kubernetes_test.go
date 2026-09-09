package compat

import (
	"testing"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
	"github.com/riccardomenegazzo/workload-abi/internal/target"
)

func TestApplyKubernetesNetworkPolicyDeniesNewOutboundIP(t *testing.T) {
	base := model.Snapshot{Image: "app:v1"}
	candidate := model.Snapshot{Image: "app:v2", RuntimeEvents: []model.RuntimeEvent{{Source: "fixture", Category: "network", Operation: "connect", Process: "/usr/bin/app", Target: "203.0.113.10:443", Protocol: "tcp", Direction: "outbound"}}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}
	targetEnv := testKubernetesTarget(testIPBlockPolicy("api-egress", "10.0.0.0/8", "443"))
	targetEnv.Namespace = "payments"
	targetEnv.NetworkPolicyFile = "egress.json"
	targetEnv.NetworkPolicies[0].Namespace = "payments"

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
	candidate := model.Snapshot{Image: "app:v2", RuntimeEvents: []model.RuntimeEvent{{Category: "network", Operation: "connect", Target: "10.20.30.40:443", Protocol: "tcp", Direction: "outbound"}}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}

	got := ApplyKubernetes(comparison, base, candidate, testKubernetesTarget(testIPBlockPolicy("allow-private", "10.0.0.0/8", "443")))
	if got.Verdict == "BREAKING" {
		t.Fatalf("allowed dependency should not break: %#v", got.Changes)
	}
}

func TestApplyKubernetesNetworkPolicyDoesNotRejudgeExistingDependency(t *testing.T) {
	event := model.RuntimeEvent{Category: "network", Operation: "connect", Target: "203.0.113.10:443", Protocol: "tcp", Direction: "outbound"}
	base := model.Snapshot{Image: "app:v1", RuntimeEvents: []model.RuntimeEvent{event}}
	candidate := model.Snapshot{Image: "app:v2", RuntimeEvents: []model.RuntimeEvent{event}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "COMPATIBLE"}

	got := ApplyKubernetes(comparison, base, candidate, testKubernetesTarget(testDenyAllPolicy("deny-all")))
	if got.Verdict == "BREAKING" {
		t.Fatalf("pre-existing dependency should not become a release regression: %#v", got.Changes)
	}
}

func TestApplyKubernetesNetworkPolicySelectorPeerRemainsUnknown(t *testing.T) {
	base := model.Snapshot{Image: "app:v1"}
	candidate := model.Snapshot{Image: "app:v2", RuntimeEvents: []model.RuntimeEvent{{Category: "network", Operation: "connect", Target: "10.42.0.7:8080", Protocol: "tcp", Direction: "outbound"}}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}
	selector := target.KubernetesLabelSelector{MatchLabels: map[string]string{"app": "backend"}}
	policy := testDenyAllPolicy("allow-backend")
	policy.Egress = []target.KubernetesEgressRule{{To: []target.KubernetesNetworkPolicyPeer{{PodSelector: &selector}}}}

	got := ApplyKubernetes(comparison, base, candidate, testKubernetesTarget(policy))
	if got.Verdict == "BREAKING" {
		t.Fatalf("unresolved destination selector must stay conservative: %#v", got.Changes)
	}
}

func TestApplyKubernetesNetworkPolicyEmptyEgressDeniesAll(t *testing.T) {
	base := model.Snapshot{Image: "app:v1"}
	candidate := model.Snapshot{Image: "app:v2", RuntimeEvents: []model.RuntimeEvent{{Category: "network", Operation: "connect", Target: "192.0.2.5:53", Protocol: "udp", Direction: "outbound"}}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}

	got := ApplyKubernetes(comparison, base, candidate, testKubernetesTarget(testDenyAllPolicy("default-deny-egress")))
	if got.Verdict != "BREAKING" {
		t.Fatalf("deny-all egress should block new dependency: %#v", got.Changes)
	}
}

func TestApplyKubernetesNetworkPolicyCombinesPoliciesAdditively(t *testing.T) {
	base := model.Snapshot{Image: "app:v1"}
	candidate := model.Snapshot{Image: "app:v2", RuntimeEvents: []model.RuntimeEvent{{Category: "network", Operation: "connect", Target: "203.0.113.10:443", Protocol: "tcp", Direction: "outbound"}}}
	comparison := model.Comparison{Baseline: base.Image, Candidate: candidate.Image, Verdict: "CHANGED"}
	deny := testDenyAllPolicy("deny-by-omission")
	allow := testIPBlockPolicy("allow-public", "203.0.113.0/24", "443")

	got := ApplyKubernetes(comparison, base, candidate, testKubernetesTarget(deny, allow))
	if got.Verdict == "BREAKING" {
		t.Fatalf("egress policies are additive; one allow must permit the dependency: %#v", got.Changes)
	}
}

func testKubernetesTarget(policies ...target.KubernetesNetworkPolicy) target.KubernetesTarget {
	return target.KubernetesTarget{Kind: "Deployment", Name: "api", Namespace: "default", Container: "api", PodLabels: map[string]string{"app": "api"}, NetworkPolicies: policies}
}

func testDenyAllPolicy(name string) target.KubernetesNetworkPolicy {
	return target.KubernetesNetworkPolicy{Name: name, Namespace: "default", EgressIsolating: true, PodSelector: target.KubernetesLabelSelector{MatchLabels: map[string]string{"app": "api"}}}
}

func testIPBlockPolicy(name, cidr, port string) target.KubernetesNetworkPolicy {
	policy := testDenyAllPolicy(name)
	policy.Egress = []target.KubernetesEgressRule{{To: []target.KubernetesNetworkPolicyPeer{{IPBlock: &target.KubernetesIPBlock{CIDR: cidr}}}, Ports: []target.KubernetesNetworkPolicyPort{{Protocol: "TCP", Port: port}}}}
	return policy
}
