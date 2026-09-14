package agent

import "testing"

func TestAppendRedNPCRewardObjectivesOffersLocalRodAsVerifiedPickup(t *testing.T) {
	obs := Observation{Map: 0xA3}
	got := appendRedNPCRewardObjectives(obs, nil, nil)
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
	if got := appendRedNPCRewardObjectives(obs, nil, nil); len(got) != 0 {
		t.Fatalf("owned rod still produced reward objectives: %+v", got)
	}
}

func TestAppendRedNPCRewardObjectivesGatesOaksAideByOwnedDex(t *testing.T) {
	obs := Observation{Map: 0x31, PokedexOwned: make([]SpeciesID, 9)}
	if got := appendRedNPCRewardObjectives(obs, nil, nil); len(got) != 0 {
		t.Fatalf("Route 2 aide offered below 10 owned: %+v", got)
	}

	obs.PokedexOwned = make([]SpeciesID, 10)
	got := appendRedNPCRewardObjectives(obs, nil, nil)
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
