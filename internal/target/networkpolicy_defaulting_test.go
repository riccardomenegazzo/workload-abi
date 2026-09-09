package target

import "testing"

func TestNetworkPolicyEmptyEgressWithoutPolicyTypesDoesNotIsolate(t *testing.T) {
	obj := map[string]any{"kind": "NetworkPolicy", "metadata": map[string]any{"name": "empty-egress"}, "spec": map[string]any{"podSelector": map[string]any{}, "egress": []any{}}}
	p, err := parseKubernetesNetworkPolicy(obj)
	if err != nil {
		t.Fatal(err)
	}
	if p.EgressIsolating {
		t.Fatal("egress: [] without policyTypes must not default to Egress isolation")
	}
}
