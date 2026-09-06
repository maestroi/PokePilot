package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func leagueTestROM(t *testing.T, ppByMove map[uint8]uint8) []byte {
	t.Helper()
	const movesOffset = 0x0e * 0x4000
	romData := make([]byte, movesOffset+256*6)
	for id, pp := range ppByMove {
		if id == 0 {
			continue
		}
		off := movesOffset + (int(id)-1)*6
		romData[off] = id
		romData[off+5] = pp
	}
	return romData
}

func leagueMon(hp, maxHP uint16, status uint8, move uint8, pp uint8) state.Mon {
	return state.Mon{
		Species: 1, Level: 50, HP: hp, MaxHP: maxHP, Status: status,
		Moves: [4]uint8{move}, PP: [4]uint8{pp},
	}
}

func TestPlanLeagueResourcesPrefersFreeCenterBeforeFiniteItems(t *testing.T) {
	romData := leagueTestROM(t, map[uint8]uint8{1: 20})
	party := state.PartyState{Count: 1, Mons: []state.Mon{leagueMon(40, 100, 0, 1, 3)}}
	inv := state.InventoryState{Items: []state.BagItem{
		{ID: itemHyperPotion, Quantity: 5},
		{ID: itemRevive, Quantity: 2},
		{ID: itemEther, Quantity: 4},
	}}
	policy := DefaultLeagueResourcePolicy(LeagueBattleCount, len(party.Mons))
	policy.FreeCenterAvailable = true

	plan, err := PlanLeagueResources(romData, party, inv, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != LeagueUseCenter {
		t.Fatalf("actions = %+v, want exactly one free Center heal", plan.Actions)
	}
	if got := plan.Reserved[itemHyperPotion]; got != 5 {
		t.Fatalf("reserved Hyper Potions = %d, want all 5 untouched", got)
	}
	if got := plan.Reserved[itemEther]; got != 4 {
		t.Fatalf("reserved Ethers = %d, want all 4 untouched", got)
	}
	if !plan.After.Viable || !plan.After.FullyRecovered {
		t.Fatalf("projected Center state = %+v, want fully recovered and viable", plan.After)
	}
}

func TestPlanLeagueResourcesFairSharesHealingAndPPAcrossFutureFights(t *testing.T) {
	romData := leagueTestROM(t, map[uint8]uint8{1: 20, 2: 20, 3: 20})
	party := state.PartyState{Count: 3, Mons: []state.Mon{
		leagueMon(10, 100, 0, 1, 0),
		leagueMon(15, 100, 0, 2, 0),
		leagueMon(100, 100, 0, 3, 20),
	}}
	inv := state.InventoryState{Items: []state.BagItem{
		{ID: itemHyperPotion, Quantity: 8},
		{ID: itemEther, Quantity: 4},
	}}
	policy := DefaultLeagueResourcePolicy(4, len(party.Mons))

	plan, err := PlanLeagueResources(romData, party, inv, policy)
	if err != nil {
		t.Fatal(err)
	}
	var heals, ppRestores int
	for _, action := range plan.Actions {
		switch action.Kind {
		case LeagueHealHP:
			heals++
		case LeagueRestorePP:
			ppRestores++
		}
	}
	if heals != 2 {
		t.Fatalf("healing actions = %d, want fair share ceil(8/4)=2: %+v", heals, plan.Actions)
	}
	if ppRestores != 1 {
		t.Fatalf("PP actions = %d, want fair share ceil(4/4)=1: %+v", ppRestores, plan.Actions)
	}
	if got := plan.Reserved[itemHyperPotion]; got != 6 {
		t.Fatalf("reserved Hyper Potions = %d, want 6 for later fights", got)
	}
	if got := plan.Reserved[itemEther]; got != 3 {
		t.Fatalf("reserved Ethers = %d, want 3 for later fights", got)
	}
	if !plan.After.Viable {
		t.Fatalf("projected state = %+v, want viable after bounded fair-share actions", plan.After)
	}
}

func TestPlanLeagueResourcesRevivesOnlyToConfiguredLiveFloor(t *testing.T) {
	romData := leagueTestROM(t, map[uint8]uint8{1: 20})
	party := state.PartyState{Count: 3, Mons: []state.Mon{
		leagueMon(100, 100, 0, 1, 20),
		leagueMon(0, 140, 0, 1, 20),
		leagueMon(0, 80, 0, 1, 20),
	}}
	inv := state.InventoryState{Items: []state.BagItem{{ID: itemRevive, Quantity: 4}}}
	policy := DefaultLeagueResourcePolicy(4, len(party.Mons))
	policy.MinimumLiveMons = 2

	plan, err := PlanLeagueResources(romData, party, inv, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != LeagueRevive {
		t.Fatalf("actions = %+v, want exactly one revive", plan.Actions)
	}
	if plan.Actions[0].Slot != 1 {
		t.Fatalf("revived slot = %d, want deterministic strongest fainted slot 1", plan.Actions[0].Slot)
	}
	if got := plan.Reserved[itemRevive]; got != 3 {
		t.Fatalf("reserved Revives = %d, want 3", got)
	}
	if !plan.After.Viable {
		t.Fatalf("projected state = %+v, want viable with two live mons", plan.After)
	}
}

func TestPlanLeagueResourcesReportsInsufficientWithoutSpendingGuesswork(t *testing.T) {
	romData := leagueTestROM(t, map[uint8]uint8{1: 20})
	party := state.PartyState{Count: 1, Mons: []state.Mon{leagueMon(1, 100, 0, 1, 0)}}
	policy := DefaultLeagueResourcePolicy(3, len(party.Mons))

	plan, err := PlanLeagueResources(romData, party, state.InventoryState{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 0 {
		t.Fatalf("actions = %+v, want no invented recovery action without items", plan.Actions)
	}
	if plan.After.Viable {
		t.Fatalf("projected state = %+v, want non-viable", plan.After)
	}
}

func TestLeagueSequenceProgressNeverCompletesOnIntermediateWin(t *testing.T) {
	p := LeagueSequenceProgress{}
	for i := 0; i < LeagueBattleCount-1; i++ {
		p = p.WithWin()
		if p.Complete() {
			t.Fatalf("progress completed after only %d win(s): %+v", p.Wins, p)
		}
	}
	p = p.WithWin()
	if p.Complete() {
		t.Fatalf("five Battle wins without Champion event reported complete: %+v", p)
	}

	var mem state.Mem
	e := state.EventBeatChampionRival
	mem[sym.EventFlags+uint16(e)/8] |= 1 << (uint16(e) % 8)
	p = ConfirmLeagueChampion(&mem, p)
	if !p.Complete() {
		t.Fatalf("Champion event + five wins did not complete progress: %+v", p)
	}
	if got := p.EncountersRemaining(); got != 0 {
		t.Fatalf("encounters remaining = %d, want 0", got)
	}
}

func TestFieldItemHadEffectRecognizesRevival(t *testing.T) {
	before := state.Mon{HP: 0, MaxHP: 100}
	after := state.Mon{HP: 50, MaxHP: 100}
	if !fieldItemHadEffect(before, after) {
		t.Fatal("0 HP -> live HP was not recognized as a field-item effect")
	}
}
