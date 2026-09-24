package agent

import "testing"

func testTrainingAreaAssessment(place PlaceID, location LocationID, encounters, travel, recovery int, method TrainingMethod, viability TrainingViability) TrainingAreaAssessment {
	assessment := TrainingAreaAssessment{
		Place: place, Location: location, Routable: true, RecoveryKnown: true,
		TravelCost: travel, RecoveryCost: recovery,
		Estimate: TrainingEstimate{
			CurrentLevel: 20, TargetLevel: 22, XPRemaining: 1000,
			XPPerEncounter: 200, EstimatedEncounters: encounters,
			SessionBudget: trainSessionBattleBudget, Viability: viability, Method: method,
		},
	}
	if method == TrainingSwitch {
		assessment.Estimate.CarryLevel = 35
	}
	finalizeTrainingAreaCost(&assessment)
	return assessment
}

func TestTrainingAreaRankingRejectsUnsafeHighXPArea(t *testing.T) {
	unsafe := testTrainingAreaAssessment("victory road", "victory", 2, 50, 100, TrainingDirect, TrainingOutsideBudget)
	unsafe.Estimate.XPPerEncounter = 900
	safe := testTrainingAreaAssessment("route 15", "route15", 5, 100, 100, TrainingDirect, TrainingViable)

	ranked := rankTrainingAreaAssessments([]TrainingAreaAssessment{unsafe, safe})
	if len(ranked) != 2 || ranked[0].Place != "route 15" || !ranked[0].Selected {
		t.Fatalf("ranking = %+v, want safe viable area selected", ranked)
	}
	if ranked[1].Selected {
		t.Fatalf("unsafe area was selected: %+v", ranked[1])
	}
}

func TestTrainingAreaRankingBalancesXPAgainstTravelAndRecovery(t *testing.T) {
	near := testTrainingAreaAssessment("near grass", "near", 6, 50, 100, TrainingDirect, TrainingViable)
	better := testTrainingAreaAssessment("better grass", "better", 3, 150, 100, TrainingDirect, TrainingViable)

	ranked := rankTrainingAreaAssessments([]TrainingAreaAssessment{near, better})
	if ranked[0].Place != "better grass" || !ranked[0].Selected {
		t.Fatalf("ranking = %+v, want materially better XP area to justify extra travel", ranked)
	}
	if ranked[0].TotalCost >= ranked[1].TotalCost {
		t.Fatalf("costs = %+v, expected selected area to have lower total cost", ranked)
	}
}

func TestTrainingAreaRankingAccountsForSwitchTrainingCost(t *testing.T) {
	direct := testTrainingAreaAssessment("direct grass", "direct", 4, 100, 100, TrainingDirect, TrainingViable)
	switching := testTrainingAreaAssessment("switch grass", "switch", 4, 100, 100, TrainingSwitch, TrainingViable)

	ranked := rankTrainingAreaAssessments([]TrainingAreaAssessment{switching, direct})
	if ranked[0].Place != "direct grass" {
		t.Fatalf("ranking = %+v, want direct area to win otherwise-equal switch overhead", ranked)
	}
}

func TestTrainingAreaRankingPenalizesMissingRecoveryHub(t *testing.T) {
	safeHub := testTrainingAreaAssessment("with center", "hub", 4, 100, 100, TrainingDirect, TrainingViable)
	noHub := testTrainingAreaAssessment("no center", "remote", 4, 50, 0, TrainingDirect, TrainingViable)
	noHub.RecoveryKnown = false
	finalizeTrainingAreaCost(&noHub)

	ranked := rankTrainingAreaAssessments([]TrainingAreaAssessment{noHub, safeHub})
	if ranked[0].Place != "with center" {
		t.Fatalf("ranking = %+v, want sustainable area with known recovery", ranked)
	}
}

func TestTrainingAreaRankingTieBreaksDeterministically(t *testing.T) {
	zeta := testTrainingAreaAssessment("zeta", "z", 4, 100, 100, TrainingDirect, TrainingViable)
	alpha := testTrainingAreaAssessment("alpha", "a", 4, 100, 100, TrainingDirect, TrainingViable)

	ranked := rankTrainingAreaAssessments([]TrainingAreaAssessment{zeta, alpha})
	if ranked[0].Place != "alpha" || !ranked[0].Selected {
		t.Fatalf("ranking = %+v, want alphabetical deterministic tie-break", ranked)
	}
}

func TestBestKnownTrainingPlaceUsesDynamicSelectedAssessment(t *testing.T) {
	current := LocationID("current")
	legacyHigh := LocationID("legacy-high")
	exactBest := LocationID("exact-best")
	known := NewKnowledge(KnowledgeTopology{Adjacency: map[LocationID][]LocationID{
		current:    {legacyHigh, exactBest},
		legacyHigh: {current},
		exactBest:  {current},
	}})
	known.TrainingAreas[legacyHigh] = TrainingAreaKnowledge{
		Location: legacyHigh, Place: "legacy high", MinLevel: 35, MaxLevel: 40,
	}
	known.TrainingAreas[exactBest] = TrainingAreaKnowledge{
		Location: exactBest, Place: "exact best", MinLevel: 20, MaxLevel: 24,
	}
	estimate := TrainingEstimate{
		CurrentLevel: 20, TargetLevel: 22, XPPerEncounter: 300,
		EstimatedEncounters: 3, SessionBudget: 20, Viability: TrainingViable, Method: TrainingDirect,
	}
	obs := Observation{
		Location: PlaceID(current),
		Party:    []PartyMon{{Species: "testmon", Level: 20, HP: 50, MaxHP: 50}},
		TrainingAreaChoices: []TrainingAreaAssessment{{
			Place: "exact best", Location: exactBest, Selected: true, Routable: true,
			TravelCost: 100, RecoveryCost: 100, TotalCost: 450, Estimate: estimate,
		}},
	}
	choice, ok := bestKnownTrainingPlace(obs, known, []string{"legacy high", "exact best"}, ObjectiveCatalog{})
	if !ok {
		t.Fatal("no training choice")
	}
	if choice.Area.Place != "exact best" || choice.Estimate == nil {
		t.Fatalf("choice = %+v, want exact dynamic selection", choice)
	}
}

func TestTrainingAreaTargetTracksCombatReadinessGap(t *testing.T) {
	obs := Observation{
		PartyCount: 1,
		Party:      []PartyMon{{Species: "testmon", Level: 20, HP: 50, MaxHP: 50}},
	}
	known := NewKnowledge(nil)
	challenge := Objective{Kind: KindGym, Place: "test gym"}
	known.Failures[combatLossFailureKey(challenge)] = Failure{
		Objective: challenge.String(), Times: 1,
		ReadinessBaseline: 80, ReadinessTarget: 100,
	}
	if got := trainingAreaTargetLevel(obs, known); got != 25 {
		t.Fatalf("target level = %d, want 25 to close readiness 80->100", got)
	}
}
