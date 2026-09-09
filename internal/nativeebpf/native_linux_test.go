//go:build linux

package nativeebpf

import (
	"encoding/binary"
	"testing"
)

func TestDecodeOpenEvent(t *testing.T) {
	raw := make([]byte, rawEventSize)
	binary.LittleEndian.PutUint32(raw[0:4], kindOpenat)
	binary.LittleEndian.PutUint32(raw[4:8], 999999)
	copy(raw[commOffset:commOffset+commSize], []byte("helper"))
	copy(raw[payloadOffset:], []byte("/var/run/secrets/token"))

	event := decodeEvent(raw)
	if event.Source != "native-ebpf" || event.Category != "file" || event.Operation != "open" {
		t.Fatalf("unexpected event: %#v", event)
	}
	if event.Process != "helper" || event.Target != "/var/run/secrets/token" {
		t.Fatalf("unexpected normalized evidence: %#v", event)
	}
}

func TestSocketTargetIPv4(t *testing.T) {
	raw := make([]byte, 28)
	binary.LittleEndian.PutUint16(raw[0:2], 2)
	binary.BigEndian.PutUint16(raw[2:4], 443)
	copy(raw[4:8], []byte{203, 0, 113, 10})

	target, protocol := socketTarget(raw)
	if protocol != "tcp" || target != "203.0.113.10:443" {
		t.Fatalf("target=%q protocol=%q", target, protocol)
	}
}

func TestSocketTargetIPv6(t *testing.T) {
	raw := make([]byte, 28)
	binary.LittleEndian.PutUint16(raw[0:2], 10)
	binary.BigEndian.PutUint16(raw[2:4], 8443)
	copy(raw[8:24], []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})

	target, protocol := socketTarget(raw)
	if protocol != "tcp" || target != "[2001:db8::1]:8443" {
		t.Fatalf("target=%q protocol=%q", target, protocol)
	}
}
