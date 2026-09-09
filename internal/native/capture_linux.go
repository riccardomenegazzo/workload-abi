//go:build linux

package native

import (
	"bufio"
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
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/riccardomenegazzo/workload-abi/internal/model"
)

const (
	nativeEventSize = 48
	defaultRingSize = 1 << 20
	afINET          = 2
	afINET6         = 10
)

type rawConnectEvent struct {
	CgroupID uint64
	PIDTGID  uint64
	Family   uint32
	PortRaw  uint32
	Addr     [16]byte
	Protocol uint32
}

func capture(ctx context.Context, cfg Config) (Result, error) {
	started := time.Now().UTC()
	result := Result{Backend: "cgroup-ebpf", StartedAt: started}

	cgroupPath, err := resolveCgroupPath(cfg)
	if err != nil {
		return result, err
	}
	result.Cgroup = cgroupPath

	if cfg.Duration <= 0 {
		cfg.Duration = 5 * time.Second
	}
	if cfg.RingBytes == 0 {
		cfg.RingBytes = defaultRingSize
	}
	cfg.RingBytes = normalizeRingSize(cfg.RingBytes)

	if err := rlimit.RemoveMemlock(); err != nil {
		result.Warnings = append(result.Warnings, "could not raise memlock rlimit: "+err.Error())
	}

	events, err := ebpf.NewMap(&ebpf.MapSpec{
		Name:       "wabi_native_events",
		Type:       ebpf.RingBuf,
		MaxEntries: cfg.RingBytes,
	})
	if err != nil {
		return result, fmt.Errorf("create native event ring: %w", err)
	}
	defer events.Close()

	drops, err := ebpf.NewMap(&ebpf.MapSpec{
		Name:       "wabi_native_drops",
		Type:       ebpf.Array,
		KeySize:    4,
		ValueSize:  8,
		MaxEntries: 1,
	})
	if err != nil {
		return result, fmt.Errorf("create native drop counter: %w", err)
	}
	defer drops.Close()

	var attached []link.Link
	attach := func(attachType ebpf.AttachType, family uint32) error {
		prog, err := ebpf.NewProgram(connectProgramSpec(events, drops, attachType, family))
		if err != nil {
			return fmt.Errorf("load %s program: %w", attachType, err)
		}
		lnk, err := link.AttachCgroup(link.CgroupOptions{
			Path:    cgroupPath,
			Attach:  attachType,
			Program: prog,
		})
		if err != nil {
			prog.Close()
			return fmt.Errorf("attach %s program to %s: %w", attachType, cgroupPath, err)
		}
		attached = append(attached, &programLink{Link: lnk, program: prog})
		return nil
	}

	var attachErrors []string
	if err := attach(ebpf.AttachCGroupInet4Connect, afINET); err != nil {
		attachErrors = append(attachErrors, err.Error())
	}
	if err := attach(ebpf.AttachCGroupInet6Connect, afINET6); err != nil {
		attachErrors = append(attachErrors, err.Error())
	}
	if len(attached) == 0 {
		return result, fmt.Errorf("native recorder could not attach any connect hook: %s", strings.Join(attachErrors, "; "))
	}
	for _, message := range attachErrors {
		result.Warnings = append(result.Warnings, message)
	}
	defer func() {
		for _, lnk := range attached {
			_ = lnk.Close()
		}
	}()

	reader, err := ringbuf.NewReader(events)
	if err != nil {
		return result, fmt.Errorf("open native event ring: %w", err)
	}
	defer reader.Close()

	deadline := time.Now().Add(cfg.Duration)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	reader.SetDeadline(deadline)

	seen := map[string]model.RuntimeEvent{}
	for {
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, ringbuf.ErrClosed) || ctx.Err() != nil {
				break
			}
			return result, fmt.Errorf("read native event: %w", err)
		}
		event, err := decodeConnectEvent(record.RawSample)
		if err != nil {
			result.Warnings = append(result.Warnings, "discarded malformed native event: "+err.Error())
			continue
		}
		resolveProcessIdentity(&event)
		seen[event.SemanticKey()] = event
	}

	for _, event := range seen {
		result.Events = append(result.Events, event)
	}
	sortRuntimeEvents(result.Events)

	var dropped uint64
	if err := drops.Lookup(uint32(0), &dropped); err != nil {
		result.Warnings = append(result.Warnings, "could not read native drop counter: "+err.Error())
	} else {
		result.Dropped = dropped
		if dropped > 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("kernel ring buffer dropped %d event(s); evidence is incomplete", dropped))
		}
	}
	result.EndedAt = time.Now().UTC()
	return result, nil
}

