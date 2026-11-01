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
