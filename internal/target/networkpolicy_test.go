package target

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadKubernetesNetworkPolicyEgress(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "networkpolicy.json")
	data := `{
	  "apiVersion":"networking.k8s.io/v1",
	  "kind":"NetworkPolicy",
	  "metadata":{"name":"api-egress","namespace":"payments"},
	  "spec":{
	    "podSelector":{
	      "matchLabels":{"app":"api"},
	      "matchExpressions":[{"key":"track","operator":"NotIn","values":["legacy"]}]
	    },
	    "policyTypes":["Egress"],
	    "egress":[{
	      "to":[{"ipBlock":{"cidr":"10.0.0.0/8","except":["10.42.0.0/16"]}}],
	      "ports":[{"protocol":"TCP","port":443,"endPort":445}]
	    }]
	  }
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	policies, err := LoadKubernetesNetworkPolicies(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(policies) != 1 {
		t.Fatalf("policies=%d want 1", len(policies))
	}
	p := policies[0]
	if p.Name != "api-egress" || p.Namespace != "payments" || !p.EgressIsolating {
		t.Fatalf("unexpected policy identity: %#v", p)
	}
	if !p.PodSelector.Matches(map[string]string{"app": "api"}) {
		t.Fatal("NotIn should match when the label key is absent")
	}
	if p.PodSelector.Matches(map[string]string{"app": "api", "track": "legacy"}) {
		t.Fatal("selector should reject excluded track")
	}
	if len(p.Egress) != 1 || len(p.Egress[0].To) != 1 || len(p.Egress[0].Ports) != 1 {
		t.Fatalf("unexpected egress rules: %#v", p.Egress)
	}
	block := p.Egress[0].To[0].IPBlock
	if block == nil || block.CIDR != "10.0.0.0/8" || len(block.Except) != 1 || block.Except[0] != "10.42.0.0/16" {
		t.Fatalf("unexpected ipBlock: %#v", block)
	}
	port := p.Egress[0].Ports[0]
	if port.Protocol != "TCP" || port.Port != "443" || port.EndPort != 445 {
		t.Fatalf("unexpected port: %#v", port)
	}
}

func TestNetworkPolicyDefaultEgressIsolation(t *testing.T) {
	obj := map[string]any{
		"kind": "NetworkPolicy",
		"metadata": map[string]any{"name": "implicit-egress"},
		"spec": map[string]any{
			"podSelector": map[string]any{},
			"egress":      []any{map[string]any{}},
		},
	}
	p, err := parseKubernetesNetworkPolicy(obj)
	if err != nil {
		t.Fatal(err)
	}
	if !p.EgressIsolating {
		t.Fatal("presence of egress rules should default policyTypes to include Egress")
	}

	denyAll := map[string]any{
		"kind": "NetworkPolicy",
		"metadata": map[string]any{"name": "deny-all-egress"},
		"spec": map[string]any{
			"podSelector": map[string]any{},
			"policyTypes": []any{"Egress"},
		},
	}
	p, err = parseKubernetesNetworkPolicy(denyAll)
	if err != nil {
		t.Fatal(err)
	}
	if !p.EgressIsolating || len(p.Egress) != 0 {
		t.Fatalf("expected explicit Egress policy with no rules to isolate and deny all: %#v", p)
	}
}

func TestKubernetesLabelSelectorExpressions(t *testing.T) {
	selector := KubernetesLabelSelector{
		MatchLabels: map[string]string{"app": "api"},
		MatchExpressions: []KubernetesLabelExpression{
			{Key: "environment", Operator: "In", Values: []string{"prod", "staging"}},
			{Key: "debug", Operator: "DoesNotExist"},
			{Key: "owner", Operator: "Exists"},
		},
	}
	if !selector.Matches(map[string]string{"app": "api", "environment": "prod", "owner": "platform"}) {
		t.Fatal("selector should match valid labels")
	}
	if selector.Matches(map[string]string{"app": "api", "environment": "dev", "owner": "platform"}) {
		t.Fatal("selector should reject environment outside In set")
	}
	if selector.Matches(map[string]string{"app": "api", "environment": "prod", "owner": "platform", "debug": "true"}) {
		t.Fatal("DoesNotExist should reject present key")
	}
}
