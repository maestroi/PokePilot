package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

// TestClearSilphCoRealROM is the focused third #34 stage. The external state
// should be controllable after the Card Key has been collected and before the
// Silph rival/Giovanni sequence is complete. It exercises both required Card
// Key doors, the 3F->7F and 7F->11F pads, both story battles, and the president
// reward without committing ROM-derived state to the repository.
func TestClearSilphCoRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_SILPH_CLEAR_TEST_STATE")
	policy := StatAwareMove(m.ROM())

	before := currentSilphFacts(m)
	if !before.SaffronGateOpen || !before.CardKeyOwned {
		t.Fatal("prepared Silph clear state must have Saffron open and Card Key owned")
	}
	if before.SilphRescueComplete {
		t.Fatal("prepared Silph clear state is already complete")
	}

	if err := ClearSilphCo(m, m.ROM(), policy); err != nil {
		t.Fatalf("ClearSilphCo: %v", err)
	}

	after := currentSilphFacts(m)
	if !after.SilphCoRivalDefeated {
		t.Fatal("Silph rival durable event is not set")
	}
	if !after.SilphCoCleared {
		t.Fatal("Giovanni durable event is not set")
	}
	if !after.MasterBallAwarded || !after.SilphRescueComplete {
		t.Fatalf("president reward did not complete Silph rescue: %+v", after)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if _, count := bagEntry(&mem, masterBallItemID); count < 1 {
		t.Fatal("Master Ball was not present immediately after president reward")
	}

	if err := ClearSilphCo(m, m.ROM(), policy); err != nil {
		t.Fatalf("ClearSilphCo idempotent retry: %v", err)
	}
}
