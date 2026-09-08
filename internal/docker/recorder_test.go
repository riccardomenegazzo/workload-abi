package docker

import "testing"

func TestParseBytes(t *testing.T) {
	cases := map[string]int64{
		"1B": 1,
		"1KiB": 1024,
		"1.5MiB": 1572864,
		"2GB": 2000000000,
	}
	for in, want := range cases {
		if got := ParseBytes(in); got != want {
			t.Fatalf("ParseBytes(%q)=%d want %d", in, got, want)
		}
	}
}
