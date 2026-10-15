package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestAppendRedNPCRewardObjectivesOffersLocalRodAsVerifiedPickup(t *testing.T) {
	obs := Observation{Map: 0xA3}
	got := appendRedNPCRewardObjectives(nil, obs, nil, nil)
	if len(got) != 1 {
		t.Fatalf("reward objectives = %d, want 1: %+v", len(got), got)
	}
	o := got[0]
	if o.Kind != KindPickup || o.X != 2 || o.Y != 4 || o.Item != ItemID("old rod") || o.Intent != redNPCRewardIntent {
		t.Fatalf("old rod reward objective = %+v", o)
	}
}

func TestAppendRedNPCRewardObjectivesDoesNotReofferOwnedRod(t *testing.T) {
	obs := Observation{
		Map: 0xA3,
		Bag: []Item{{Name: "old rod", Quantity: 1}},
	}
	if got := appendRedNPCRewardObjectives(nil, obs, nil, nil); len(got) != 0 {
		t.Fatalf("owned rod still produced reward objectives: %+v", got)
	}
}

// TestAppendRedNPCRewardObjectivesDoesNotReofferGrantedRodRetiredToPC is the
// regression for run-d6dokr184ky81: the good rod was granted (BIT_GOT_GOOD_ROD
// set) and then stored in the Player PC by bag pressure, so it is absent from
// the bag. The gate must not re-offer it: the ROM will not re-grant, and
// receiveChoiceReward would hard-fail on a postcondition the game already
// satisfied.
func TestAppendRedNPCRewardObjectivesDoesNotReofferGrantedRodRetiredToPC(t *testing.T) {
	var mem state.Mem
	mem[sym.StatusFlags1] |= 1 << 4 // BIT_GOT_GOOD_ROD
	obs := Observation{Map: 0xA4}   // fuchsia good rod house
	if got := appendRedNPCRewardObjectives(&mem, obs, nil, nil); len(got) != 0 {
		t.Fatalf("granted good rod (retired to PC) still produced reward objectives: %+v", got)
	}
}

// The positive control: with no grant flag set and no rod in the bag, the
// local good-rod reward is offered as a verified pickup.
func TestAppendRedNPCRewardObjectivesOffersUngrantedRod(t *testing.T) {
	var mem state.Mem             // no grant flags set
	obs := Observation{Map: 0xA4} // fuchsia good rod house
	got := appendRedNPCRewardObjectives(&mem, obs, nil, nil)
	if len(got) != 1 {
		t.Fatalf("ungranted good rod objectives = %d, want 1: %+v", len(got), got)
	}
	o := got[0]
	if o.Kind != KindPickup || o.X != 5 || o.Y != 3 || o.Item != ItemID("good rod") || o.Intent != redNPCRewardIntent {
		t.Fatalf("good rod reward objective = %+v", o)
	}
}

func TestAppendRedNPCRewardObjectivesGatesOaksAideByOwnedDex(t *testing.T) {
	obs := Observation{Map: 0x31, PokedexOwned: make([]SpeciesID, 9)}
	if got := appendRedNPCRewardObjectives(nil, obs, nil, nil); len(got) != 0 {
		t.Fatalf("Route 2 aide offered below 10 owned: %+v", got)
	}

	obs.PokedexOwned = make([]SpeciesID, 10)
	got := appendRedNPCRewardObjectives(nil, obs, nil, nil)
	if len(got) != 1 {
		t.Fatalf("Route 2 aide objectives = %d, want 1: %+v", len(got), got)
	}
	o := got[0]
	if o.Kind != KindPickup || o.X != 1 || o.Y != 4 || o.Item != ItemID("hm05") || o.Intent != redNPCRewardIntent {
		t.Fatalf("Route 2 aide reward objective = %+v", o)
	}
}

func TestFilterRedServiceTalkObjectivesSuppressesRewardAndPewterGuideChoices(t *testing.T) {
	rod := []Objective{
		{Kind: KindTalk, X: 2, Y: 4},
		{Kind: KindTalk, X: 1, Y: 1},
	}
	got := filterRedServiceTalkObjectives(nil, Observation{Map: 0xA3}, rod)
	if len(got) != 1 || got[0].X != 1 || got[0].Y != 1 {
		t.Fatalf("rod guru filter = %+v, want only ordinary NPC", got)
	}

	guide := []Objective{
		{Kind: KindTalk, X: pewterGymGuideHomeX, Y: pewterGymGuideHomeY},
		{Kind: KindTalk, X: 1, Y: 1},
	}
	got = filterRedServiceTalkObjectives(nil, Observation{Map: pewterGymMapID}, guide)
	if len(got) != 1 || got[0].X != 1 || got[0].Y != 1 {
		t.Fatalf("Pewter guide filter = %+v, want only ordinary NPC", got)
	}
}
