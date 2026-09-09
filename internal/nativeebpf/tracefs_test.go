package nativeebpf

import "testing"

func TestParseFieldOffset(t *testing.T) {
	format := `name: sys_enter_openat
ID: 123
format:
	field:unsigned short common_type;	offset:0;	size:2;	signed:0;
	field:int __syscall_nr;	offset:8;	size:4;	signed:1;
	field:const char * filename;	offset:24;	size:8;	signed:0;
`
	offset, err := parseFieldOffset(format, "filename")
	if err != nil {
		t.Fatal(err)
	}
	if offset != 24 {
		t.Fatalf("offset=%d want 24", offset)
	}
}

func TestParseFieldOffsetRejectsMissingField(t *testing.T) {
	if _, err := parseFieldOffset("field:int fd; offset:16; size:8;", "filename"); err == nil {
		t.Fatal("expected missing field error")
	}
}
