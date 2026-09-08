package doctor

import "testing"

func TestTruncate(t *testing.T) {
	if got := truncate("abc", 3); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if got := truncate("abcd", 3); got != "abc…" {
		t.Fatalf("got %q", got)
	}
}
