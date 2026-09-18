package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

func TestStructuredCampaignGoalsAreProfileNeutralForYellow(t *testing.T) {
	obs := Observation{
		GameID: yellowprofile.GameID,
		Badges: []string{"boulder", "cascade", "thunder", "rainbow", "soul", "marsh", "volcano", "earth"},
		Story: game.ProgressState{
			{ID: gen1.ProgressMainStoryComplete, Complete: true},
		},
	}
	status, structured, err := PlannerGoalStatus("elite-four", obs)
	if err != nil || !structured {
		t.Fatalf("Yellow elite-four goal parse: structured=%v err=%v", structured, err)
	}
	if !status.Complete {
		t.Fatalf("Yellow elite-four goal not complete: %+v", status)
	}

	status, structured, err = PlannerGoalStatus("badges:8", obs)
	if err != nil || !structured || !status.Complete {
		t.Fatalf("Yellow badge goal = %+v structured=%v err=%v", status, structured, err)
	}
}

func TestYellowDexGoalDoesNotAssumeRed151Target(t *testing.T) {
	obs := Observation{
		GameID: yellowprofile.GameID,
		Dex: DexCatalog{
			Owned:       []DexEntry{{Species: "pikachu"}},
			Targets:     []DexEntry{{Species: "eevee"}, {Species: "vaporeon"}},
			Unavailable: []DexEntry{{Species: "mew"}},
		},
	}
	status := EvaluateGoal(Goal{Kind: GoalDex}, obs)
	if status.Target != 3 || status.Current != 1 || status.Complete {
		t.Fatalf("Yellow Dex goal used fixed target instead of catalog: %+v", status)
	}
}
