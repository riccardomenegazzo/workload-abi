package evidence

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

const maxEventLine = 4 * 1024 * 1024

func Load(path, format string) ([]model.RuntimeEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open runtime evidence: %w", err)
	}
	defer f.Close()
	return Decode(f, format)
}

func Decode(r io.Reader, format string) ([]model.RuntimeEvent, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "generic", "wabi":
		return decodeGeneric(r)
	case "falco":
		return decodeRecords(r, normalizeFalco)
	case "tracee":
		return decodeRecords(r, normalizeTracee)
	default:
		return nil, fmt.Errorf("unsupported runtime evidence format %q", format)
	}
}

func Merge(snapshot model.Snapshot, events []model.RuntimeEvent) model.Snapshot {
	all := append(append([]model.RuntimeEvent(nil), snapshot.RuntimeEvents...), events...)
	snapshot.RuntimeEvents = dedupe(all)
	snapshot.SchemaVersion = model.SchemaVersion
	snapshot.Fingerprint = model.Fingerprint(snapshot)
	return snapshot
}

func dedupe(events []model.RuntimeEvent) []model.RuntimeEvent {
	byKey := map[string]model.RuntimeEvent{}
	for _, event := range events {
		event = normalizeFields(event)
		if event.Category == "" || event.Operation == "" {
			continue
		}
		key := event.SemanticKey()
		if old, ok := byKey[key]; ok {
			// Keep diagnostics when the newer record has more context without
			// making provider-specific metadata part of semantic identity.
			if old.Source == "" {
				old.Source = event.Source
			}
			if old.PID == 0 {
				old.PID = event.PID
			}
			if old.Rule == "" {
				old.Rule = event.Rule
			}
			byKey[key] = old
			continue
		}
		byKey[key] = event
	}
	out := make([]model.RuntimeEvent, 0, len(byKey))
	for _, event := range byKey {
		out = append(out, event)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SemanticKey() < out[j].SemanticKey() })
	return out
}

func normalizeFields(event model.RuntimeEvent) model.RuntimeEvent {
	event.Source = strings.TrimSpace(event.Source)
	event.Category = strings.ToLower(strings.TrimSpace(event.Category))
	event.Operation = strings.ToLower(strings.TrimSpace(event.Operation))
	event.Process = strings.TrimSpace(event.Process)
	event.ParentProcess = strings.TrimSpace(event.ParentProcess)
	event.Target = strings.TrimSpace(event.Target)
	event.Protocol = strings.ToLower(strings.TrimSpace(event.Protocol))
	event.Direction = strings.ToLower(strings.TrimSpace(event.Direction))
	event.Rule = strings.TrimSpace(event.Rule)
	return event
}

func decodeGeneric(r io.Reader) ([]model.RuntimeEvent, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil
	}
	if data[0] == '[' {
		var events []model.RuntimeEvent
		if err := json.Unmarshal(data, &events); err != nil {
			return nil, fmt.Errorf("parse generic event array: %w", err)
		}
		for i := range events {
			if events[i].Source == "" {
				events[i].Source = "generic"
			}
		}
		return dedupe(events), nil
	}
	return decodeRecords(bytes.NewReader(data), func(raw map[string]any) (model.RuntimeEvent, bool) {
		encoded, err := json.Marshal(raw)
		if err != nil {
			return model.RuntimeEvent{}, false
		}
		var event model.RuntimeEvent
		if err := json.Unmarshal(encoded, &event); err != nil {
			return model.RuntimeEvent{}, false
		}
		if event.Source == "" {
			event.Source = "generic"
		}
		return event, event.Category != "" && event.Operation != ""
	})
}

