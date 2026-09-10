package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// TestSaffronGymRealROM is the focused final phase of issue #34. The external
// checkpoint should be a controllable post-Silph-rescue state in Saffron City
// or Saffron Gym, before Sabrina has awarded the Marsh Badge. The test proves
// the city door, same-map teleport maze, trainer interruptions, Sabrina battle,
// and positive badge postcondition without committing ROM-derived state.
func TestSaffronGymRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_SAFFRON_GYM_TEST_STATE")
	policy := StatAwareMove(m.ROM())

	var before state.Mem
	state.Snapshot(m, &before)
	facts := state.DecodeStoryFacts(&before, state.DecodeInventory(&before))
	if !facts.SilphRescueComplete {
		t.Fatal("prepared Saffron Gym state has not completed the Silph rescue")
	}
	if state.DecodeProgress(&before).Has(state.BadgeMarsh) {
		t.Fatal("prepared Saffron Gym state already has the Marsh Badge")
	}

	switch before.U8(sym.CurMap) {
	case 0x0A: // Saffron City
		dest, ok := Place("saffron gym")
		if !ok {
			t.Fatal("saffron gym place missing")
		}
		if _, err := TravelFlee(m, m.ROM(), dest, policy, 30); err != nil {
			t.Fatalf("travel from Saffron City to Gym: %v", err)
		}
	case saffronGymMap:
		// Resumability: a checkpoint may already be somewhere in the warp maze.
	default:
		t.Fatalf("prepared Saffron Gym state is on map %#02x, want Saffron City or Gym", before.U8(sym.CurMap))
	}

	outcome, err := Gym(m, m.ROM(), policy)
	if err != nil {
		t.Fatalf("Saffron Gym: %v", err)
	}
	if outcome != state.ResultWon {
		t.Fatalf("Sabrina outcome = %v, want won", outcome)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	if !state.DecodeProgress(&after).Has(state.BadgeMarsh) {
		t.Fatal("Marsh Badge is not set after beating Sabrina")
	}
	if !state.Controllable(&after) {
		t.Fatal("player is not controllable after the Sabrina sequence")
	}
}
