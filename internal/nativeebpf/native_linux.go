//go:build linux

package nativeebpf

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

const (
	rawEventSize = 160
	commOffset   = 8
	commSize     = 16
	payloadOffset = 24
	payloadSize   = 128

	kindExec    = 1
	kindOpenat  = 2
	kindConnect = 3
)

type attachedProbe struct {
	name string
	prog *ebpf.Program
	link link.Link
}

func record(ctx context.Context, opts Options) (Result, error) {
	if opts.Duration <= 0 {
		opts.Duration = 3 * time.Second
	}
	if opts.MaxEvents <= 0 {
		opts.MaxEvents = 10000
	}

	started := time.Now()
	result := Result{}
	if err := rlimit.RemoveMemlock(); err != nil {
		return result, fmt.Errorf("remove eBPF memlock limit: %w", err)
	}

	events, err := ebpf.NewMap(&ebpf.MapSpec{
		Name: "wabi_events",
		Type: ebpf.PerfEventArray,
	})
	if err != nil {
		return result, fmt.Errorf("create native eBPF event map: %w", err)
	}
	defer events.Close()

	probes := make([]attachedProbe, 0, 3)
	attach := func(name, group, event string, kind int32, field string) {
		var offset int16
		if field != "" {
			value, fieldErr := tracepointFieldOffset(group, event, field)
			if fieldErr != nil {
				result.Stats.Probes = append(result.Stats.Probes, ProbeStatus{Name: name, Error: fieldErr.Error()})
				return
			}
			offset = value
		}
		prog, progErr := newTracepointProgram(name, kind, offset, events)
		if progErr != nil {
			result.Stats.Probes = append(result.Stats.Probes, ProbeStatus{Name: name, Error: progErr.Error()})
			return
		}
		tp, linkErr := link.Tracepoint(group, event, prog, nil)
		if linkErr != nil {
			prog.Close()
			result.Stats.Probes = append(result.Stats.Probes, ProbeStatus{Name: name, Error: linkErr.Error()})
			return
		}
		probes = append(probes, attachedProbe{name: name, prog: prog, link: tp})
		result.Stats.Probes = append(result.Stats.Probes, ProbeStatus{Name: name, Available: true})
	}

	attach("process-exec", "sched", "sched_process_exec", kindExec, "")
	attach("file-openat", "syscalls", "sys_enter_openat", kindOpenat, "filename")
	attach("network-connect", "syscalls", "sys_enter_connect", kindConnect, "uservaddr")

	defer func() {
		for _, probe := range probes {
			probe.link.Close()
			probe.prog.Close()
		}
	}()
	if len(probes) == 0 {
		return result, fmt.Errorf("no native eBPF probes could be attached")
	}

	rd, err := perf.NewReader(events, os.Getpagesize()*8)
	if err != nil {
		return result, fmt.Errorf("create native eBPF perf reader: %w", err)
	}
	defer rd.Close()

	deadline := time.NewTimer(opts.Duration)
	defer deadline.Stop()
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			rd.Close()
		case <-deadline.C:
			rd.Close()
		case <-stop:
		}
	}()
	defer close(stop)

	seen := make(map[string]struct{})
	for len(result.Events) < opts.MaxEvents {
		record, readErr := rd.Read()
		if readErr != nil {
			if errors.Is(readErr, perf.ErrClosed) {
				break
			}
			return result, fmt.Errorf("read native eBPF event: %w", readErr)
		}
		result.Stats.LostSamples += record.LostSamples
		if len(record.RawSample) < rawEventSize {
			continue
		}
		event := decodeEvent(record.RawSample[:rawEventSize])
		if event.Category == "" || event.Operation == "" {
			continue
		}
		key := event.SemanticKey()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result.Events = append(result.Events, event)
	}
	if len(result.Events) >= opts.MaxEvents {
		rd.Close()
	}
	result.Stats.Captured = len(result.Events)
	result.Stats.Duration = time.Since(started)
	return result, nil
}