func decodeRecords(r io.Reader, normalize func(map[string]any) (model.RuntimeEvent, bool)) ([]model.RuntimeEvent, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), maxEventLine)
	var events []model.RuntimeEvent
	line := 0
	for scanner.Scan() {
		line++
		text := bytes.TrimSpace(scanner.Bytes())
		if len(text) == 0 {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(text, &raw); err != nil {
			return nil, fmt.Errorf("parse runtime evidence line %d: %w", line, err)
		}
		if event, ok := normalize(raw); ok {
			events = append(events, event)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read runtime evidence: %w", err)
	}
	return dedupe(events), nil
}

func normalizeFalco(raw map[string]any) (model.RuntimeEvent, bool) {
	fields := mapValue(raw["output_fields"])
	eventName := firstString(fields, "syscall.type", "evt.type")
	category, operation, direction := classify(eventName)
	if operation == "" {
		return model.RuntimeEvent{}, false
	}

	event := model.RuntimeEvent{
		Source:        "falco",
		Category:      category,
		Operation:     operation,
		Direction:     direction,
		Process:       firstString(fields, "proc.exepath", "proc.name"),
		ParentProcess: firstString(fields, "proc.pexepath", "proc.pname"),
		PID:           firstInt(fields, "proc.pid"),
		Rule:          stringValue(raw["rule"]),
	}

	switch category {
	case "file":
		event.Target = firstString(fields,
			"fs.path.name", "fd.name", "evt.arg.pathname", "evt.arg.filename",
			"evt.arg.path", "evt.arg.name", "evt.arg.newpath", "evt.arg.oldpath")
	case "network":
		event.Protocol = firstString(fields, "fd.l4proto", "fd.rproto", "fd.lproto", "fd.sproto")
		host := firstString(fields, "fd.rip.name", "fd.rip", "fd.sip.name", "fd.sip")
		port := firstString(fields, "fd.rport", "fd.sport")
		if host != "" && port != "" {
			event.Target = host + ":" + port
		} else if host != "" {
			event.Target = host
		} else {
			event.Target = firstString(fields, "fd.name")
		}
	}
	return normalizeFields(event), true
}

func normalizeTracee(raw map[string]any) (model.RuntimeEvent, bool) {
	eventName := stringValue(raw["eventName"])
	if eventName == "" {
		eventName = stringValue(raw["syscall"])
	}
	category, operation, direction := classify(eventName)
	if operation == "" {
		return model.RuntimeEvent{}, false
	}
	args := traceeArgs(raw["args"])
	event := model.RuntimeEvent{
		Source:    "tracee",
		Category:  category,
		Operation: operation,
		Direction: direction,
		Process:   traceeProcess(raw),
		PID:       intValue(raw["processId"]),
	}
	switch category {
	case "file":
		event.Target = firstArg(args, "pathname", "path", "filename", "file", "oldname", "newname", "newpath", "oldpath")
	case "network":
		event.Protocol = strings.ToLower(firstArg(args, "protocol", "proto", "family"))
		event.Target = firstArg(args, "remote_addr", "remote", "sockaddr", "address", "addr", "dst", "destination")
	}
	return normalizeFields(event), true
}

func classify(name string) (category, operation, direction string) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "", "", ""
	}
	switch name {
	case "execve", "execveat", "sched_process_exec":
		return "process", "exec", ""
	case "fork", "vfork", "clone", "clone3", "sched_process_fork":
		return "process", "spawn", ""
	case "open", "openat", "openat2", "creat", "security_file_open":
		return "file", "open", ""
	case "read", "pread64", "readv", "preadv", "preadv2":
		return "file", "read", ""
	case "write", "pwrite64", "writev", "pwritev", "pwritev2":
		return "file", "write", ""
	case "unlink", "unlinkat":
		return "file", "unlink", ""
	case "rename", "renameat", "renameat2":
		return "file", "rename", ""
	case "mkdir", "mkdirat":
		return "file", "mkdir", ""
	case "rmdir":
		return "file", "rmdir", ""
	case "connect":
		return "network", "connect", "outbound"
	case "accept", "accept4":
		return "network", "accept", "inbound"
	case "bind":
		return "network", "bind", "inbound"
	case "listen":
		return "network", "listen", "inbound"
	case "sendto", "sendmsg", "sendmmsg":
		return "network", "send", "outbound"
	case "recvfrom", "recvmsg", "recvmmsg":
		return "network", "receive", "inbound"
	case "socket", "socketpair":
		return "network", "socket", ""
	case "net_packet_dns", "dns_request", "dns_query":
		return "network", "dns", "outbound"
	default:
		return "syscall", name, ""
	}
}

func traceeProcess(raw map[string]any) string {
	if executable := mapValue(raw["executable"]); len(executable) > 0 {
		if path := stringValue(executable["path"]); path != "" {
			return path
		}
	}
	return stringValue(raw["processName"])
}

func traceeArgs(v any) map[string]string {
	out := map[string]string{}
	for _, item := range sliceValue(v) {
		arg := mapValue(item)
		name := stringValue(arg["name"])
		if name == "" {
			continue
		}
		out[name] = stableValue(arg["value"])
	}
	return out
}

func firstArg(args map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(args[key]); value != "" {
			return value
		}
	}
	return ""
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(stringValue(values[key])); value != "" {
			return value
		}
	}
	return ""
}

func firstInt(values map[string]any, keys ...string) int {
	for _, key := range keys {
		if n := intValue(values[key]); n != 0 {
			return n
		}
	}
	return 0
}

func stringValue(v any) string {
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
	case bool:
		return strconv.FormatBool(x)
	default:
		return ""
	}
}

func intValue(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case json.Number:
		n, _ := strconv.Atoi(x.String())
		return n
	case string:
		n, _ := strconv.Atoi(x)
		return n
	default:
		return 0
	}
}

func stableValue(v any) string {
	if scalar := stringValue(v); scalar != "" {
		return scalar
	}
	if v == nil {
		return ""
	}
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}

func mapValue(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func sliceValue(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}
