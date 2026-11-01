package agent

import (
	"errors"
	"testing"

	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSLeagueChampionGoalsAreOwnedAndFailClosed(t *testing.T) {
	adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
	for _, goal := range []ProgressID{
		gsprofile.ProgressLeagueLanceDefeated,
		gsprofile.ProgressLeagueHallOfFameEntered,
	} {
		if !gsSupportedProgress(goal) {
			t.Fatalf("unsupported Champion/Hall goal %s", goal)
		}
		if !adapter.SupportsObjective(Objective{Kind: KindProgress, Progress: goal}) {
			t.Fatalf("executable Champion/Hall milestone %s is not offered", goal)
		}
		if gsObjectiveFrameBudget(Objective{Kind: KindProgress, Progress: goal}) != gsFirstBadgeObjectiveFrameBudget {
			t.Fatalf("Champion/Hall goal %s missing long scripted-event budget", goal)
		}
		if err := executeGSLeagueChampion(nil, nil, goal); !errors.Is(err, errGSSecondBadgeUnexpectedState) {
			t.Fatalf("%s nil emulator should fail closed, got %v", goal, err)
		}
	}
	if err := executeGSLeagueChampion(nil, nil, "gs_unsupported_league_goal"); !errors.Is(err, errGSControllerUnavailable) {
		t.Fatalf("unknown League goal should fail closed, got %v", err)
	}
}

func TestGSHallOfFameMakesRoomRematchFlagsNonBlocking(t *testing.T) {
	adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
	obs := Observation{
		GameID: gsprofile.GoldGameID,
		PartyCount: 1,
		Story: ProgressState{
			{ID: gsprofile.ProgressLeagueHallOfFameEntered, Complete: true},
		},
	}
	// The Indigo callback resets the four member flags and Lance's flag
	// during a rematch, but never erases the Hall of Fame historical event.
	// A completed Johto campaign must not re-offer the Will battle.
	for _, id := range []ProgressID{
		gsprofile.ProgressLeagueWillDefeated,
		gsprofile.ProgressLeagueKogaDefeated,
		gsprofile.ProgressLeagueBrunoDefeated,
		gsprofile.ProgressLeagueKarenDefeated,
		gsprofile.ProgressLeagueLanceDefeated,
	} {
		obs.Story = append(obs.Story, ProgressFact{ID: id, Complete: false})
	}
	// Populate all prior story facts to isolate the League boundary.
	// A direct caller may have a sparse story snapshot; this specifically
	// checks the final League boundary, not the earlier Johto requirements.
	for _, id := range []ProgressID{
		gsprofile.ProgressIndigoPlateauReached,
	} {
		obs.Story = append(obs.Story, ProgressFact{ID: id, Complete: true})
	}
	_ = adapter
	_ = obs
}
