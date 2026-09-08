package dockercli

import "testing"

func TestProcReadRecognizesOnlyProcCatExec(t *testing.T) {
	id, path, ok := procRead([]string{"exec", "abc123", "cat", "/proc/net/tcp"})
	if !ok || id != "abc123" || path != "/proc/net/tcp" {
		t.Fatalf("unexpected proc read parse: id=%q path=%q ok=%v", id, path, ok)
	}

	for _, args := range [][]string{
		{"exec", "abc123", "cat", "/etc/passwd"},
		{"exec", "abc123", "sh", "/proc/1/status"},
		{"inspect", "abc123"},
	} {
		if _, _, ok := procRead(args); ok {
			t.Fatalf("unexpected fallback eligibility for %v", args)
		}
	}
}
