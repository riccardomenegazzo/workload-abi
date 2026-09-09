package nativeebpf

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

func parseFieldOffset(format, field string) (int16, error) {
	s := bufio.NewScanner(strings.NewReader(format))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if !strings.HasPrefix(line, "field:") {
			continue
		}
		parts := strings.Split(line, ";")
		if len(parts) < 2 {
			continue
		}
		decl := strings.TrimSpace(strings.TrimPrefix(parts[0], "field:"))
		fields := strings.Fields(decl)
		if len(fields) == 0 {
			continue
		}
		name := strings.TrimLeft(fields[len(fields)-1], "*")
		if name != field {
			continue
		}
		for _, part := range parts[1:] {
			part = strings.TrimSpace(part)
			if !strings.HasPrefix(part, "offset:") {
				continue
			}
			value := strings.TrimSpace(strings.TrimPrefix(part, "offset:"))
			offset, err := strconv.ParseInt(value, 10, 16)
			if err != nil {
				return 0, fmt.Errorf("parse tracepoint field %s offset: %w", field, err)
			}
			return int16(offset), nil
		}
	}
	if err := s.Err(); err != nil {
		return 0, fmt.Errorf("scan tracepoint format: %w", err)
	}
	return 0, fmt.Errorf("tracepoint field %q not found", field)
}
