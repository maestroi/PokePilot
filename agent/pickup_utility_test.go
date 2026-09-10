package agent

import (
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
)

func TestEnhancePickupObjectivesMarksLowCaptureStockHighValue(t *testing.T) {
	ball, _ := ItemByName("pokeball")
	obs := Observation{X: 5, Y: 5, Bag: []Item{{Name: "pokeball", Quantity: 1}}}
	out := enhancePickupObjectives(nil, state.PartyState{}, obs, []Objective{{
		Kind: KindPickup, Item: ball, X: 6, Y: 5,
	}})
	if len(out) != 1 {
		t.Fatalf("enhanced pickups = %d, want 1", len(out))
	}
	for _, want := range []string{"high-value preparation", "nearby", "capture stock low", "free resupply", "¥200"} {
		if !strings.Contains(out[0].Note, want) {
			t.Errorf("pickup note %q does not contain %q", out[0].Note, want)
		}
	}
}

func TestEnhancePickupObjectivesMarksThinHealingStockHighValue(t *testing.T) {
	potion, _ := ItemByName("super potion")
	obs := Observation{
		X: 2, Y: 2,
		Party: []PartyMon{{Species: "wartortle", Level: 25, HP: 30, MaxHP: 72}},
	}
	out := enhancePickupObjectives(nil, state.PartyState{}, obs, []Objective{{
		Kind: KindPickup, Item: potion, X: 7, Y: 2,
	}})
	if len(out) != 1 {
		t.Fatalf("enhanced pickups = %d, want 1", len(out))
	}
	for _, want := range []string{"high-value preparation", "small local detour", "no HP-healing stock", "¥700"} {
		if !strings.Contains(out[0].Note, want) {
			t.Errorf("pickup note %q does not contain %q", out[0].Note, want)
		}
	}
}

func TestEnhancePickupObjectivesDoesNotTurnSituationalBattleItemIntoGoal(t *testing.T) {
	xAttack, _ := ItemByName("x attack")
	obs := Observation{X: 0, Y: 0}
	out := enhancePickupObjectives(nil, state.PartyState{}, obs, []Objective{{
		Kind: KindPickup, Item: xAttack, X: 20, Y: 10,
	}})
	if len(out) != 1 {
		t.Fatalf("enhanced pickups = %d, want 1", len(out))
	}
	if !strings.Contains(out[0].Note, "optional pickup") || !strings.Contains(out[0].Note, "longer same-map detour") {
		t.Fatalf("situational pickup was not framed as optional/costly: %q", out[0].Note)
	}
	if strings.Contains(out[0].Note, "high-value preparation") {
		t.Fatalf("situational pickup was overvalued: %q", out[0].Note)
	}
}

func TestEnhancePickupObjectivesMarksImmediatelyUsefulTMHighValue(t *testing.T) {
	const (
		typeNormal   = 0x00
		typeGrass    = 0x16
		typeElectric = 0x17
	)
	tackle := rom.Move{ID: 33, Power: 35, Type: typeNormal, Accuracy: 255, PP: 35}
	growl := rom.Move{ID: 45, Effect: rom.AttackDown1Effect, Accuracy: 255, PP: 40}
	leechSeed := rom.Move{ID: 73, Effect: 0x54, Accuracy: 229, PP: 10}
	vineWhip := rom.Move{ID: 22, Power: 35, Type: typeGrass, Accuracy: 255, PP: 10}
	thunderbolt := rom.Move{ID: 85, Power: 95, Type: typeElectric, Accuracy: 255, PP: 15}
	romData := plannerTMHMROM(t, rom.TM01Item, thunderbolt.ID, 0x99, 1, tackle, growl, leechSeed, vineWhip, thunderbolt)
	party := state.PartyState{Count: 1, Mons: []state.Mon{{
		Species: 0x99, Type1: typeGrass, Type2: 0x03,
		Moves: [4]uint8{tackle.ID, growl.ID, leechSeed.ID, vineWhip.ID},
	}}}
	tm, ok := ItemByName("tm01")
	if !ok {
		t.Fatal("TM01 missing from semantic item vocabulary")
	}
	out := enhancePickupObjectives(romData, party, Observation{X: 1, Y: 1}, []Objective{{
		Kind: KindPickup, Item: tm, X: 3, Y: 1,
	}})
	if len(out) != 1 {
		t.Fatalf("enhanced pickups = %d, want 1", len(out))
	}
	for _, want := range []string{"high-value preparation", "TM01", "one-time move resource", "usable now", "score"} {
		if !strings.Contains(out[0].Note, want) {
			t.Errorf("TM pickup note %q does not contain %q", out[0].Note, want)
		}
	}
}

