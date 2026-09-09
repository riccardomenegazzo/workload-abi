//go:build linux

package native

import (
	"encoding/binary"
	"net"
	"testing"
)

func TestDecodeIPv4ConnectEvent(t *testing.T) {
	sample := make([]byte, nativeEventSize)
	binary.NativeEndian.PutUint64(sample[0:8], 77)
	binary.NativeEndian.PutUint64(sample[8:16], uint64(4242)<<32|4243)
	binary.NativeEndian.PutUint32(sample[16:20], afINET)
	copy(sample[20:22], []byte{0x01, 0xbb})
	copy(sample[24:28], net.IPv4(203, 0, 113, 10).To4())
	binary.NativeEndian.PutUint32(sample[40:44], 6)

	event, err := decodeConnectEvent(sample)
	if err != nil {
		t.Fatal(err)
	}
	if event.Source != "native-ebpf" || event.Category != "network" || event.Operation != "connect" {
		t.Fatalf("unexpected event identity: %#v", event)
	}
	if event.Target != "203.0.113.10:443" {
		t.Fatalf("target=%q", event.Target)
	}
	if event.Protocol != "tcp" || event.Direction != "outbound" {
		t.Fatalf("unexpected network metadata: %#v", event)
	}
	if event.PID != 4242 {
		t.Fatalf("PID=%d want TGID 4242", event.PID)
	}
}

func TestDecodeIPv6ConnectEvent(t *testing.T) {
	sample := make([]byte, nativeEventSize)
	binary.NativeEndian.PutUint64(sample[8:16], uint64(99)<<32|100)
	binary.NativeEndian.PutUint32(sample[16:20], afINET6)
	copy(sample[20:22], []byte{0x20, 0xfb})
	ip := net.ParseIP("2001:db8::42").To16()
	copy(sample[24:40], ip)
	binary.NativeEndian.PutUint32(sample[40:44], 17)

	event, err := decodeConnectEvent(sample)
	if err != nil {
		t.Fatal(err)
	}
	if event.Target != "[2001:db8::42]:8443" {
		t.Fatalf("target=%q", event.Target)
	}
	if event.Protocol != "udp" {
		t.Fatalf("protocol=%q", event.Protocol)
	}
}

func TestDecodeRejectsShortNativeSample(t *testing.T) {
	if _, err := decodeConnectEvent(make([]byte, 12)); err == nil {
		t.Fatal("expected short sample rejection")
	}
}

func TestNormalizeRingSize(t *testing.T) {
	page := uint32(4096)
	if got := normalizeRingSize(1); got < page {
		t.Fatalf("ring size %d is below a page", got)
	}
	got := normalizeRingSize(7000)
	if got&(got-1) != 0 {
		t.Fatalf("ring size %d is not a power of two", got)
	}
}

func TestProtocolName(t *testing.T) {
	if protocolName(6) != "tcp" || protocolName(17) != "udp" || protocolName(132) != "132" {
		t.Fatal("unexpected protocol normalization")
	}
}
