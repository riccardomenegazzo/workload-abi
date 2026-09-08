package observe

import "testing"

func TestParseProcNetListenAndEstablished(t *testing.T) {
	input := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  0 0 1
   1: 0100007F:9C40 08080808:01BB 01 00000000:00000000 00:00000000 00000000  0 0 2`
	eps, err := ParseProcNet(input, "tcp", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 2 {
		t.Fatalf("want 2 endpoints, got %d", len(eps))
	}
	if eps[0].Direction != "listen" || eps[0].LocalPort != 8080 {
		t.Fatalf("unexpected listener: %+v", eps[0])
	}
	if eps[1].RemoteAddress != "8.8.8.8" || eps[1].RemotePort != 443 {
		t.Fatalf("unexpected outbound: %+v", eps[1])
	}
}
