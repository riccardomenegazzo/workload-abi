package scenario

import "testing"

func TestEnvListIsDeterministic(t *testing.T) {
	s := Spec{Environment: map[string]string{"Z": "last", "A": "first"}}
	got := s.EnvList()
	if len(got) != 2 || got[0] != "A=first" || got[1] != "Z=last" {
		t.Fatalf("unexpected env list: %#v", got)
	}
}
