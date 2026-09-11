package agent

import "testing"

// TestRunEndsEarlyAtADeadPosition: a stuck position (same map+tile, zero
// newly completed objectives) must reach the stop threshold in three
// unchanged follow-up rounds. Modeled on run-g9ojxmtgvrff1ezck9g7t1o7x,
// where the player never left ROUTE_12 (9,62) across 15 rounds. Completing
// an objective or leaving the tile must reset the streak so a Center
// heal-then-return loop cannot look dead.
func TestRunEndsEarlyAtADeadPosition(t *testing.T) {
	var d deadPosition
	obs := Observation{Map: 0x17, X: 9, Y: 62}

	if got := d.observe(obs, 0); got != 0 {
		t.Fatalf("first observation streak = %d, want 0 (no previous round)", got)
	}
	for round := 2; round <= 4; round++ {
		got := d.observe(obs, 0)
		want := round - 1
		if got != want {
			t.Fatalf("round %d streak = %d, want %d", round, got, want)
		}
	}
	if d.streak < deadPositionAfter {
		t.Fatalf("streak = %d after 3 unchanged rounds, want >= %d", d.streak, deadPositionAfter)
	}

	if got := d.observe(obs, 1); got != 0 {
		t.Fatalf("streak after a new completion = %d, want 0", got)
	}
	moved := obs
	moved.X = 10
	if got := d.observe(moved, 1); got != 0 {
		t.Fatalf("streak after leaving the tile = %d, want 0", got)
	}
}