type programLink struct {
	link.Link
	program *ebpf.Program
}

func (l *programLink) Close() error {
	linkErr := l.Link.Close()
	programErr := l.program.Close()
	if linkErr != nil {
		return linkErr
	}
	return programErr
}

func connectProgramSpec(events, drops *ebpf.Map, attachType ebpf.AttachType, family uint32) *ebpf.ProgramSpec {
	instructions := asm.Instructions{
		asm.Mov.Reg(asm.R6, asm.R1),
		asm.LoadMapPtr(asm.R1, events.FD()),
		asm.Mov.Imm(asm.R2, nativeEventSize),
		asm.Mov.Imm(asm.R3, 0),
		asm.FnRingbufReserve.Call(),
		asm.JEq.Imm(asm.R0, 0, "drop"),
		asm.Mov.Reg(asm.R7, asm.R0),

		asm.FnGetCurrentCgroupId.Call(),
		asm.StoreMem(asm.R7, 0, asm.R0, asm.DWord),
		asm.FnGetCurrentPidTgid.Call(),
		asm.StoreMem(asm.R7, 8, asm.R0, asm.DWord),

		asm.LoadMem(asm.R4, asm.R6, 0, asm.Word),
		asm.StoreMem(asm.R7, 16, asm.R4, asm.Word),
		asm.LoadMem(asm.R4, asm.R6, 24, asm.Word),
		asm.StoreMem(asm.R7, 20, asm.R4, asm.Word),
		asm.StoreImm(asm.R7, 24, 0, asm.DWord),
		asm.StoreImm(asm.R7, 32, 0, asm.DWord),
	}

	if family == afINET {
		instructions = append(instructions,
			asm.LoadMem(asm.R4, asm.R6, 4, asm.Word),
			asm.StoreMem(asm.R7, 24, asm.R4, asm.Word),
		)
	} else {
		for i := int16(0); i < 16; i += 4 {
			instructions = append(instructions,
				asm.LoadMem(asm.R4, asm.R6, 8+i, asm.Word),
				asm.StoreMem(asm.R7, 24+i, asm.R4, asm.Word),
			)
		}
	}

	instructions = append(instructions,
		asm.LoadMem(asm.R4, asm.R6, 36, asm.Word),
		asm.StoreMem(asm.R7, 40, asm.R4, asm.Word),
		asm.StoreImm(asm.R7, 44, 0, asm.Word),
		asm.Mov.Reg(asm.R1, asm.R7),
		asm.Mov.Imm(asm.R2, 0),
		asm.FnRingbufSubmit.Call(),
		asm.Ja.Label("allow"),

		asm.StoreImm(asm.RFP, -4, 0, asm.Word).WithSymbol("drop"),
		asm.LoadMapPtr(asm.R1, drops.FD()),
		asm.Mov.Reg(asm.R2, asm.RFP),
		asm.Add.Imm(asm.R2, -4),
		asm.FnMapLookupElem.Call(),
		asm.JEq.Imm(asm.R0, 0, "allow"),
		asm.Mov.Imm(asm.R1, 1),
		asm.StoreXAdd(asm.R0, asm.R1, asm.DWord),

		asm.Mov.Imm(asm.R0, 1).WithSymbol("allow"),
		asm.Return(),
	)

	return &ebpf.ProgramSpec{
		Name:         "wabi_connect",
		Type:         ebpf.CGroupSockAddr,
		AttachType:   attachType,
		License:      "Apache-2.0",
		Instructions: instructions,
	}
}

