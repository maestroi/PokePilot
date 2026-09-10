package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

// TestAcquireCinnabarSecretKeyRealROM exercises the first #35 stage from a
// controllable post-#34 checkpoint. The state may start anywhere before
// Cinnabar; the skill deliberately routes through Pallet/Route 21, enters the
// Mansion, follows live statue geometry, takes the 3F dungeon fall, and proves
// the Secret Key from the resulting bag state.
func TestAcquireCinnabarSecretKeyRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_CINNABAR_SECRET_KEY_TEST_STATE")

	var before state.Mem
	state.Snapshot(m, &before)
	if !CinnabarSecretKeyReady(&before) {
		t.Fatal("prepared #35 Secret Key state does not satisfy the post-#34 Surf handoff")
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
