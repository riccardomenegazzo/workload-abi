package main

import "testing"

func TestInterspersedFlags(t *testing.T) {
	got := interspersed([]string{"app:v1", "app:v2", "--scenario", "s.json", "--fail-on=never"})
	want := []string{"--scenario", "s.json", "--fail-on=never", "app:v1", "app:v2"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestValidFailOn(t *testing.T) {
	for _, value := range []string{"breaking", "degraded", "never", "BREAKING"} {
		if !validFailOn(value) {
			t.Fatalf("expected %q to be valid", value)
		}
	}
	if validFailOn("critical") {
		t.Fatal("unexpectedly accepted invalid failure threshold")
	}
}
