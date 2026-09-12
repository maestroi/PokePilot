package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestPlanPartySlotLeavesOpenPartyAlone(t *testing.T) {
	slot, err := planPartySlot(nil, state.PartyState{Count: 3, Mons: []state.Mon{{}, {}, {}}}, state.BoxState{}, state.Mon{Species: 0x24}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if slot != -1 {
		t.Fatalf("open party deposit slot = %d, want -1 (no deposit)", slot)
	}
}

func TestPlanPartySlotBlocksOnFullBox(t *testing.T) {
	party := state.PartyState{Count: 6, Mons: []state.Mon{{}, {}, {}, {}, {}, {}}}
	box := state.BoxState{Count: gen1BoxCapacity}
	_, err := planPartySlot(nil, party, box, state.Mon{Species: 0x24}, nil)
	if !errors.Is(err, ErrPCBoxFull) {
		t.Fatalf("full box err = %v, want ErrPCBoxFull", err)
	}
}

func TestPlanPartySlotDepositsWeakestWhenNoFieldMovesRequired(t *testing.T) {
	party := state.PartyState{Count: 6, Mons: []state.Mon{
		{Species: 0x10, Level: 20, MaxHP: 60},
		{Species: 0x11, Level: 18, MaxHP: 50},
		{Species: 0x12, Level: 5, MaxHP: 20},
		{Species: 0x13, Level: 16, MaxHP: 40},
		{Species: 0x14, Level: 14, MaxHP: 35},
		{Species: 0x15, Level: 12, MaxHP: 30},
	}}
	slot, err := planPartySlot(nil, party, state.BoxState{Count: 1}, state.Mon{Species: 0x24}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if slot != 2 {
		t.Fatalf("deposit slot = %d, want weakest bench slot 2", slot)
	}
}
