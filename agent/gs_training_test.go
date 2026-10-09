package agent

import (
	"testing"

	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSTrainingOfferedOnlyWhileCombatReadinessIsOwed(t *testing.T) {
	obs := Observation{
		Party: []PartyMon{{Species: "bayleef", Level: 31}, {Species: "togepi", Level: 35}},
		Story: ProgressState{{ID: gsprofile.ProgressSlowpokeWellCleared, Complete: true}, {ID: gsprofile.ProgressSudowoodoCleared, Complete: true}},
	}
	if got := gsTrainingObjectives(obs); len(got) != 0 {
		t.Fatalf("training offered without a combat loss: %+v", got)
	}
	obs.CombatLossRecorded = true
	got := gsTrainingObjectives(obs)
	if len(got) != 1 || got[0].Kind != KindTrain || got[0].Level != 31+trainStep || got[0].Validate() != nil {
		t.Fatalf("training = %+v, want train the lead to %d", got, 31+trainStep)
	}
	if spot, _ := gsTrainingSpot(obs.Story); spot.mapName != "BURNED_TOWER_1F" {
		t.Fatalf("grind spot = %s, want the strongest open spot", spot.mapName)
	}
	obs.Story = nil
	if got := gsTrainingObjectives(obs); len(got) != 0 {
		t.Fatalf("training offered with no grind spot open: %+v", got)
	}
}
