package target

import (
	"encoding/json"
	"testing"
)

func TestParseMemoryLimit(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int64
	}{
		{name: "bytes", raw: `536870912`, want: 536870912},
		{name: "mib", raw: `"512MiB"`, want: 536870912},
		{name: "gb", raw: `"1GB"`, want: 1000000000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseMemoryLimit(json.RawMessage(tc.raw)); got != tc.want {
				t.Fatalf("got %d want %d", got, tc.want)
			}
		})
	}
}

func TestCleanPath(t *testing.T) {
	if got := cleanPath("var/lib/app/"); got != "/var/lib/app" {
		t.Fatalf("cleanPath=%q", got)
	}
}
