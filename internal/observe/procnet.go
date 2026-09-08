package observe

import (
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

var tcpStates = map[string]string{
	"01": "ESTABLISHED", "02": "SYN_SENT", "03": "SYN_RECV", "04": "FIN_WAIT1",
	"05": "FIN_WAIT2", "06": "TIME_WAIT", "07": "CLOSE", "08": "CLOSE_WAIT",
	"09": "LAST_ACK", "0A": "LISTEN", "0B": "CLOSING",
}

func ParseProcNet(content, protocol string, ipv6 bool) ([]model.NetworkEndpoint, error) {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) <= 1 {
		return nil, nil
	}
	var out []model.NetworkEndpoint
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		localAddr, localPort, err := parseHexAddrPort(fields[1], ipv6)
		if err != nil {
			continue
		}
		remoteAddr, remotePort, err := parseHexAddrPort(fields[2], ipv6)
		if err != nil {
			continue
		}
		state := tcpStates[fields[3]]
		if protocol == "udp" {
			state = fields[3]
		}
		direction := "outbound"
		if (protocol == "tcp" && fields[3] == "0A") || (protocol == "udp" && remotePort == 0) {
			direction = "listen"
		}
		if direction == "outbound" && (remotePort == 0 || remoteAddr == "0.0.0.0" || remoteAddr == "::") {
			continue
		}
		out = append(out, model.NetworkEndpoint{
			Protocol: protocol, Direction: direction, LocalAddress: localAddr, LocalPort: localPort,
			RemoteAddress: remoteAddr, RemotePort: remotePort, State: state,
		})
	}
	return out, nil
}

func parseHexAddrPort(s string, ipv6 bool) (string, int, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid address %q", s)
	}
	p, err := strconv.ParseInt(parts[1], 16, 32)
	if err != nil {
		return "", 0, err
	}
	b, err := hex.DecodeString(parts[0])
	if err != nil {
		return "", 0, err
	}
	if ipv6 {
		if len(b) != 16 {
			return "", 0, fmt.Errorf("invalid ipv6 bytes")
		}
		for i := 0; i < 16; i += 4 {
			b[i], b[i+3] = b[i+3], b[i]
			b[i+1], b[i+2] = b[i+2], b[i+1]
		}
		return net.IP(b).String(), int(p), nil
	}
	if len(b) != 4 {
		return "", 0, fmt.Errorf("invalid ipv4 bytes")
	}
	return net.IPv4(b[3], b[2], b[1], b[0]).String(), int(p), nil
}