func decodeConnectEvent(sample []byte) (model.RuntimeEvent, error) {
	if len(sample) < nativeEventSize {
		return model.RuntimeEvent{}, fmt.Errorf("sample length %d, want at least %d", len(sample), nativeEventSize)
	}
	raw := rawConnectEvent{
		CgroupID: binary.NativeEndian.Uint64(sample[0:8]),
		PIDTGID:  binary.NativeEndian.Uint64(sample[8:16]),
		Family:   binary.NativeEndian.Uint32(sample[16:20]),
		PortRaw:  binary.NativeEndian.Uint32(sample[20:24]),
		Protocol: binary.NativeEndian.Uint32(sample[40:44]),
	}
	copy(raw.Addr[:], sample[24:40])

	pid := int(uint32(raw.PIDTGID))
	tgid := int(uint32(raw.PIDTGID >> 32))
	if tgid == 0 {
		tgid = pid
	}
	port := int(binary.BigEndian.Uint16(sample[20:22]))
	var ip net.IP
	switch raw.Family {
	case afINET:
		ip = net.IPv4(raw.Addr[0], raw.Addr[1], raw.Addr[2], raw.Addr[3])
	case afINET6:
		ip = net.IP(append([]byte(nil), raw.Addr[:]...))
	default:
		return model.RuntimeEvent{}, fmt.Errorf("unsupported address family %d", raw.Family)
	}
	if ip == nil {
		return model.RuntimeEvent{}, fmt.Errorf("invalid native address")
	}

	return model.RuntimeEvent{
		Source:    "native-ebpf",
		Category:  "network",
		Operation: "connect",
		Target:    net.JoinHostPort(ip.String(), strconv.Itoa(port)),
		Protocol:  protocolName(raw.Protocol),
		Direction: "outbound",
		PID:       tgid,
	}, nil
}

func protocolName(protocol uint32) string {
	switch protocol {
	case 6:
		return "tcp"
	case 17:
		return "udp"
	case 0:
		return ""
	default:
		return strconv.FormatUint(uint64(protocol), 10)
	}
}

func resolveProcessIdentity(event *model.RuntimeEvent) {
	if event.PID <= 0 {
		return
	}
	event.Process = executableIdentity(event.PID)
	parent := parentPID(event.PID)
	if parent > 0 {
		event.ParentProcess = executableIdentity(parent)
	}
}

func executableIdentity(pid int) string {
	if target, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil {
		return strings.TrimSuffix(target, " (deleted)")
	}
	if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid)); err == nil {
		return strings.TrimSpace(string(data))
	}
	return ""
}

func parentPID(pid int) int {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}
	text := string(data)
	end := strings.LastIndex(text, ")")
	if end < 0 || end+2 >= len(text) {
		return 0
	}
	fields := strings.Fields(text[end+2:])
	if len(fields) < 2 {
		return 0
	}
	ppid, _ := strconv.Atoi(fields[1])
	return ppid
}

func resolveCgroupPath(cfg Config) (string, error) {
	if cfg.CgroupPath != "" && cfg.PID > 0 {
		return "", fmt.Errorf("native recorder accepts either cgroup path or PID, not both")
	}
	if cfg.CgroupPath != "" {
		return validateCgroupPath(cfg.CgroupPath)
	}
	if cfg.PID <= 0 {
		return "", fmt.Errorf("native recorder requires --cgroup or --pid")
	}

	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", cfg.PID))
	if err != nil {
		return "", fmt.Errorf("read PID %d cgroup: %w", cfg.PID, err)
	}
	var relative string
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) == 3 && parts[0] == "0" && parts[1] == "" {
			relative = parts[2]
			break
		}
	}
	if relative == "" {
		return "", fmt.Errorf("PID %d is not in a discoverable cgroup v2 hierarchy", cfg.PID)
	}
	mount, err := cgroup2MountPoint()
	if err != nil {
		return "", err
	}
	path := filepath.Join(mount, strings.TrimPrefix(relative, "/"))
	return validateCgroupPath(path)
}

func cgroup2MountPoint() (string, error) {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return "", fmt.Errorf("read mountinfo: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		separator := strings.Index(line, " - ")
		if separator < 0 {
			continue
		}
		left := strings.Fields(line[:separator])
		right := strings.Fields(line[separator+3:])
		if len(left) < 5 || len(right) == 0 || right[0] != "cgroup2" {
			continue
		}
		return unescapeMountField(left[4]), nil
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan mountinfo: %w", err)
	}
	return "", fmt.Errorf("cgroup v2 mount not found")
}

func unescapeMountField(value string) string {
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return replacer.Replace(value)
}

func validateCgroupPath(path string) (string, error) {
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat cgroup %s: %w", path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("cgroup path %s is not a directory", path)
	}
	return path, nil
}

func normalizeRingSize(size uint32) uint32 {
	page := uint32(os.Getpagesize())
	if size < page {
		size = page
	}
	value := page
	for value < size && value <= 1<<30 {
		value <<= 1
	}
	return value
}

func sortRuntimeEvents(events []model.RuntimeEvent) {
	for i := 1; i < len(events); i++ {
		for j := i; j > 0 && events[j].SemanticKey() < events[j-1].SemanticKey(); j-- {
			events[j], events[j-1] = events[j-1], events[j]
		}
	}
}
