package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

// TestAcquireSilphCardKeyRealROM is the focused second #34 stage. The external
// state should be a controllable post-#33 checkpoint with Saffron access open
// and the Card Key not yet collected. The test intentionally exercises the
// real building graph from wherever that checkpoint resumes, including Silph
// Co's ordinary stair warps and trainer interruptions on the way to 5F.
func TestAcquireSilphCardKeyRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_SILPH_CARD_KEY_TEST_STATE")

	var before state.Mem
	state.Snapshot(m, &before)
	if !SilphCardKeyReady(&before) {
		t.Fatal("prepared #34 Card Key state does not have Saffron access open")
	}
	if SilphCardKeyOwned(&before) {
		t.Fatal("prepared #34 Card Key state already owns the Card Key")
	}

	if err := AcquireSilphCardKey(m, m.ROM(), StatAwareMove(m.ROM())); err != nil {
		t.Fatalf("AcquireSilphCardKey: %v", err)
	}

	var after state.Mem
	state.Snapshot(m, &after)
	if !SilphCardKeyOwned(&after) {
		t.Fatal("Card Key semantic postcondition is false after acquisition")
	}

	// Idempotence is part of the progression contract: a resumed checkpoint
	// after pickup must not navigate back to the ball or try to collect it.
	if err := AcquireSilphCardKey(m, m.ROM(), StatAwareMove(m.ROM())); err != nil {
		t.Fatalf("AcquireSilphCardKey idempotent retry: %v", err)
	}
}
