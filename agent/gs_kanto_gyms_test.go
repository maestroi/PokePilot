package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSKantoGymAndStoryCatalogRunsThroughAllEightBadges(t *testing.T) {
	chain := []game.ProgressID{
		gsprofile.ProgressKantoMarshBadgeEarned,
		gsprofile.ProgressKantoRainbowBadgeEarned,
		gsprofile.ProgressKantoSoulBadgeEarned,
		gsprofile.ProgressKantoManagerMet,
		gsprofile.ProgressKantoRocketAtGym,
		gsprofile.ProgressKantoMachinePartFound,
		gsprofile.ProgressKantoPowerRestored,
		gsprofile.ProgressKantoMistyReturned,
		gsprofile.ProgressKantoCascadeBadgeEarned,
		gsprofile.ProgressKantoExpansionCard,
		gsprofile.ProgressKantoSnorlaxCleared,
		gsprofile.ProgressKantoBoulderBadgeEarned,
		gsprofile.ProgressKantoVolcanoBadgeEarned,
		gsprofile.ProgressKantoBlueInvited,
		gsprofile.ProgressKantoEarthBadgeEarned,
	}
	for _, id := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		adapter := newGSObjectiveAdapter(nil, nil, id)
		obs := Observation{GameID: id, Story: ProgressState{
			{ID: gsprofile.ProgressKantoThunderBadgeEarned, Complete: true},
		}}
		for _, want := range chain {
			goals := gsKantoGymObjectives(obs)
			if len(goals) != 1 || goals[0].Progress != want {
				t.Fatalf("%s: next Kanto objective = %+v, want %s", id, goals, want)
			}
			if !gsSupportedProgress(want) || !adapter.SupportsObjective(goals[0]) {
				t.Fatalf("%s: unsupported Kanto stage %s", id, want)
			}
			if got := gsObjectiveFrameBudget(goals[0]); got != gsFirstBadgeObjectiveFrameBudget {
				t.Fatalf("%s: stage %s frame budget = %d", id, want, got)
			}
			obs.Story = append(obs.Story, ProgressFact{ID: want, Complete: true})
		}
		if got := gsKantoGymObjectives(obs); len(got) != 1 ||
			got[0].Progress != gsprofile.ProgressMtSilverOpened {
			t.Fatalf("%s: eight Kanto badges must offer Oak's Mt. Silver permit: %+v", id, got)
		}
		goal, recognized, err := PlannerGoalStatus("progress:gs_kanto_all_badges_earned", obs)
		if err != nil || !recognized || goal.Complete {
			t.Fatalf("%s: full Kanto goal completed without all-eight fact: %+v, %v", id, goal, err)
		}
		obs.Story = append(obs.Story, ProgressFact{ID: gsprofile.ProgressKantoAllBadgesEarned, Complete: true})
		obs.Story = append(obs.Story, ProgressFact{ID: gsprofile.ProgressMtSilverOpened, Complete: true})
		if got := gsKantoGymObjectives(obs); len(got) != 1 ||
			got[0].Progress != gsprofile.ProgressMtSilverRedDefeated {
			t.Fatalf("%s: Oak permit must offer Red at Mt. Silver: %+v", id, got)
		}
		obs.Story = append(obs.Story, ProgressFact{ID: gsprofile.ProgressMtSilverRedDefeated, Complete: true})
		if got := gsKantoGymObjectives(obs); len(got) != 0 {
			t.Fatalf("%s: Red victory should end the offered main story: %+v", id, got)
		}
		goal, recognized, err = PlannerGoalStatus("progress:gs_kanto_all_badges_earned", obs)
		if err != nil || !recognized || !goal.Complete {
			t.Fatalf("%s: all eight badge fact did not finish goal: %+v, %v", id, goal, err)
		}
		redGoal, recognized, err := PlannerGoalStatus("progress:gs_mt_silver_red_defeated", obs)
		if err != nil || !recognized || !redGoal.Complete {
			t.Fatalf("%s: Red event did not finish final goal: %+v, %v", id, redGoal, err)
		}
	}
}

func TestGSKantoGymSpecsReferToActualRetailMaps(t *testing.T) {
	for _, spec := range gsKantoGymStages {
		if spec.progress == "" || spec.previous == "" || spec.x != spec.sx ||
			spec.y+1 != spec.sy || spec.leader == "" {
			t.Fatalf("incomplete or nonadjacent Kanto leader spec: %+v", spec)
		}
		if _, ok := gsKantoGymForProgress(spec.progress); !ok {
			t.Fatalf("cannot dispatch Kanto Gym spec %s", spec.progress)
		}
		for _, name := range []string{spec.gym, spec.center} {
			id, err := gsOpeningMapID(name)
			if err != nil || !gsSecondBadgeOwnedMap(id) {
				t.Fatalf("%s not mapped and owned: map=%#04x err=%v", name, id, err)
			}
		}
	}
	if _, ok := gsKantoGymForProgress("gs_kanto_unimplemented"); ok {
		t.Fatal("unknown leader must not default to an existing Gym")
	}
}

func TestGSKantoStoryExecutorsNeverManufactureResultsOnNilEmulator(t *testing.T) {
	for _, f := range []func(*emu.Emu, []byte) error{
		executeGSKantoPowerManager,
		executeGSKantoRocketAtGym,
		executeGSKantoMachinePart,
		executeGSKantoRestorePower,
		executeGSKantoMistyDate,
		executeGSKantoInviteBlue,
	} {
		if err := f(nil, nil); err == nil {
			t.Fatal("Kanto story executor accepted a nil emulator")
		}
	}
	if err := executeGSKantoGym(nil, nil, gsprofile.ProgressKantoMarshBadgeEarned); err == nil {
		t.Fatal("Gym executor accepted a nil emulator")
	}
}