func TestEnhancePickupObjectivesKeepsNonUpgradeTMUsefulButNotHighValue(t *testing.T) {
	const typeNormal = 0x00
	tackle := rom.Move{ID: 33, Power: 35, Type: typeNormal, Accuracy: 255, PP: 35}
	bodySlam := rom.Move{ID: 34, Power: 85, Type: typeNormal, Accuracy: 255, PP: 15}
	splash := rom.Move{ID: 150, Effect: 0x55, Accuracy: 255, PP: 40}
	romData := plannerTMHMROM(t, rom.TM01Item, splash.ID, 0x99, 1, tackle, bodySlam, splash)
	party := state.PartyState{Count: 1, Mons: []state.Mon{{
		Species: 0x99, Type1: typeNormal, Type2: typeNormal,
		Moves: [4]uint8{tackle.ID, bodySlam.ID},
	}}}
	tm, _ := ItemByName("tm01")
	out := enhancePickupObjectives(romData, party, Observation{}, []Objective{{Kind: KindPickup, Item: tm, X: 4, Y: 4}})
	if len(out) != 1 {
		t.Fatalf("enhanced pickups = %d, want 1", len(out))
	}
	for _, want := range []string{"useful preparation", "one-time move resource", "no current material move-set upgrade"} {
		if !strings.Contains(out[0].Note, want) {
			t.Errorf("TM pickup note %q does not contain %q", out[0].Note, want)
		}
	}
	if strings.Contains(out[0].Note, "high-value preparation") {
		t.Fatalf("non-upgrade TM was overvalued: %q", out[0].Note)
	}
}

func TestEnhancePickupObjectivesWithholdsNewKindWhenBagIsFull(t *testing.T) {
	bag := make([]Item, bagItemCapacity)
	for i := range bag {
		bag[i] = Item{Name: "occupied", Quantity: 1}
	}
	potion, _ := ItemByName("potion")
	out := enhancePickupObjectives(nil, state.PartyState{}, Observation{Bag: bag}, []Objective{{
		Kind: KindPickup, Item: potion, X: 1, Y: 1,
	}})
	if len(out) != 0 {
		t.Fatalf("new item kind offered with full bag: %+v", out)
	}
}

func TestEnhancePickupObjectivesAllowsStackWhenBagIsFull(t *testing.T) {
	bag := make([]Item, bagItemCapacity)
	for i := range bag {
		bag[i] = Item{Name: "occupied", Quantity: 1}
	}
	bag[0] = Item{Name: "potion", Quantity: 1}
	potion, _ := ItemByName("potion")
	out := enhancePickupObjectives(nil, state.PartyState{}, Observation{Bag: bag}, []Objective{{
		Kind: KindPickup, Item: potion, X: 1, Y: 1,
	}})
	if len(out) != 1 {
		t.Fatalf("stackable pickup withheld with full bag: %+v", out)
	}
}

func TestEnhancePickupObjectivesPreservesExistingHistoryNote(t *testing.T) {
	potion, _ := ItemByName("potion")
	out := enhancePickupObjectives(nil, state.PartyState{}, Observation{}, []Objective{{
		Kind: KindPickup, Item: potion, X: 1, Y: 1, Note: "(failed 1x)",
	}})
	if len(out) != 1 || !strings.Contains(out[0].Note, "(failed 1x)") || !strings.Contains(out[0].Note, "preparation") {
		t.Fatalf("existing pickup note not preserved/enhanced: %+v", out)
	}
}
