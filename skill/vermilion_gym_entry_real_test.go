package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// This reuses the controllable Vermilion checkpoint used by the lower-level
// semantic Cut transition test, but exercises the production Gym entry seam.
// The regression case is specifically a party that already knows usable Cut:
// the entry must execute that move through the semantic route gate and then
// cross the gym warp instead of probing arbitrary solid tiles near the door.
func TestVermilionGymEntryViaRouteGateRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_CUT_ROUTE_TEST_STATE")
	if got := m.Peek8(sym.CurMap); got != vermilionCity {
		t.Fatalf("Vermilion entry checkpoint map=%02x, want %02x", got, vermilionCity)
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	cap := FieldCapabilityFor(&mem, FieldCut)
	if !cap.Usable {
		t.Skipf("checkpoint does not model the learned-Cut regression case: %+v", cap)
	}

	if err := enterVermilionGymViaRouteGate(m, m.ROM(), FirstUsableMove); err != nil {
		t.Fatalf("enter Vermilion Gym through semantic Cut gate: %v", err)
	}
	if got := m.Peek8(sym.CurMap); got != vermilionGymMap {
		t.Fatalf("map after Vermilion Gym entry=%02x, want %02x", got, vermilionGymMap)
	}
}
