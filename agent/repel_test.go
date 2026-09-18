package agent

import "testing"

func TestRepelProviderOffersUseOnlyOnEncounterMapWithoutActiveEffect(t *testing.T) {
	provider := repelObjectiveProvider{}
	ctx := &objectiveOfferContext{obs: Observation{
		WildGrass: []WildSpecies{{Name: "zubat", MinLevel: 6, MaxLevel: 11, Slots: 10}},
		Bag:       []Item{{Name: "repel", Quantity: 1}, {Name: "super repel", Quantity: 1}},
	}}
	got := provider.Provide(ctx).Candidates
	if len(got) != 1 {
		t.Fatalf("repel candidates = %v, want one", got)
	}
	if got[0].Intent != speedrunRepelUseIntent || got[0].Item != "super repel" || got[0].Slot != -1 {
		t.Fatalf("repel objective = %+v, want targetless SUPER REPEL use", got[0])
	}

	ctx.obs.RepelSteps = 42
	if got := provider.Provide(ctx).Candidates; len(got) != 0 {
		t.Fatalf("active repel still offered use: %v", got)
	}
}

func TestRepelObjectivesAreSpeedrunSpecific(t *testing.T) {
	repel := Objective{Kind: KindUseItem, Item: "repel", Slot: -1, Intent: speedrunRepelUseIntent}
	progress := Objective{Kind: KindProgress, Progress: "test_progress"}
	offered := []Objective{repel, progress}

	speed := AnnotatePlayStyle(Observation{}, offered, PlayStyle(PlayStyleSpeedrun))
	if len(speed) != 2 {
		t.Fatalf("speedrun offered %d objectives, want 2", len(speed))
	}
	adventure := AnnotatePlayStyle(Observation{}, offered, PlayStyle(PlayStyleAdventure))
	if len(adventure) != 1 || adventure[0].Kind != KindProgress {
		t.Fatalf("adventure offered = %+v, want repel objective removed", adventure)
	}
}

func TestFightEverythingRemovesRepelAvoidance(t *testing.T) {
	offered := []Objective{
		{Kind: KindUseItem, Item: "repel", Slot: -1, Intent: speedrunRepelUseIntent},
		{Kind: KindBuy, Item: "super repel", Qty: 2, Intent: speedrunRepelBuyIntent},
		{Kind: KindGoTo, Place: "route 3", Flee: true},
	}
	got := ApplyRunPolicy(Observation{}, offered, "", WildEncountersFight)
	if len(got) != 1 || got[0].Kind != KindGoTo || got[0].Flee {
		t.Fatalf("fight-everything policy = %+v, want only non-flee travel", got)
	}
}

func TestRepelObjectiveValidationIsTargetless(t *testing.T) {
	if err := (Objective{Kind: KindUseItem, Item: "repel", Slot: -1, Intent: speedrunRepelUseIntent}).Validate(); err != nil {
		t.Fatalf("targetless repel validation: %v", err)
	}
	if err := (Objective{Kind: KindUseItem, Item: "potion", Slot: -1}).Validate(); err == nil {
		t.Fatal("ordinary targetless field item unexpectedly validated")
	}
}

func TestSpeedrunMarksCrossMapFleeTravelForRepel(t *testing.T) {
	obs := Observation{Map: 0x00, MapName: "PALLET_TOWN"}
	offered := []Objective{
		{Kind: KindGoTo, Place: "viridian city", Flee: true},
		{Kind: KindGoTo, Place: "viridian city"},
	}
	got := AnnotatePlayStyle(obs, offered, PlayStyle(PlayStyleSpeedrun))
	if len(got) != 2 || !got[0].RepelBeforeTravel || got[1].RepelBeforeTravel {
		t.Fatalf("speedrun travel repel hints = %+v, want only cross-map flee travel marked", got)
	}

	obs.MapName = "SAFARI_ZONE_CENTER"
	got = AnnotatePlayStyle(obs, offered, PlayStyle(PlayStyleSpeedrun))
	if got[0].RepelBeforeTravel {
		t.Fatalf("Safari travel was marked for bag Repel use: %+v", got[0])
	}
}
