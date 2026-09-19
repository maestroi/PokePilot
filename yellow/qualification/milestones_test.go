package qualification

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

func TestCampaignMilestonesAreOrderedAndSemantic(t *testing.T) {
	ms := CampaignMilestones()
	if len(ms) < 20 {
		t.Fatalf("milestones = %d, want staged full campaign", len(ms))
	}
	if ms[0].ID != "fresh_overworld" || ms[len(ms)-1].ID != "hall_of_fame" {
		t.Fatalf("milestone endpoints = %q..%q", ms[0].ID, ms[len(ms)-1].ID)
	}
}

func TestEvaluateStopsAtFirstMissingRequiredMilestone(t *testing.T) {
	obs := game.ProfileObservation{
		NativeMapID: 0x26,
		Controllable: true,
		Badges: []string{"Boulder", "Cascade", "Thunder"},
		Story: game.ProgressState{
			{ID: yellowprofile.ProgressYellowStarterReceived, Complete: true},
			{ID: yellowprofile.ProgressYellowLabRivalResolved, Complete: true},
			{ID: gen1.ProgressPokedexAcquired, Complete: true},
			// Deliberately omit Yellow's Mt. Moon Jessie/James exit.
			{ID: gen1.ProgressMainStoryComplete, Complete: true},
		},
	}
	got := Evaluate(obs)
	if got.Next == nil || got.Next.ID != "mt_moon_exit" {
		t.Fatalf("status = %s, want first missing Mt. Moon milestone", got.String())
	}
	if got.Reached != 5 {
		t.Fatalf("reached = %d, want 5", got.Reached)
	}
}

func TestEvaluateCompleteCampaign(t *testing.T) {
	story := game.ProgressState{}
	for _, id := range []game.ProgressID{
		yellowprofile.ProgressYellowStarterReceived,
		yellowprofile.ProgressYellowLabRivalResolved,
		gen1.ProgressPokedexAcquired,
		yellowprofile.ProgressYellowMtMoonExitResolved,
		yellowprofile.ProgressYellowRocketJessieJamesDefeated,
		yellowprofile.ProgressYellowTowerJessieJamesDefeated,
		gen1.ProgressPokeFluteAcquired,
		gen1.ProgressFuchsiaProgressionComplete,
		yellowprofile.ProgressYellowSilphJessieJamesDefeated,
		gen1.ProgressSilphRescueComplete,
		gen1.ProgressRoute23BadgeChecks,
		gen1.ProgressVictoryRoadCleared,
		gen1.ProgressLeagueChallengeStarted,
		gen1.ProgressLeagueChampionDefeated,
		gen1.ProgressMainStoryComplete,
	} {
		story = append(story, game.ProgressFact{ID: id, Complete: true})
	}
	obs := game.ProfileObservation{
		NativeMapID: 0x26,
		Controllable: true,
		Badges: []string{"1","2","3","4","5","6","7","8"},
		Story: story,
	}
	got := Evaluate(obs)
	if got.Next != nil || got.Reached != got.Total {
		t.Fatalf("complete status = %s", got.String())
	}
}
