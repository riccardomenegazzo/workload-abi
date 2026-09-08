package scenario

import "testing"

func TestEnvListIsDeterministic(t *testing.T) {
	s := Spec{Environment: map[string]string{"Z": "last", "A": "first"}}
	got := s.EnvList()
	if len(got) != 2 || got[0] != "A=first" || got[1] != "Z=last" {
		t.Fatalf("unexpected env list: %#v", got)
	}
}

func TestPlansNormalizeDefaults(t *testing.T) {
	s := Spec{Steps: []Step{{Exec: []string{"true"}}}}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	plans, err := s.Plans()
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || plans[0].Name != "step-1" || plans[0].Timeout <= 0 {
		t.Fatalf("unexpected plans: %#v", plans)
	}
}

func TestValidateRejectsDuplicateNames(t *testing.T) {
	s := Spec{Steps: []Step{
		{Name: "probe", Exec: []string{"true"}},
		{Name: "probe", Exec: []string{"true"}},
	}}
	if err := s.Validate(); err == nil {
		t.Fatal("expected duplicate step name error")
	}
}
