package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
)

var gen2TestMedicines = []game.Medicine{
	{NativeItemID: 0x10, HP: 200},
	{NativeItemID: 0x0e, HP: game.MedicineFullHP, Cures: game.MedicineCuresAny},
}

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
		// Gen-II numbering: 0x0e is FULL_RESTORE, 0x10 is HYPER_POTION
		// (0x10 is FULL_RESTORE in Gen I). Policy must follow the profile.
		Medicines: gen2TestMedicines,
		Bag: []game.InventoryItem{
			{NativeItemID: 0x10, Quantity: 1},
			{NativeItemID: 0x0e, Quantity: 1},
		},
	}

	choice, ok := chooseBattleMedicineState(resources)
	if !ok || choice.Item != 0x0e || choice.Slot != 0 {
		t.Fatalf("medicine = %+v,%v want Gen-II FULL RESTORE 0x0e on active slot", choice, ok)
	}
	hyperOnly := resources
	hyperOnly.Bag = []game.InventoryItem{{NativeItemID: 0x10, Quantity: 1}}
	if choice, ok := chooseBattleMedicineState(hyperOnly); !ok || choice.Item != 0x10 || choice.Reason != "active HP 8/30" {
		t.Fatalf("Gen-II 0x10 = %+v,%v want an HP-only HYPER POTION turn", choice, ok)
	}
	none := resources
	none.Medicines = nil
	if choice, ok := chooseBattleMedicineState(none); ok {
		t.Fatalf("no profile medicine table chose %+v; generic policy must not guess ids", choice)
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
			Moves:           [4]game.BattlePartyMove{{NativeMoveID: 400, PP: 3}},
		}},
		Bag: []game.InventoryItem{{NativeItemID: 500, Quantity: 2}},
	}
	if resources.Party[0].NativeSpeciesID != 300 ||
		resources.Party[0].Moves[0].NativeMoveID != 400 ||
		resources.ItemQuantity(500) != 2 {
		t.Fatalf("portable resources narrowed native ids: %+v", resources)
	}
}

func TestSacrificialBenchHealRestoresMateriallyStrongerCarry(t *testing.T) {
	strategy := &fakeGen2CombatStrategy{
		scores: map[uint16]int64{1: 100, 2: 1000},
	}
	resources := game.BattleResourcesState{
		InBattle:   true,
		ActiveSlot: 0,
		Party: []game.BattlePartyMon{
			{
				NativeSpeciesID: 1,
				Level:           30,
				HP:              60,
				MaxHP:           60,
				Moves:           [4]game.BattlePartyMove{{NativeMoveID: 1, PP: 10}},
			},
			{
				NativeSpeciesID: 2,
				Level:           50,
				HP:              10,
				MaxHP:           60,
				Moves:           [4]game.BattlePartyMove{{NativeMoveID: 2, PP: 10}},
			},
		},
		Bag:       []game.InventoryItem{{NativeItemID: uint16(itemSuperPotion), Quantity: 1}},
		Medicines: gen1.Medicines,
	}
	battle := game.BattleState{
		ActiveLevel: 30,
		ActiveHP:    60,
		ActiveMaxHP: 60,
		EnemyLevel:  50,
		EnemyHP:     100,
		EnemyMaxHP:  100,
		Moves:       [4]game.BattleMove{{ID: 1, PP: 10}},
	}

	choice, ok := chooseSacrificialBenchHealWithStrategy(strategy, nil, resources, battle)
	if !ok {
		t.Fatal("sacrificial bench heal = no choice, want recovery turn")
	}
	if choice.Item != itemSuperPotion || choice.Slot != 1 {
		t.Fatalf("sacrificial bench heal = %+v, want SUPER POTION on slot 1", choice)
	}
}

func TestSacrificialBenchHealRejectsComparableBench(t *testing.T) {
	strategy := &fakeGen2CombatStrategy{
		scores: map[uint16]int64{1: 100, 2: 140},
	}
	resources := game.BattleResourcesState{
		InBattle:   true,
		ActiveSlot: 0,
		Party: []game.BattlePartyMon{
			{
				NativeSpeciesID: 1,
				Level:           30,
				HP:              60,
				MaxHP:           60,
				Moves:           [4]game.BattlePartyMove{{NativeMoveID: 1, PP: 10}},
			},
			{
				NativeSpeciesID: 2,
				Level:           30,
				HP:              10,
				MaxHP:           60,
				Moves:           [4]game.BattlePartyMove{{NativeMoveID: 2, PP: 10}},
			},
		},
		Bag: []game.InventoryItem{{NativeItemID: uint16(itemSuperPotion), Quantity: 1}},
	}
	battle := game.BattleState{
		ActiveLevel: 30,
		ActiveHP:    60,
		ActiveMaxHP: 60,
		EnemyLevel:  30,
		EnemyHP:     60,
		EnemyMaxHP:  60,
		Moves:       [4]game.BattleMove{{ID: 1, PP: 10}},
	}

	if choice, ok := chooseSacrificialBenchHealWithStrategy(strategy, nil, resources, battle); ok {
		t.Fatalf("sacrificial bench heal = %+v, true; want no sacrifice for comparable bench", choice)
	}
}

