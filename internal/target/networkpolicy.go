package target

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type KubernetesNetworkPolicy struct {
	Name            string
	Namespace       string
	PodSelector     KubernetesLabelSelector
	EgressIsolating bool
	Egress          []KubernetesEgressRule
}

type KubernetesLabelSelector struct {
	MatchLabels      map[string]string
	MatchExpressions []KubernetesLabelExpression
}

type KubernetesLabelExpression struct {
	Key      string
	Operator string
	Values   []string
}

type KubernetesEgressRule struct {
	To    []KubernetesNetworkPolicyPeer
	Ports []KubernetesNetworkPolicyPort
}

type KubernetesNetworkPolicyPeer struct {
	IPBlock           *KubernetesIPBlock
	PodSelector       *KubernetesLabelSelector
	NamespaceSelector *KubernetesLabelSelector
}

type KubernetesIPBlock struct {
	CIDR   string
	Except []string
}

type KubernetesNetworkPolicyPort struct {
	Protocol string
	Port     string
	EndPort  int
}

func LoadKubernetesNetworkPolicies(ctx context.Context, file string) ([]KubernetesNetworkPolicy, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read kubernetes NetworkPolicy target: %w", err)
	}
	data, err = kubernetesJSON(ctx, file, data)
	if err != nil {
		return nil, err
	}

	var root any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("parse kubernetes NetworkPolicy target: %w", err)
	}

	var objects []map[string]any
	obj := mapValue(root)
	if strings.EqualFold(stringValue(obj["kind"]), "List") {
		for _, raw := range sliceValue(obj["items"]) {
			objects = append(objects, mapValue(raw))
		}
	} else {
		objects = append(objects, obj)
	}

	var out []KubernetesNetworkPolicy
	for _, item := range objects {
		if !strings.EqualFold(stringValue(item["kind"]), "NetworkPolicy") {
			continue
		}
		p, err := parseKubernetesNetworkPolicy(item)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("kubernetes NetworkPolicy file contains no NetworkPolicy objects")
	}
	return out, nil
}

func parseKubernetesNetworkPolicy(obj map[string]any) (KubernetesNetworkPolicy, error) {
	meta := mapValue(obj["metadata"])
	spec := mapValue(obj["spec"])
	p := KubernetesNetworkPolicy{
		Name:        stringValue(meta["name"]),
		Namespace:   defaultNamespace(stringValue(meta["namespace"])),
		PodSelector: parseKubernetesLabelSelector(mapValue(spec["podSelector"])),
	}
	if p.Name == "" {
		return p, fmt.Errorf("kubernetes NetworkPolicy is missing metadata.name")
	}

	policyTypes := stringSlice(spec["policyTypes"])
	if len(policyTypes) == 0 {
		_, p.EgressIsolating = spec["egress"]
	} else {
		for _, policyType := range policyTypes {
			if strings.EqualFold(policyType, "Egress") {
				p.EgressIsolating = true
				break
			}
		}
	}

	for _, rawRule := range sliceValue(spec["egress"]) {
		ruleMap := mapValue(rawRule)
		rule := KubernetesEgressRule{}
		for _, rawPeer := range sliceValue(ruleMap["to"]) {
			peerMap := mapValue(rawPeer)
			peer := KubernetesNetworkPolicyPeer{}
			if raw, ok := peerMap["ipBlock"]; ok {
				ipBlock := mapValue(raw)
				peer.IPBlock = &KubernetesIPBlock{
					CIDR:   stringValue(ipBlock["cidr"]),
					Except: stringSlice(ipBlock["except"]),
				}
			}
			if raw, ok := peerMap["podSelector"]; ok {
				selector := parseKubernetesLabelSelector(mapValue(raw))
				peer.PodSelector = &selector
			}
			if raw, ok := peerMap["namespaceSelector"]; ok {
				selector := parseKubernetesLabelSelector(mapValue(raw))
				peer.NamespaceSelector = &selector
			}
			rule.To = append(rule.To, peer)
		}
		for _, rawPort := range sliceValue(ruleMap["ports"]) {
			portMap := mapValue(rawPort)
			port := KubernetesNetworkPolicyPort{
				Protocol: strings.ToUpper(strings.TrimSpace(stringValue(portMap["protocol"]))),
				Port:     scalarString(portMap["port"]),
			}
			if port.Protocol == "" {
				port.Protocol = "TCP"
			}
			if end, ok := intValue(portMap["endPort"]); ok && end > 0 {
				port.EndPort = int(end)
			}
			rule.Ports = append(rule.Ports, port)
		}
		p.Egress = append(p.Egress, rule)
	}
	return p, nil
}

func parseKubernetesLabelSelector(m map[string]any) KubernetesLabelSelector {
	selector := KubernetesLabelSelector{MatchLabels: map[string]string{}}
	for key, raw := range mapValue(m["matchLabels"]) {
		if value := scalarString(raw); value != "" {
			selector.MatchLabels[key] = value
		}
	}
	for _, raw := range sliceValue(m["matchExpressions"]) {
		expr := mapValue(raw)
		selector.MatchExpressions = append(selector.MatchExpressions, KubernetesLabelExpression{
			Key:      stringValue(expr["key"]),
			Operator: stringValue(expr["operator"]),
			Values:   stringSlice(expr["values"]),
		})
	}
	return selector
}

func (s KubernetesLabelSelector) Matches(labels map[string]string) bool {
	for key, expected := range s.MatchLabels {
		if labels[key] != expected {
			return false
		}
	}
	for _, expr := range s.MatchExpressions {
		value, exists := labels[expr.Key]
		switch strings.ToLower(expr.Operator) {
		case "in":
			if !exists || !containsString(expr.Values, value) {
				return false
			}
		case "notin":
			if !exists || containsString(expr.Values, value) {
				return false
			}
		case "exists":
			if !exists {
				return false
			}
		case "doesnotexist":
			if exists {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func stringMap(v any) map[string]string {
	out := map[string]string{}
	for key, raw := range mapValue(v) {
		if value := scalarString(raw); value != "" {
			out[key] = value
		}
	}
	return out
}

func scalarString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	default:
		return ""
	}
}

func defaultNamespace(namespace string) string {
	if strings.TrimSpace(namespace) == "" {
		return "default"
	}
	return namespace
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
