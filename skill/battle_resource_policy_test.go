package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestBattleResourcePolicyAcceptsGen2ShapedState(t *testing.T) {
	resources := game.BattleResourcesState{
		InBattle:   true,
		ActiveSlot: 0,
		Party: []game.BattlePartyMon{
			{
				NativeSpeciesID: 152,
				Level:           18,
				HP:              8,
				MaxHP:           30,
				Status:          "poisoned",
				Type1:           12,
				Type2:           12,
				Moves: [4]game.BattlePartyMove{
					{NativeMoveID: 33, PP: 0},
				},
			},
			{
				NativeSpeciesID: 251,
				Level:           22,
				HP:              50,
				MaxHP:           50,
				Type1:           14,
				Type2:           14,
				SpecialAttack:   120,
				SpecialDefense:  110,
				Moves: [4]game.BattlePartyMove{
					{NativeMoveID: 251, PP: 5},
				},
			},
		},
		Bag: []game.InventoryItem{
			{NativeItemID: uint16(itemFullRestore), Quantity: 1},
		},
	}

	choice, ok := chooseBattleMedicineState(resources)
	if !ok || choice.Item != itemFullRestore || choice.Slot != 0 {
		t.Fatalf("medicine = %+v,%v want FULL RESTORE on active slot", choice, ok)
	}
	if slot, ok := resources.PPRecoverySlot(); !ok || slot != 1 {
		t.Fatalf("PPRecoverySlot = %d,%v want 1,true", slot, ok)
	}
	if got := resources.Party[1].NativeSpeciesID; got != 251 {
		t.Fatalf("species narrowed: got %d want 251", got)
	}
	if got := resources.Party[1].Moves[0].NativeMoveID; got != 251 {
		t.Fatalf("move narrowed: got %d want 251", got)
	}
}

func TestBattleResourcesPreserveWideNativeIDs(t *testing.T) {
	resources := game.BattleResourcesState{
		Party: []game.BattlePartyMon{{
			NativeSpeciesID: 300,
			Type1:           301,
			Type2:           302,
			Moves: [4]game.BattlePartyMove{{NativeMoveID: 400, PP: 3}},
		}},
		Bag: []game.InventoryItem{{NativeItemID: 500, Quantity: 2}},
	}
	if resources.Party[0].NativeSpeciesID != 300 ||
		resources.Party[0].Moves[0].NativeMoveID != 400 ||
		resources.ItemQuantity(500) != 2 {
		t.Fatalf("portable resources narrowed native ids: %+v", resources)
	}
}
