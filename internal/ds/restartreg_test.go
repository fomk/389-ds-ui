package ds

import (
	"slices"
	"testing"
)

func TestRestartRegistry(t *testing.T) {
	var r RestartRegistry
	r.Change("USN", "off", "on")
	r.Change("nsslapd-port", "3389", "3390")
	if !slices.Equal(r.Reasons(), []string{"USN", "nsslapd-port"}) {
		t.Fatalf("reasons = %v", r.Reasons())
	}
	// Back to what the server runs with: no restart needed.
	r.Change("USN", "on", "off")
	if !slices.Equal(r.Reasons(), []string{"nsslapd-port"}) {
		t.Errorf("after revert = %v", r.Reasons())
	}
	// Further changes keep the running value, not the intermediate one.
	r.Change("nsslapd-port", "3390", "3391")
	r.Change("nsslapd-port", "3391", "3390")
	if !slices.Equal(r.Reasons(), []string{"nsslapd-port"}) {
		t.Errorf("intermediate value cleared the reason: %v", r.Reasons())
	}
	r.Change("nsslapd-port", "3390", "3389")
	if len(r.Reasons()) != 0 {
		t.Errorf("reasons = %v", r.Reasons())
	}
	// A no-op change records nothing; an unknown running value needs a restart.
	r.Change("x", "a", "a")
	r.Require("y")
	r.Change("y", "b", "")
	if !slices.Equal(r.Reasons(), []string{"y"}) {
		t.Errorf("reasons = %v", r.Reasons())
	}
	r.Clear()
	if len(r.Reasons()) != 0 {
		t.Error("Clear")
	}
}
