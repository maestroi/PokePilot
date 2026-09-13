package agent

import (
	"strings"
	"testing"
)

func TestAdventureFrontierExplorationCanOutrankDirectProgression(t *testing.T) {
	profile := PlayStyle("adventure")
	obs := Observation{Map: 0xff, PartyCount: 2}
	frontier := Objective{Kind: KindGoTo, Place: PlaceID("viridian city"), Flee: true, Note: "(unvisited adjacent map)"}
	progress := Objective{Kind: KindProgress, Progress: ProgressID("test_progress")}

	frontierScore := ScoreObjective(obs, frontier, profile)
	progressScore := ScoreObjective(obs, progress, profile)
	if frontierScore.Total <= progressScore.Total {
		t.Fatalf("frontier score %.3f <= direct progression %.3f", frontierScore.Total, progressScore.Total)
	}
	if !hasNaturalTag(frontierScore.Natural, "frontier") {
		t.Fatalf("frontier natural tags = %v, want frontier", frontierScore.Natural.Tags)
	}
}

func TestAdventureHealthyCenterHealIsDeprioritized(t *testing.T) {
	profile := PlayStyle("adventure")
	obs := Observation{
		MapName:    "Viridian Pokemon Center",
		PartyCount: 1,
		Party:      []PartyMon{{Species: SpeciesID("squirtle"), Level: 8, HP: 28, MaxHP: 28}},
	}
	heal := Objective{Kind: KindHeal}
	progress := Objective{Kind: KindProgress, Progress: ProgressID("test_progress")}

	healScore := ScoreObjective(obs, heal, profile)
	progressScore := ScoreObjective(obs, progress, profile)
	if healScore.Natural.RepeatPenalty <= 0 || !hasNaturalTag(healScore.Natural, "already-recovered") {
		t.Fatalf("healthy heal natural signal = %#v, want already-recovered penalty", healScore.Natural)
	}
	if healScore.Total >= progressScore.Total {
		t.Fatalf("healthy heal %.3f >= progression %.3f", healScore.Total, progressScore.Total)
	}
}

func TestAdventureTownRoutineSignalsRecoveryNPCAndResupply(t *testing.T) {
	profile := PlayStyle("adventure")
	obs := Observation{
		Map:        0xff,
		X:          5,
		Y:          5,
		PartyCount: 2,
		Party: []PartyMon{
			{Species: SpeciesID("squirtle"), Level: 14, HP: 8, MaxHP: 40},
			{Species: SpeciesID("pidgey"), Level: 8, HP: 20, MaxHP: 20},
		},
		Money:     2000,
		HasGrass:  true,
		WildGrass: []WildSpecies{{Name: "rattata", MinLevel: 3, MaxLevel: 5, Slots: 10}},
	}

	heal := ScoreObjective(obs, Objective{Kind: KindHeal}, profile)
	if !hasNaturalTag(heal.Natural, "consolidate-recovery") {
		t.Fatalf("heal tags = %v, want consolidate-recovery", heal.Natural.Tags)
	}

	talk := ScoreObjective(obs, Objective{Kind: KindTalk, X: 6, Y: 5}, profile)
	if !hasNaturalTag(talk.Natural, "new-npc") || !hasNaturalTag(talk.Natural, "nearby") {
		t.Fatalf("talk tags = %v, want new-npc+nearby", talk.Natural.Tags)
	}

	mart := ScoreObjective(obs, Objective{Kind: KindGoTo, Place: PlaceID("viridian mart")}, profile)
	if !hasNaturalTag(mart.Natural, "resupply-stop") {
		t.Fatalf("mart tags = %v, want resupply-stop", mart.Natural.Tags)
	}

	buy := ScoreObjective(obs, Objective{Kind: KindBuy, Item: ItemID("pokeball"), Qty: 5}, profile)
	if !hasNaturalTag(buy.Natural, "needed-resupply") {
		t.Fatalf("buy tags = %v, want needed-resupply", buy.Natural.Tags)
	}
}

