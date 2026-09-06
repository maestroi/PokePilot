package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// TestRepairFieldCapabilitiesFullPartySurfRealROM is #107's prepared-state
// qualification: start with a FULL six-mon party that owns HM03 + Soul Badge
// but has no member that can learn Surf, while the active Bill's PC box holds
// a compatible mon. The prepared state must stand directly SOUTH of surfable
// water so the test can return to that exact tile after the PC round trip and
// positively exercise Surf.
//
// The state is external for the same reason as the other field-action states:
// .state files are ROM-derived artifacts and are never committed. Set
// POKEPILOT_FIELD_ROSTER_TEST_STATE to run this test locally/private CI.
func TestRepairFieldCapabilitiesFullPartySurfRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_FIELD_ROSTER_TEST_STATE")
	policy := StatAwareMove(m.ROM())

	var before state.Mem
	state.Snapshot(m, &before)
	partyBefore := state.DecodeParty(&before)
	if partyBefore.Count != gen1PartyCapacity {
		t.Fatalf("prepared roster state has party count %d, want full party %d", partyBefore.Count, gen1PartyCapacity)
	}
	capBefore := FieldCapabilityFor(&before, FieldSurf)
	if !capBefore.BadgeOwned || !capBefore.HMOwned {
		t.Fatalf("prepared roster state must own Soul Badge + HM03: %+v", capBefore)
	}
	if capBefore.Usable || CanPrepareFieldMove(m.ROM(), &before, FieldSurf) {
		t.Fatalf("prepared roster state already has/permits a current-party Surf user: %+v", capBefore)
	}

	boxBefore := state.DecodeBox(&before)
	boxIndex, depositSlot, ok, err := chooseCompatibleBoxMon(m.ROM(), partyBefore, boxBefore, FieldSurf, []FieldMove{FieldSurf})
	if err != nil {
		t.Fatalf("inspect active box: %v", err)
	}
	if !ok || boxIndex < 0 || depositSlot < 0 {
		t.Fatalf("prepared roster state has no legal active-box Surf swap: box=%d count=%d index=%d deposit=%d", boxBefore.Number+1, boxBefore.Count, boxIndex, depositSlot)
	}
	if boxBefore.Count >= gen1BoxCapacity {
		t.Fatalf("prepared active box is full (%d); this qualification targets the normal deposit/withdraw path", boxBefore.Count)
	}

	origin := Destination{
		Map: before.U8(sym.CurMap),
		X:   before.U8(sym.XCoord),
		Y:   before.U8(sym.YCoord),
	}
	if origin.Y == 0 {
		t.Fatal("prepared roster state cannot face a water tile north of Y=0")
	}

	if err := RepairFieldCapabilities(m, m.ROM(), policy, []FieldMove{FieldSurf}); err != nil {
		t.Fatalf("RepairFieldCapabilities(Surf): %v", err)
	}

	var repaired state.Mem
	state.Snapshot(m, &repaired)
	partyAfter := state.DecodeParty(&repaired)
	if partyAfter.Count != gen1PartyCapacity {
		t.Fatalf("party count after repair = %d, want %d", partyAfter.Count, gen1PartyCapacity)
	}
	if cap := FieldCapabilityFor(&repaired, FieldSurf); !cap.Usable {
		t.Fatalf("Surf is not usable after roster repair: %+v", cap)
	}
	changed := false
	for i := range partyBefore.Mons {
		if partyBefore.Mons[i].Species != partyAfter.Mons[i].Species {
			changed = true
			break
		}
	}
	if !changed {
		t.Fatal("full-party repair reported success without changing party composition")
	}

	// Repair visits a Pokémon Center for Bill's PC. Return to the prepared
	// shoreline, face the guaranteed water tile immediately north, and prove
	// the repaired roster can execute the field move in the real ROM.
	if _, err := TravelFlee(m, m.ROM(), origin, policy, pcTravelBattles); err != nil {
		t.Fatalf("return to prepared Surf shoreline: %v", err)
	}
	if err := Face(m, origin.X, origin.Y-1); err != nil {
		t.Fatalf("face prepared water tile: %v", err)
	}
	result, err := UseFieldMove(m, FieldSurf)
	if err != nil {
		t.Fatalf("UseFieldMove(Surf) after roster repair: %v", err)
	}
	if !result.Surfing || m.Peek8(sym.WalkBikeSurfState) != fieldSurfingState {
		t.Fatalf("Surf result=%+v wWalkBikeSurfState=%d, want surfing state %d", result, m.Peek8(sym.WalkBikeSurfState), fieldSurfingState)
	}
}
