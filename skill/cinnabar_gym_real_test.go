package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// TestCinnabarGymRealROM is the focused final phase of issue #35. The external
// checkpoint should be a controllable state after the Secret Key has been
// obtained, before Blaine has awarded the Volcano Badge. It may be left in the
// Mansion basement by the preceding objective, on Cinnabar Island, or already
// inside the Gym. The test proves the Secret Key door, six quiz/trainer gates,
// Blaine battle, and positive badge postcondition without committing ROM data.
func TestCinnabarGymRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_CINNABAR_GYM_TEST_STATE")
	policy := StatAwareMove(m.ROM())

	var before state.Mem
	state.Snapshot(m, &before)
	if !CinnabarSecretKeyOwned(&before) {
		t.Fatal("prepared Cinnabar Gym state does not own the Secret Key")
	}
	if state.DecodeProgress(&before).Has(state.BadgeVolcano) {
		t.Fatal("prepared Cinnabar Gym state already has the Volcano Badge")
	}
	switch before.U8(sym.CurMap) {
	case pokemonMansionB1FMap, cinnabarIslandMap, cinnabarGymMap:
	default:
		t.Fatalf("prepared Cinnabar Gym state is on map %#02x, want Mansion B1F, Cinnabar Island, or Gym", before.U8(sym.CurMap))
	}

	if err := CinnabarProgression(m, m.ROM(), policy); err != nil {
		t.Fatalf("Cinnabar progression: %v", err)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	if !state.DecodeProgress(&after).Has(state.BadgeVolcano) {
		t.Fatal("Volcano Badge is not set after beating Blaine")
	}
	if !state.Controllable(&after) {
		t.Fatal("player is not controllable after the Blaine sequence")
	}
}
