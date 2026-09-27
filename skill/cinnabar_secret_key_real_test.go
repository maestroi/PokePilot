package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// TestAcquireCinnabarSecretKeyRealROM exercises the Mansion transaction from a
// controllable checkpoint with usable Surf. Campaign ordering is adapter-owned;
// the state may start anywhere before Cinnabar, and the skill routes through
// Pallet/Route 21, enters the
// Mansion, follows live statue geometry, takes the 3F dungeon fall, and proves
// the Secret Key from the resulting bag state.
func TestAcquireCinnabarSecretKeyRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_CINNABAR_SECRET_KEY_TEST_STATE")

	var before state.Mem
	state.Snapshot(m, &before)
	if !CinnabarSecretKeyReady(&before) {
		t.Fatal("prepared Secret Key state does not have usable Surf")
	}
	if CinnabarSecretKeyOwned(&before) {
		t.Fatal("prepared #35 Secret Key state already owns the Secret Key")
	}

	if err := AcquireCinnabarSecretKey(m, m.ROM(), StatAwareMove(m.ROM())); err != nil {
		t.Fatalf("AcquireCinnabarSecretKey: %v", err)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	if !CinnabarSecretKeyOwned(&after) {
		t.Fatal("Secret Key semantic postcondition is false after Mansion traversal")
	}
	if err := AcquireCinnabarSecretKey(m, m.ROM(), StatAwareMove(m.ROM())); err != nil {
		t.Fatalf("AcquireCinnabarSecretKey idempotent retry: %v", err)
	}
}

// TestCinnabarProgressionLeavesSealedMansion1FRealROM starts where the
// Secret Key hunt hands off in run-1biaubd9xooqm: 1F at the B1F stairs, with
// the shared switch sealing that pocket and the 1F statue outside it. The
// exit needs a B1F statue flip that keeps the B1F stairs reachable.
func TestCinnabarProgressionLeavesSealedMansion1FRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_CINNABAR_MANSION_1F_SEALED_TEST_STATE")
	if got := m.Peek8(sym.CurMap); got != pokemonMansion1FMap {
		t.Fatalf("prepared state map = %#04x, want Mansion 1F", got)
	}
	if err := CinnabarProgression(m, m.ROM(), StatAwareMove(m.ROM())); err != nil {
		t.Fatalf("CinnabarProgression: %v", err)
	}
}
