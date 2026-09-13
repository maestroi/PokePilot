package agent

import (
	"math"
	"strings"
	"testing"
)

func TestAdventureOpportunityCostPrefersNearbyPickup(t *testing.T) {
	profile := PlayStyle("adventure")
	obs := Observation{X: 10, Y: 10, PartyCount: 2}
	near := Objective{Kind: KindPickup, Item: ItemID("potion"), X: 11, Y: 10}
	far := Objective{Kind: KindPickup, Item: ItemID("potion"), X: 34, Y: 10}

	nearScore := ScoreObjective(obs, near, profile)
	farScore := ScoreObjective(obs, far, profile)
	if nearScore.Cost.Total >= farScore.Cost.Total {
		t.Fatalf("near cost %.3f >= far cost %.3f", nearScore.Cost.Total, farScore.Cost.Total)
	}
	if nearScore.Total <= farScore.Total {
		t.Fatalf("near net %.3f <= far net %.3f", nearScore.Total, farScore.Total)
	}
	if nearScore.Cost.DistanceTiles >= farScore.Cost.DistanceTiles {
		t.Fatalf("near distance %d >= far distance %d", nearScore.Cost.DistanceTiles, farScore.Cost.DistanceTiles)
	}
}

func TestAdventureHighValuePickupCanJustifyLargerDetour(t *testing.T) {
	profile := PlayStyle("adventure")
	obs := Observation{X: 10, Y: 10, PartyCount: 2}
	farPotion := Objective{Kind: KindPickup, Item: ItemID("potion"), X: 34, Y: 10}
	farHM := Objective{Kind: KindPickup, Item: ItemID("hm01"), X: 34, Y: 10}
	progress := Objective{Kind: KindProgress, Progress: ProgressID("test_progress")}

	potionScore := ScoreObjective(obs, farPotion, profile)
	hmScore := ScoreObjective(obs, farHM, profile)
	progressScore := ScoreObjective(obs, progress, profile)

	if hmScore.Cost.Total != potionScore.Cost.Total {
		t.Fatalf("same detour has different cost: HM %.3f potion %.3f", hmScore.Cost.Total, potionScore.Cost.Total)
	}
	if hmScore.Value <= potionScore.Value {
		t.Fatalf("high-value HM gross %.3f <= potion gross %.3f", hmScore.Value, potionScore.Value)
	}
	if potionScore.Total >= progressScore.Total {
		t.Fatalf("far low-value potion net %.3f >= direct progress %.3f", potionScore.Total, progressScore.Total)
	}
	if hmScore.Total <= progressScore.Total {
		t.Fatalf("far high-value HM net %.3f <= direct progress %.3f", hmScore.Total, progressScore.Total)
	}
}

func TestOpportunityCostMechanismCoversAdventureObjectiveFamilies(t *testing.T) {
	profile := PlayStyle("adventure")
	obs := Observation{X: 10, Y: 10, PartyCount: 2}
	cases := []struct {
		name string
		obj  Objective
	}{
		{"item", Objective{Kind: KindPickup, Item: ItemID("potion"), X: 15, Y: 10}},
		{"npc", Objective{Kind: KindTalk, X: 15, Y: 10}},
		{"catch", Objective{Kind: KindCatch, Species: SpeciesID("pikachu")}},
		{"training", Objective{Kind: KindTrain, Level: 12}},
		{"shop", Objective{Kind: KindBuy, Item: ItemID("pokeball"), Qty: 5}},
		{"exploration", Objective{Kind: KindGoTo, Place: PlaceID("unknown frontier"), Note: "(unvisited adjacent map)"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			score := ScoreObjective(obs, tc.obj, profile)
			if score.Cost.Total <= 0 {
				t.Fatalf("cost = %.3f, want an explicit positive opportunity cost", score.Cost.Total)
			}
			if math.Abs(score.Total-(score.Value-score.Cost.Total)) > 1e-9 {
				t.Fatalf("net %.6f != gross %.6f - cost %.6f", score.Total, score.Value, score.Cost.Total)
			}
		})
	}
}

func TestOpportunityCostRaisesPressureNearExplicitRoundDeadline(t *testing.T) {
	profile := PlayStyle("adventure")
	pickup := Objective{Kind: KindPickup, Item: ItemID("potion"), X: 11, Y: 10}
	unlimited := Observation{X: 10, Y: 10, PartyCount: 2, RoundsLeft: 0}
	almostDone := unlimited
	almostDone.RoundsLeft = 2

	base := ScoreObjective(unlimited, pickup, profile)
	pressured := ScoreObjective(almostDone, pickup, profile)
	if pressured.Cost.GoalPressure <= 0 {
		t.Fatalf("deadline pressure = %.3f, want > 0", pressured.Cost.GoalPressure)
	}
	if pressured.Cost.Total <= base.Cost.Total {
		t.Fatalf("deadline cost %.3f <= unlimited cost %.3f", pressured.Cost.Total, base.Cost.Total)
	}
}

func TestOpportunityCostMarksRecentReturnAsBacktracking(t *testing.T) {
	profile := PlayStyle("adventure")
	obs := Observation{
		Location:   PlaceID("cerulean city"),
		PartyCount: 2,
		History: []RoundRecord{
			{Objective: "go to route 2", Outcome: "completed"},
			{Objective: "go to cerulean city", Outcome: "completed"},
		},
	}
	back := Objective{Kind: KindGoTo, Place: PlaceID("route 2")}

	score := ScoreObjective(obs, back, profile)
	if score.Cost.Backtracking <= 0 {
		t.Fatalf("backtracking cost = %.3f, want > 0", score.Cost.Backtracking)
	}
}

func TestAdventureAnnotationExposesNetAndOpportunityCost(t *testing.T) {
	obs := Observation{X: 10, Y: 10, PartyCount: 2}
	offered := []Objective{{Kind: KindPickup, Item: ItemID("potion"), X: 20, Y: 10}}

	got := AnnotatePlayStyle(obs, offered, PlayStyle("adventure"))
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if !strings.Contains(got[0].Note, "[adventure ") || !strings.Contains(got[0].Note, "cost ") {
		t.Fatalf("annotation %q does not expose Adventure net score and cost", got[0].Note)
	}
}
