package agent

import (
	"testing"

	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSLakeOfRageStoryTargetsRetailNPCAndShinyEncounter(t *testing.T) {
	if gsRedGyaradosX != 18 || gsRedGyaradosY != 22 ||
		gsRedGyaradosStandX != 18 || gsRedGyaradosStandY != 23 ||
		gsLakeLanceX != 21 || gsLakeLanceY != 28 ||
		gsLakeLanceStandX != 21 || gsLakeLanceStandY != 29 {
		t.Fatal("lake coordinates diverged from pret/pokegold/maps/LakeOfRage.asm")
	}
	for _, name := range []string{"MAHOGANY_TOWN", "MAHOGANY_POKECENTER_1F", "LAKE_OF_RAGE"} {
		id, err := gsOpeningMapID(name)
		if err != nil {
			t.Fatal(err)
		}
		if !gsSecondBadgeOwnedMap(id) {
			t.Fatalf("Lake of Rage script interruptions not owned on %s", name)
		}
	}
	from, err := gsOpeningMapID("LAKE_OF_RAGE")
	if err != nil {
		t.Fatal(err)
	}
	center, err := gsOwnedPokecenterID(from)
	if err != nil {
		t.Fatal(err)
	}
	want, err := gsOpeningMapID("MAHOGANY_POKECENTER_1F")
	if err != nil {
		t.Fatal(err)
	}
	if center != want {
		t.Fatalf("Lake of Rage recovery center = %#04x, want Mahogany %#04x", center, want)
	}
}

func TestGSLakeOfRageProgressionOffersExecutableStagesAfterJasmine(t *testing.T) {
	for _, gameID := range []string{string(gsprofile.GoldGameID), string(gsprofile.SilverGameID)} {
		adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
		if gameID == string(gsprofile.SilverGameID) {
			adapter = newGSObjectiveAdapter(nil, nil, gsprofile.SilverGameID)
		}
		obs := Observation{GameID: adapter.gameID, PartyCount: 1,
			Story: ProgressState{{ID: gsprofile.ProgressStarterReceived, Complete: true}}}
		foundRed := false
		foundLance := false
		for stage := 0; stage < 40; stage++ {
			goals := adapter.ProgressionObjectives(obs)
			if len(goals) == 0 {
				break
			}
			next := goals[0]
			if next.Progress == gsprofile.ProgressRedGyaradosDefeated {
				foundRed = true
				if !obs.Story.Has(gsprofile.ProgressMineralBadgeEarned) {
					t.Fatal("Red Gyarados was offered before Mineral Badge")
				}
			}
			if next.Progress == gsprofile.ProgressLanceHelpAccepted {
				foundLance = true
				if !obs.Story.Has(gsprofile.ProgressRedGyaradosDefeated) {
					t.Fatal("Lance was offered before Red Gyarados")
				}
			}
			obs.Story = append(obs.Story, ProgressFact{ID: next.Progress, Complete: true})
		}
		if !foundRed || !foundLance {
			t.Fatalf("%s milestones not offered: red=%v lance=%v", gameID, foundRed, foundLance)
		}
		if !obs.Story.Has(gsprofile.ProgressMahoganyStairsOpened) {
			t.Fatalf("%s never offered Mahogany Mart staircase reveal", gameID)
		}
		if got := adapter.ProgressionObjectives(obs); len(got) != 0 {
			t.Fatalf("%s must stop safely before the unfinished Rocket Hideout battles: %+v", gameID, got)
		}
	}
}