func TestAdventureRouteRoutineRewardsNearbyItemsTeamBuildingAndCatchUp(t *testing.T) {
	profile := PlayStyle("adventure")
	obs := Observation{
		Map:        0xff,
		X:          5,
		Y:          5,
		PartyCount: 2,
		Party: []PartyMon{
			{Species: SpeciesID("wartortle"), Level: 24, HP: 60, MaxHP: 60},
			{Species: SpeciesID("pikachu"), Level: 15, HP: 35, MaxHP: 35},
		},
	}

	pickup := ScoreObjective(obs, Objective{
		Kind: KindPickup, Item: ItemID("tm24"), X: 7, Y: 5,
		Note: "(high-value preparation; nearby; one-time move resource)",
	}, profile)
	if !hasNaturalTag(pickup.Natural, "high-value-item") || !hasNaturalTag(pickup.Natural, "nearby") || !hasNaturalTag(pickup.Natural, "durable-upgrade") {
		t.Fatalf("pickup tags = %v", pickup.Natural.Tags)
	}

	catchScore := ScoreObjective(obs, Objective{Kind: KindCatch, Species: SpeciesID("oddish")}, profile)
	if !hasNaturalTag(catchScore.Natural, "build-party") || !hasNaturalTag(catchScore.Natural, "local-encounter") {
		t.Fatalf("catch tags = %v", catchScore.Natural.Tags)
	}

	train := ScoreObjective(obs, Objective{Kind: KindTrain, Species: SpeciesID("pikachu"), Slot: 1, Level: 17}, profile)
	if !hasNaturalTag(train.Natural, "catch-up-party") {
		t.Fatalf("training tags = %v, want catch-up-party", train.Natural.Tags)
	}

	trainer := ScoreObjective(obs, Objective{Kind: KindTrainer, X: 10, Y: 5}, profile)
	if !hasNaturalTag(trainer.Natural, "useful-training") {
		t.Fatalf("trainer tags = %v, want useful-training", trainer.Natural.Tags)
	}
}

func TestAdventureRepeatedOptionalObjectiveLosesNovelty(t *testing.T) {
	profile := PlayStyle("adventure")
	talk := Objective{Kind: KindTalk, X: 6, Y: 5}
	freshObs := Observation{X: 5, Y: 5, PartyCount: 2}
	repeatedObs := freshObs
	repeatedObs.History = []RoundRecord{
		{Objective: talk.String(), Outcome: "completed"},
		{Objective: talk.String(), Outcome: "completed"},
	}

	fresh := ScoreObjective(freshObs, talk, profile)
	repeated := ScoreObjective(repeatedObs, talk, profile)
	if repeated.Natural.RepeatPenalty <= 0 || !hasNaturalTag(repeated.Natural, "recent-repeat") {
		t.Fatalf("repeat signal = %#v, want recent-repeat penalty", repeated.Natural)
	}
	if repeated.Total >= fresh.Total {
		t.Fatalf("repeated talk %.3f >= fresh talk %.3f", repeated.Total, fresh.Total)
	}
}

func TestOfferDoesNotReofferAlreadyTalkedPerson(t *testing.T) {
	known := NewKnowledge(nil)
	known.TalkedTo(0xfe, 3, 4)
	obs := Observation{
		Map:        0xfe,
		MapName:    "test map",
		PartyCount: 1,
		Party:      []PartyMon{{Species: SpeciesID("squirtle"), Level: 8, HP: 24, MaxHP: 24}},
		MapObjects: []MapObject{{X: 3, Y: 4, Kind: "person"}},
	}

	for _, o := range Offer(obs, known) {
		if o.Kind == KindTalk && o.X == 3 && o.Y == 4 {
			t.Fatalf("Offer re-offered already exhausted NPC: %#v", o)
		}
	}
}

func TestAdventureAnnotationIncludesNaturalReason(t *testing.T) {
	obs := Observation{X: 5, Y: 5, PartyCount: 2}
	offered := []Objective{{Kind: KindTalk, X: 6, Y: 5}}
	got := AnnotatePlayStyle(obs, offered, PlayStyle("adventure"))
	if len(got) != 1 || !strings.Contains(got[0].Note, "natural +") || !strings.Contains(got[0].Note, "new-npc") {
		t.Fatalf("Adventure annotation = %#v, want natural-play reason", got)
	}
}

func TestNaturalPlayIsDisabledForSpeedrun(t *testing.T) {
	obs := Observation{X: 5, Y: 5, PartyCount: 2}
	score := ScoreObjective(obs, Objective{Kind: KindTalk, X: 6, Y: 5}, PlayStyle("speedrun"))
	if score.Natural.Bonus != 0 || score.Natural.RepeatPenalty != 0 || len(score.Natural.Tags) != 0 {
		t.Fatalf("speedrun natural signal = %#v, want zero", score.Natural)
	}
}

func hasNaturalTag(s NaturalPlaySignal, want string) bool {
	for _, tag := range s.Tags {
		if tag == want {
			return true
		}
	}
	return false
}
