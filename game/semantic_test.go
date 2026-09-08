package game

import "testing"

func TestCanonicalID(t *testing.T) {
	if got := CanonicalID("  Pidgey  "); got != "pidgey" {
		t.Fatalf("CanonicalID = %q, want pidgey", got)
	}
}

func TestProgressState(t *testing.T) {
	const gate ProgressID = "gate_open"
	const checks ProgressID = "badge_checks"
	state := ProgressState{
		{ID: gate, Complete: true},
		{ID: checks, Complete: false, Value: 4},
	}
	if !state.Has(gate) {
		t.Fatal("completed fact not reported")
	}
	if state.Has(checks) {
		t.Fatal("incomplete fact reported complete")
	}
	if got, ok := state.Value(checks); !ok || got != 4 {
		t.Fatalf("Value = %d,%v, want 4,true", got, ok)
	}
	if _, ok := state.Value("missing"); ok {
		t.Fatal("missing fact unexpectedly present")
	}
}
