package gen1

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestEarlyCampaignStagesOrder(t *testing.T) {
	want := []game.ProgressID{
		ProgressPokedexAcquired,
		ProgressBoulderBadge,
		ProgressMtMoonFossilAcquired,
		ProgressSSTicketAcquired,
		ProgressHM01Acquired,
	}
	got := EarlyCampaignStages()
	if len(got) != len(want) {
		t.Fatalf("EarlyCampaignStages length=%d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("EarlyCampaignStages[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}


func TestMiddleCampaignStagesOrder(t *testing.T) {
	want := []game.ProgressID{
		ProgressCascadeBadge,
		ProgressThunderBadge,
		ProgressPostSurgeLavenderReached,
		ProgressPostSurgeCeladonReady,
		ProgressRainbowBadge,
	}
	got := MiddleCampaignStages()
	if len(got) != len(want) {
		t.Fatalf("MiddleCampaignStages length=%d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("MiddleCampaignStages[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestLeagueStagesOrder(t *testing.T) {
	want := []game.ProgressID{
		ProgressLeagueChallengeStarted,
		ProgressLeagueLoreleiDefeated,
		ProgressLeagueBrunoDefeated,
		ProgressLeagueAgathaDefeated,
		ProgressLeagueLanceDefeated,
		ProgressLeagueChampionDefeated,
		ProgressMainStoryComplete,
	}
	got := LeagueStages()
	if len(got) != len(want) {
		t.Fatalf("LeagueStages length=%d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("LeagueStages[%d]=%q, want %q", i, got[i], want[i])
		}
	}
	got[0] = "mutated"
	if LeagueStages()[0] != ProgressLeagueChallengeStarted {
		t.Fatal("LeagueStages returned mutable global storage")
	}
}

func TestFirstIncomplete(t *testing.T) {
	stages := LeagueApproachStages()
	state := game.ProgressState{
		{ID: ProgressRoute22RivalResolved, Complete: true},
		{ID: ProgressRoute23BadgeChecks, Complete: true},
	}
	got, ok := FirstIncomplete(state, stages)
	if !ok || got != ProgressVictoryRoadCleared {
		t.Fatalf("FirstIncomplete=%q,%v want %q,true", got, ok, ProgressVictoryRoadCleared)
	}

	for _, id := range stages {
		state = append(state, game.ProgressFact{ID: id, Complete: true})
	}
	if got, ok := FirstIncomplete(state, stages); ok || got != "" {
		t.Fatalf("completed FirstIncomplete=%q,%v want empty,false", got, ok)
	}
}