func TestSacrificialBenchHealRequiresCriticalBenchHP(t *testing.T) {
	strategy := &fakeGen2CombatStrategy{
		scores: map[uint16]int64{1: 100, 2: 1000},
	}
	resources := game.BattleResourcesState{
		InBattle:   true,
		ActiveSlot: 0,
		Party: []game.BattlePartyMon{
			{
				NativeSpeciesID: 1,
				Level:           30,
				HP:              60,
				MaxHP:           60,
				Moves:           [4]game.BattlePartyMove{{NativeMoveID: 1, PP: 10}},
			},
			{
				NativeSpeciesID: 2,
				Level:           50,
				HP:              20,
				MaxHP:           60,
				Moves:           [4]game.BattlePartyMove{{NativeMoveID: 2, PP: 10}},
			},
		},
		Bag: []game.InventoryItem{{NativeItemID: uint16(itemSuperPotion), Quantity: 1}},
	}
	battle := game.BattleState{
		ActiveLevel: 30,
		ActiveHP:    60,
		ActiveMaxHP: 60,
		EnemyLevel:  50,
		EnemyHP:     100,
		EnemyMaxHP:  100,
		Moves:       [4]game.BattleMove{{ID: 1, PP: 10}},
	}

	if choice, ok := chooseSacrificialBenchHealWithStrategy(strategy, nil, resources, battle); ok {
		t.Fatalf("sacrificial bench heal = %+v, true; want target above critical HP left alone", choice)
	}
}

// A fainted, much stronger carry (Meganium one-shot by Red's Charizard) is
// revived while a weak bench mon holds the field.
func TestSacrificialTurnRevivesFaintedCarry(t *testing.T) {
	strategy := &fakeGen2CombatStrategy{scores: map[uint16]int64{1: 100, 2: 1000}}
	resources := game.BattleResourcesState{
		InBattle:   true,
		ActiveSlot: 0,
		Party: []game.BattlePartyMon{
			{NativeSpeciesID: 1, Level: 17, HP: 60, MaxHP: 60, Moves: [4]game.BattlePartyMove{{NativeMoveID: 1, PP: 10}}},
			{NativeSpeciesID: 2, Level: 95, HP: 0, MaxHP: 322, Moves: [4]game.BattlePartyMove{{NativeMoveID: 2, PP: 10}}},
		},
		Bag:       []game.InventoryItem{{NativeItemID: 0x35, Quantity: 1}},
		Medicines: gen1.Medicines,
	}
	battle := game.BattleState{ActiveLevel: 17, ActiveHP: 60, ActiveMaxHP: 60, EnemyLevel: 77, EnemyHP: 200, EnemyMaxHP: 200,
		Moves: [4]game.BattleMove{{ID: 1, PP: 10}}}
	choice, ok := chooseSacrificialBenchHealWithStrategy(strategy, nil, resources, battle)
	if !ok || choice.Item != 0x35 || choice.Slot != 1 {
		t.Fatalf("choice = %+v,%v; want REVIVE on the fainted carry", choice, ok)
	}
	resources.Bag = nil
	if choice, ok := chooseSacrificialBenchHealWithStrategy(strategy, nil, resources, battle); ok {
		t.Fatalf("no revive in the bag chose %+v", choice)
	}
}

func TestSacrificialReviveSkipsUnderleveledBench(t *testing.T) {
	strategy := &fakeGen2CombatStrategy{scores: map[uint16]int64{1: 100, 2: 1000}}
	resources := game.BattleResourcesState{
		InBattle: true, ActiveSlot: 0,
		Party: []game.BattlePartyMon{
			{NativeSpeciesID: 1, Level: 5, HP: 19, MaxHP: 19, Moves: [4]game.BattlePartyMove{{NativeMoveID: 1, PP: 10}}},
			{NativeSpeciesID: 2, Level: 17, HP: 0, MaxHP: 60, Moves: [4]game.BattlePartyMove{{NativeMoveID: 2, PP: 10}}},
		},
		Bag:       []game.InventoryItem{{NativeItemID: 0x35, Quantity: 1}},
		Medicines: gen1.Medicines,
	}
	battle := game.BattleState{ActiveLevel: 5, ActiveHP: 19, ActiveMaxHP: 19, EnemyLevel: 77, EnemyHP: 200, EnemyMaxHP: 200,
		Moves: [4]game.BattleMove{{ID: 1, PP: 10}}}
	if choice, ok := chooseSacrificialBenchHealWithStrategy(strategy, nil, resources, battle); ok {
		t.Fatalf("revived an underleveled bench mon: %+v", choice)
	}
}
