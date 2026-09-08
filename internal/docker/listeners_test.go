package docker

import "testing"

func TestParseTCPListeners(t *testing.T) {
	raw := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 0
   1: 0100007F:C001 0100007F:01BB 01 00000000:00000000 00:00000000 00000000 0 0 0`
	got := parseTCPListeners(raw, "tcp")
	if len(got) != 1 || got[0].Port != 8080 {
		t.Fatalf("unexpected listeners: %#v", got)
	}
}
