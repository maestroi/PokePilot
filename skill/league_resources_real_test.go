package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

// TestLeagueResourceItemsRealROM is #111's opt-in prepared-state qualification
// for the two item classes that ordinary field medicine did not previously
// expose semantically: revival and PP recovery. The external state must be
// controllable in the overworld, contain at least one fainted party member,
// carry REVIVE or MAX REVIVE, and contain at least one live member with a PP
// deficit plus an Ether/Elixer-family item.
//
// The state is intentionally external: .state files are ROM-derived artifacts
// and are never committed. Set POKEPILOT_LEAGUE_RESOURCE_TEST_STATE in local or
// private ROM-backed CI.
func TestLeagueResourceItemsRealROM(t *testing.T) {
	m := loadPreparedFieldActionState(t, "POKEPILOT_LEAGUE_RESOURCE_TEST_STATE")

	var before state.Mem
	state.Snapshot(m, &before)
	party := state.DecodeParty(&before)
	fainted := -1
	for slot, mon := range party.Mons {
		if mon.Fainted() {
			fainted = slot
			break
		}
	}
	if fainted < 0 {
		t.Fatal("prepared League resource state has no fainted party member")
	}

	_, reviveQty := bagEntry(&before, itemRevive)
	_, maxReviveQty := bagEntry(&before, itemMaxRevive)
	useMax := false
	if reviveQty == 0 {
		if maxReviveQty == 0 {
			t.Fatal("prepared League resource state has no REVIVE or MAX REVIVE")
		}
		useMax = true
	}
	if err := RevivePartyMember(m, fainted, useMax); err != nil {
		t.Fatalf("RevivePartyMember(slot=%d,max=%v): %v", fainted, useMax, err)
	}

	var revived state.Mem
	state.Snapshot(m, &revived)
	if state.DecodeParty(&revived).Mons[fainted].Fainted() {
		t.Fatalf("slot %d is still fainted after verified revive", fainted)
	}

	ppItem := uint8(0)
	for _, item := range []uint8{itemEther, itemMaxEther, itemElixer, itemMaxElixer} {
		if _, qty := bagEntry(&revived, item); qty > 0 {
			ppItem = item
			break
		}
	}
	if ppItem == 0 {
		t.Fatal("prepared League resource state has no Ether/Elixer-family item")
	}

	party = state.DecodeParty(&revived)
	ppSlot := -1
	for slot, mon := range party.Mons {
		if mon.Fainted() {
			continue
		}
		for i, moveID := range mon.Moves {
			if moveID == 0 {
				continue
			}
			move, err := rom.LookupMove(m.ROM(), moveID)
			if err != nil {
				t.Fatalf("lookup move %d: %v", moveID, err)
			}
			if mon.PP[i] < move.PP {
				ppSlot = slot
				break
			}
		}
		if ppSlot >= 0 {
			break
		}
	}
	if ppSlot < 0 {
		t.Fatal("prepared League resource state has no live party member with a base-PP deficit")
	}

	beforePP := party.Mons[ppSlot].PP
	if err := RestorePartyPP(m, ppItem, ppSlot); err != nil {
		t.Fatalf("RestorePartyPP(item=%#02x,slot=%d): %v", ppItem, ppSlot, err)
	}
	var after state.Mem
	state.Snapshot(m, &after)
	afterPP := state.DecodeParty(&after).Mons[ppSlot].PP
	increased := false
	for i := range beforePP {
		if afterPP[i] > beforePP[i] {
			increased = true
			break
		}
	}
	if !increased {
		t.Fatalf("PP did not increase on slot %d: %v -> %v", ppSlot, beforePP, afterPP)
	}
}