func newTracepointProgram(name string, kind int32, contextOffset int16, events *ebpf.Map) (*ebpf.Program, error) {
	insns := asm.Instructions{
		asm.Mov.Reg(asm.R7, asm.R1),
		asm.Mov.Reg(asm.R6, asm.RFP),
		asm.Add.Imm(asm.R6, -rawEventSize),
	}
	for offset := 0; offset < rawEventSize; offset += 8 {
		insns = append(insns, asm.StoreImm(asm.R6, int16(offset), 0, asm.DWord))
	}
	insns = append(insns,
		asm.StoreImm(asm.R6, 0, int64(kind), asm.Word),
		asm.FnGetCurrentPidTgid.Call(),
		asm.RSh.Imm(asm.R0, 32),
		asm.StoreMem(asm.R6, 4, asm.R0, asm.Word),
		asm.Mov.Reg(asm.R1, asm.R6),
		asm.Add.Imm(asm.R1, commOffset),
		asm.Mov.Imm(asm.R2, commSize),
		asm.FnGetCurrentComm.Call(),
	)

	switch kind {
	case kindOpenat:
		insns = append(insns,
			asm.LoadMem(asm.R3, asm.R7, contextOffset, asm.DWord),
			asm.Mov.Reg(asm.R1, asm.R6),
			asm.Add.Imm(asm.R1, payloadOffset),
			asm.Mov.Imm(asm.R2, payloadSize),
			asm.FnProbeReadUserStr.Call(),
		)
	case kindConnect:
		insns = append(insns,
			asm.LoadMem(asm.R3, asm.R7, contextOffset, asm.DWord),
			asm.Mov.Reg(asm.R1, asm.R6),
			asm.Add.Imm(asm.R1, payloadOffset),
			asm.Mov.Imm(asm.R2, 28),
			asm.FnProbeReadUser.Call(),
		)
	}

	insns = append(insns,
		asm.Mov.Reg(asm.R1, asm.R7),
		asm.LoadMapPtr(asm.R2, events.FD()),
		asm.LoadImm(asm.R3, 0xffffffff, asm.DWord),
		asm.Mov.Reg(asm.R4, asm.R6),
		asm.Mov.Imm(asm.R5, rawEventSize),
		asm.FnPerfEventOutput.Call(),
		asm.Mov.Imm(asm.R0, 0),
		asm.Return(),
	)

	return ebpf.NewProgram(&ebpf.ProgramSpec{
		Name:         "wabi_" + strings.ReplaceAll(name, "-", "_"),
		Type:         ebpf.TracePoint,
		License:      "GPL",
		Instructions: insns,
	})
}

func tracepointFieldOffset(group, event, field string) (int16, error) {
	roots := []string{"/sys/kernel/tracing", "/sys/kernel/debug/tracing"}
	var lastErr error
	for _, root := range roots {
		path := filepath.Join(root, "events", group, event, "format")
		data, err := os.ReadFile(path)
		if err != nil {
			lastErr = err
			continue
		}
		return parseFieldOffset(string(data), field)
	}
	return 0, fmt.Errorf("read tracepoint format for %s/%s: %w", group, event, lastErr)
}

func decodeEvent(raw []byte) model.RuntimeEvent {
	kind := binary.LittleEndian.Uint32(raw[0:4])
	pid := int(binary.LittleEndian.Uint32(raw[4:8]))
	comm := cString(raw[commOffset : commOffset+commSize])
	payload := raw[payloadOffset : payloadOffset+payloadSize]
	parent := parentProcess(pid)

	switch kind {
	case kindExec:
		return model.RuntimeEvent{Source: "native-ebpf", Category: "process", Operation: "exec", Process: comm, ParentProcess: parent, PID: pid}
	case kindOpenat:
		target := cString(payload)
		return model.RuntimeEvent{Source: "native-ebpf", Category: "file", Operation: "open", Process: comm, ParentProcess: parent, Target: target, PID: pid}
	case kindConnect:
		target, protocol := socketTarget(payload)
		return model.RuntimeEvent{Source: "native-ebpf", Category: "network", Operation: "connect", Process: comm, ParentProcess: parent, Target: target, Protocol: protocol, Direction: "outbound", PID: pid}
	default:
		return model.RuntimeEvent{}
	}
}

func cString(data []byte) string {
	if i := bytes.IndexByte(data, 0); i >= 0 {
		data = data[:i]
	}
	return strings.TrimSpace(string(data))
}

func socketTarget(data []byte) (string, string) {
	if len(data) < 8 {
		return "", "tcp"
	}
	family := binary.LittleEndian.Uint16(data[0:2])
	port := binary.BigEndian.Uint16(data[2:4])
	switch family {
	case 2:
		return net.JoinHostPort(net.IP(data[4:8]).String(), strconv.Itoa(int(port))), "tcp"
	case 10:
		if len(data) < 24 {
			return "", "tcp"
		}
		return net.JoinHostPort(net.IP(data[8:24]).String(), strconv.Itoa(int(port))), "tcp"
	default:
		return "", "tcp"
	}
}

func parentProcess(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}
	text := string(data)
	end := strings.LastIndex(text, ")")
	if end < 0 || end+2 >= len(text) {
		return ""
	}
	fields := strings.Fields(text[end+2:])
	if len(fields) < 2 {
		return ""
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil || ppid <= 0 {
		return ""
	}
	comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", ppid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(comm))
}
