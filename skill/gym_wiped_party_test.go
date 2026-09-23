package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// TestGymWipedPartyBlacksOut replays farm triage 2aae9cbdac003648 (Fuchsia /
// Koga, 340 occurrences). The prepared state is a controllable overworld on
// the Fuchsia Gym (0x9D) with the party already wiped out and the player at
// the leader. Before the fix, Gym tapped A, the game ran HandleBlackOut and
// carried the player to the center, and the battle wait timed out into a
// terminal unknown_failure. Gym must now report the structured blackout so
// the objective re-plans instead of the run dying.
func TestGymWipedPartyBlacksOut(t *testing.T) {
	m := loadPreparedState(t, "POKEPILOT_FUCHSIA_GYM_WIPED_STATE")
	rom := m.ROM()

	var before state.Mem
	state.Snapshot(m, &before)
	if before.U8(sym.CurMap) != 0x9d {
		t.Fatalf("prepared state is on map %#02x, want Fuchsia Gym %#02x", before.U8(sym.CurMap), 0x9d)
	}
	if !partyAllFainted(m) {
		t.Fatal("prepared state party is not wiped out")
	}

	res, err := Gym(m, rom, StatAwareMove(rom))
	if !errors.Is(err, ErrBlackedOut) {
		t.Fatalf("Gym res=%d err=%v, want ErrBlackedOut", res, err)
	}
	var after state.Mem
	state.Snapshot(m, &after)
	if !state.Controllable(&after) {
		t.Fatal("player not controllable after the blackout")
	}
	if partyAllFainted(m) {
		t.Fatal("party still wiped out after the blackout (should be healed)")
	}
}
