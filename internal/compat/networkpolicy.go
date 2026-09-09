package compat

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
	"github.com/riccardomenegazzo/workload-abi/internal/target"
)

type networkDecision uint8

const (
	networkDeny networkDecision = iota
	networkAllow
	networkUnknown
)

func applyKubernetesNetworkPolicies(c *model.Comparison, base, candidate model.Snapshot, t target.KubernetesTarget) {
	policies := selectingEgressPolicies(t)
	if len(policies) == 0 {
		return
	}

	baseEvents := make(map[string]struct{}, len(base.RuntimeEvents))
	for _, event := range base.RuntimeEvents {
		baseEvents[event.SemanticKey()] = struct{}{}
	}

	policyNames := make([]string, 0, len(policies))
	for _, policy := range policies {
		policyNames = append(policyNames, policy.Name)
	}
	sort.Strings(policyNames)

	for _, event := range candidate.RuntimeEvents {
		if !isOutboundConnect(event) {
			continue
		}
		if _, existed := baseEvents[event.SemanticKey()]; existed {
			continue
		}

		if evaluateEgressPolicies(event, policies) != networkDeny {
			continue
		}
		breaking(c, "runtime-network", "target-conflict", runtimeEventDisplay(event),
			fmt.Sprintf("candidate introduces a new outbound dependency denied by Kubernetes NetworkPolicy egress rules selecting this workload (%s)", strings.Join(policyNames, ", ")))
	}
}

func selectingEgressPolicies(t target.KubernetesTarget) []target.KubernetesNetworkPolicy {
	var out []target.KubernetesNetworkPolicy
	for _, policy := range t.NetworkPolicies {
		if !policy.EgressIsolating || policy.Namespace != t.Namespace {
			continue
		}
		if !policy.PodSelector.Matches(t.PodLabels) {
			continue
		}
		out = append(out, policy)
	}
	return out
}

func evaluateEgressPolicies(event model.RuntimeEvent, policies []target.KubernetesNetworkPolicy) networkDecision {
	unknown := false
	for _, policy := range policies {
		for _, rule := range policy.Egress {
			switch evaluateEgressRule(event, rule) {
			case networkAllow:
				return networkAllow
			case networkUnknown:
				unknown = true
			}
		}
	}
	if unknown {
		return networkUnknown
	}
	return networkDeny
}

func evaluateEgressRule(event model.RuntimeEvent, rule target.KubernetesEgressRule) networkDecision {
	destination := evaluateDestination(event, rule.To)
	ports := evaluatePorts(event, rule.Ports)
	if destination == networkDeny || ports == networkDeny {
		return networkDeny
	}
	if destination == networkAllow && ports == networkAllow {
		return networkAllow
	}
	return networkUnknown
}

func evaluateDestination(event model.RuntimeEvent, peers []target.KubernetesNetworkPolicyPeer) networkDecision {
	if len(peers) == 0 {
		return networkAllow
	}
	host, _, _ := splitRuntimeEndpoint(event.Target)
	ip := net.ParseIP(strings.Trim(host, "[]"))
	unknown := false

	for _, peer := range peers {
		if peer.IPBlock != nil {
			if ip == nil {
				unknown = true
				continue
			}
			if ipBlockContains(*peer.IPBlock, ip) {
				return networkAllow
			}
			continue
		}
		if peer.PodSelector != nil || peer.NamespaceSelector != nil {
			// RuntimeEvent currently knows the observed endpoint, not the
			// destination Pod/Namespace identity required to prove a selector.
			unknown = true
			continue
		}
		unknown = true
	}
	if unknown {
		return networkUnknown
	}
	return networkDeny
}

func evaluatePorts(event model.RuntimeEvent, ports []target.KubernetesNetworkPolicyPort) networkDecision {
	if len(ports) == 0 {
		return networkAllow
	}
	_, portText, hasPort := splitRuntimeEndpoint(event.Target)
	eventProtocol := strings.ToUpper(strings.TrimSpace(event.Protocol))
	unknown := false

	for _, policyPort := range ports {
		protocol := strings.ToUpper(strings.TrimSpace(policyPort.Protocol))
		if protocol == "" {
			protocol = "TCP"
		}
		if eventProtocol == "" {
			unknown = true
		} else if eventProtocol != protocol {
			continue
		}

		if strings.TrimSpace(policyPort.Port) == "" {
			if eventProtocol == "" {
				unknown = true
				continue
			}
			return networkAllow
		}

		start, err := strconv.Atoi(policyPort.Port)
		if err != nil {
			// Named ports require destination Pod/container metadata, which the
			// current runtime event does not contain.
			unknown = true
			continue
		}
		if !hasPort {
			unknown = true
			continue
		}
		observed, err := strconv.Atoi(portText)
		if err != nil {
			unknown = true
			continue
		}
		end := start
		if policyPort.EndPort >= start {
			end = policyPort.EndPort
		}
		if observed >= start && observed <= end {
			if eventProtocol == "" {
				unknown = true
				continue
			}
			return networkAllow
		}
	}
	if unknown {
		return networkUnknown
	}
	return networkDeny
}

func ipBlockContains(block target.KubernetesIPBlock, ip net.IP) bool {
	_, cidr, err := net.ParseCIDR(strings.TrimSpace(block.CIDR))
	if err != nil || !cidr.Contains(ip) {
		return false
	}
	for _, excluded := range block.Except {
		_, exceptCIDR, err := net.ParseCIDR(strings.TrimSpace(excluded))
		if err == nil && exceptCIDR.Contains(ip) {
			return false
		}
	}
	return true
}

func splitRuntimeEndpoint(value string) (host, port string, hasPort bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", false
	}
	if host, port, err := net.SplitHostPort(value); err == nil {
		return host, port, true
	}
	if ip := net.ParseIP(strings.Trim(value, "[]")); ip != nil {
		return strings.Trim(value, "[]"), "", false
	}
	// A hostname without an explicit port is still useful as an unknown
	// destination when the policy only contains IPBlock peers.
	return value, "", false
}

func isOutboundConnect(event model.RuntimeEvent) bool {
	return strings.EqualFold(event.Category, "network") &&
		strings.EqualFold(event.Operation, "connect") &&
		(strings.EqualFold(event.Direction, "outbound") || strings.TrimSpace(event.Direction) == "")
}

func runtimeEventDisplay(event model.RuntimeEvent) string {
	parts := []string{"connect"}
	if event.Process != "" {
		parts = append(parts, "process="+event.Process)
	}
	if event.Target != "" {
		parts = append(parts, "target="+event.Target)
	}
	if event.Protocol != "" {
		parts = append(parts, "protocol="+strings.ToLower(event.Protocol))
	}
	return strings.Join(parts, " ")
}
