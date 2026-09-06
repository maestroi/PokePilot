package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

// TestFuchsiaProgressionRealROM is the focused #33 run that PR #57 could not
// execute in ROM-free CI. POKEPILOT_FUCHSIA_TEST_STATE should point at a
// controllable post-#32 state before the Route 12 Snorlax has been cleared,
// with the Poké Flute owned and enough ordinary battle resources/money to
// attempt the slice. The state remains external because .state files are
// ROM-derived artifacts and must not be committed.
func TestFuchsiaProgressionRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_FUCHSIA_TEST_STATE")

	var before state.Mem
	state.Snapshot(m, &before)
	if !FuchsiaProgressionReady(&before) {
		t.Fatal("prepared #33 state does not own the Poke Flute")
	}
	if !FuchsiaProgressionAvailable(before.U8(0xD35E)) {
		t.Fatalf("prepared #33 state is on unsupported map %#02x", before.U8(0xD35E))
	}
	if state.HasEvent(&before, eventBeatRoute12Snorlax) {
		t.Fatal("prepared #33 state already cleared the Route 12 Snorlax; want the full post-#32 slice")
	}
	if FuchsiaProgressionComplete(&before) {
		t.Fatal("prepared #33 state is already complete")
	}

	if err := FuchsiaProgression(m, m.ROM(), StatAwareMove(m.ROM())); err != nil {
		t.Fatalf("FuchsiaProgression: %v", err)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	if !state.HasEvent(&after, eventBeatRoute12Snorlax) {
		t.Fatal("Route 12 Snorlax completion event is not set")
	}
	if !state.DecodeProgress(&after).Has(state.BadgeSoul) {
		t.Fatal("Soul Badge is not set after #33 progression")
	}
	if !state.HasEvent(&after, eventGotHM03) || !hasBagItem(&after, hm03SurfItem) {
		t.Fatal("HM03 Surf was not positively awarded")
	}
	if !state.HasEvent(&after, eventGotHM04) || !hasBagItem(&after, hm04StrengthItem) {
		t.Fatal("HM04 Strength was not positively awarded")
	}
	if !FuchsiaProgressionComplete(&after) {
		t.Fatal("#33 positive postcondition is false after progression")
	}
}
