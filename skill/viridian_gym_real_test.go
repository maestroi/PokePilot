package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

// TestViridianGymRealROM is the focused issue #36 qualification. The external
// checkpoint should be a controllable post-Blaine state with the seven prior
// badges and no Earth Badge. It may be anywhere ordinary Travel can resume
// from, including Viridian City or inside Viridian Gym. The test proves the
// city story-open handoff, real gym door, forced arrow transitions, trainer
// interruptions, Giovanni battle, and all-eight-badges postcondition.
func TestViridianGymRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_VIRIDIAN_GYM_TEST_STATE")
	policy := StatAwareMove(m.ROM())

	var before state.Mem
	state.Snapshot(m, &before)
	progress := state.DecodeProgress(&before)
	if progress.BadgeCount < 7 {
		t.Fatalf("prepared Viridian Gym state has %d badges, need at least seven", progress.BadgeCount)
	}
	if progress.Has(state.BadgeEarth) {
		t.Fatal("prepared Viridian Gym state already has the Earth Badge")
	}

	if err := ViridianProgression(m, m.ROM(), policy); err != nil {
		t.Fatalf("Viridian progression: %v", err)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	progress = state.DecodeProgress(&after)
	if !progress.Has(state.BadgeEarth) || progress.BadgeCount != 8 {
		t.Fatalf("badges after Giovanni = %#02x count=%d, want all eight", progress.Badges, progress.BadgeCount)
	}
	if !state.Controllable(&after) {
		t.Fatal("player is not controllable after the Giovanni sequence")
	}
}
