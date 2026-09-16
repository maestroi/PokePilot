package agent

import "testing"

func TestCompletionistCoveragePrefersNovelOptionalSurfaces(t *testing.T) {
	profile := PlayStyle(PlayStyleCompletionist)
	obs := Observation{
		PartyCount: 2,
		Party: []PartyMon{
			{Species: "squirtle", Level: 12, HP: 30, MaxHP: 30},
			{Species: "pidgey", Level: 9, HP: 24, MaxHP: 24},
		},
		PokedexOwned: []SpeciesID{"squirtle", "pidgey"},
	}
	progress := ScoreObjective(obs, Objective{Kind: KindProgress, Progress: "test_progress"}, profile)

	cases := []struct {
		name string
		obj  Objective
		tag  string
	}{
		{"new map", Objective{Kind: KindGoTo, Place: "route 3", Note: "(unvisited adjacent map)"}, "coverage-new-map"},
		{"new npc", Objective{Kind: KindTalk, X: 4, Y: 7}, "coverage-new-npc"},
		{"new trainer", Objective{Kind: KindTrainer, X: 8, Y: 3}, "coverage-new-trainer"},
		{"new item", Objective{Kind: KindPickup, Item: "tm01", X: 5, Y: 5}, "coverage-new-item"},
		{"new species", Objective{Kind: KindCatch, Species: "rattata"}, "coverage-new-species"},
		{"evolution", Objective{Kind: KindTrain, Species: "pidgey", Slot: 1, Level: 18, Intent: "dex-evolution"}, "coverage-evolution"},
		{"machine use", Objective{Kind: KindUseItem, Item: "tm01", Slot: 0}, "coverage-machine-use"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			score := ScoreObjective(obs, tc.obj, profile)
			if !naturalSignalHasTag(score.Natural, tc.tag) {
				t.Fatalf("score natural = %+v, want tag %q", score.Natural, tc.tag)
			}
			if score.Total <= progress.Total {
				t.Fatalf("novel completionist surface should beat generic progression: %s %.3f <= progress %.3f", tc.obj, score.Total, progress.Total)
			}
		})
	}
}

func TestCompletionistCoverageBacksOffUnderSafetyPressure(t *testing.T) {
	profile := PlayStyle(PlayStyleCompletionist)
	obj := Objective{Kind: KindTrainer, X: 8, Y: 3}

	healthy := Observation{Party: []PartyMon{{Species: "squirtle", HP: 30, MaxHP: 30}}}
	hurt := Observation{Party: []PartyMon{{Species: "squirtle", HP: 4, MaxHP: 30}}}

	healthySignal := completionistCoverageSignal(healthy, obj, profile)
	hurtSignal := completionistCoverageSignal(hurt, obj, profile)
	if healthySignal.Bonus <= 0 {
		t.Fatalf("healthy coverage bonus = %.3f, want positive", healthySignal.Bonus)
	}
	if hurtSignal.Bonus <= 0 || hurtSignal.Bonus >= healthySignal.Bonus {
		t.Fatalf("hurt coverage bonus = %.3f, healthy = %.3f; want reduced but positive", hurtSignal.Bonus, healthySignal.Bonus)
	}
}

func TestCompletionistCoverageDoesNotLeakIntoOtherStyles(t *testing.T) {
	obj := Objective{Kind: KindTalk, X: 4, Y: 7}
	for _, style := range []string{PlayStyleSpeedrun, PlayStyleAdventure, PlayStyleTeamBuilder} {
		signal := completionistCoverageSignal(Observation{}, obj, PlayStyle(style))
		if signal.Bonus != 0 || len(signal.Tags) != 0 {
			t.Fatalf("%s received completionist coverage signal %+v", style, signal)
		}
	}
}
