package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func leagueStageObservation() Observation {
	return Observation{
		Map:        0xAE,
		PartyCount: 1,
		Story: ProgressState{
			{ID: redProgressIndigoPlateauReady, Complete: true},
		},
	}
}

func TestOfferEliteFourStagesInOrderAfterIndigoReady(t *testing.T) {
	obs := leagueStageObservation()
	stages := []ProgressID{
		ProgressLeagueChallengeStarted,
		redProgressLeagueLoreleiDefeated,
		redProgressLeagueBrunoDefeated,
		redProgressLeagueAgathaDefeated,
		redProgressLeagueLanceDefeated,
		ProgressLeagueChampionDefeated,
		ProgressMainStoryComplete,
	}
	for i, want := range stages {
		got := redProgressionObjectives(obs)
		if !offeredProgressID(got, want) {
			t.Fatalf("League stage %q missing at step %d: %v", want, i, got)
		}
		for _, later := range stages[i+1:] {
			if offeredProgressID(got, later) {
				t.Fatalf("later League stage %q leaked before %q: %v", later, want, got)
			}
		}
		obs.Story = append(obs.Story, ProgressFact{ID: want, Complete: true})
	}
	if got := redProgressionObjectives(obs); offeredProgressID(got, ProgressMainStoryComplete) {
		t.Fatalf("Hall of Fame stage re-offered after main-story completion: %v", got)
	}
}

func TestEliteFourStagesWaitForIndigoReadiness(t *testing.T) {
	before := Observation{Map: 0x01, PartyCount: 1}
	for _, id := range []ProgressID{
		ProgressLeagueChallengeStarted,
		redProgressLeagueLoreleiDefeated,
		redProgressLeagueBrunoDefeated,
		redProgressLeagueAgathaDefeated,
		redProgressLeagueLanceDefeated,
		ProgressLeagueChampionDefeated,
		ProgressMainStoryComplete,
	} {
		if got := redProgressionObjectives(before); offeredProgressID(got, id) {
			t.Fatalf("League stage %q offered before Indigo readiness: %v", id, got)
		}
	}
}

func TestChampionCompletionKeepsHallOfFameStageActive(t *testing.T) {
	obs := leagueStageObservation()
	obs.Story = append(obs.Story,
		ProgressFact{ID: ProgressLeagueChallengeStarted, Complete: true},
		ProgressFact{ID: redProgressLeagueLoreleiDefeated, Complete: true},
		ProgressFact{ID: redProgressLeagueBrunoDefeated, Complete: true},
		ProgressFact{ID: redProgressLeagueAgathaDefeated, Complete: true},
		ProgressFact{ID: redProgressLeagueLanceDefeated, Complete: true},
		ProgressFact{ID: ProgressLeagueChampionDefeated, Complete: true},
	)
	if got := redProgressionObjectives(obs); !offeredProgressID(got, ProgressMainStoryComplete) {
		t.Fatalf("Champion victory must leave the Hall of Fame stage active: %v", got)
	}
}

func TestRedProgressionAcceptsAllLeagueStageFacts(t *testing.T) {
	for _, id := range []ProgressID{
		ProgressLeagueChallengeStarted,
		redProgressLeagueLoreleiDefeated,
		redProgressLeagueBrunoDefeated,
		redProgressLeagueAgathaDefeated,
		redProgressLeagueLanceDefeated,
		ProgressLeagueChampionDefeated,
		ProgressMainStoryComplete,
	} {
		if !redProgressionKnown(id) {
			t.Fatalf("League progression fact %q is not registered as executable Red progression", id)
		}
	}
}

func TestRedProgressStateProjectsIndividualLeagueFacts(t *testing.T) {
	facts := state.StoryFacts{
		LeagueChallengeStarted: true,
		LeagueLoreleiDefeated:  true,
		LeagueBrunoDefeated:    true,
		LeagueAgathaDefeated:   true,
		LeagueLanceDefeated:    true,
	}
	progress := redProgressState(facts)
	for _, id := range []ProgressID{
		ProgressLeagueChallengeStarted,
		redProgressLeagueLoreleiDefeated,
		redProgressLeagueBrunoDefeated,
		redProgressLeagueAgathaDefeated,
		redProgressLeagueLanceDefeated,
	} {
		if !progress.Has(id) {
			t.Fatalf("League fact %q did not project into semantic progress: %v", id, progress)
		}
	}
}

func TestRedProgressStateProjectsMainStoryCompletion(t *testing.T) {
	progress := redProgressState(state.StoryFacts{MainStoryComplete: true, LeagueChampionDefeated: true})
	if !progress.Has(ProgressMainStoryComplete) {
		t.Fatal("main story fact did not project into semantic progress")
	}
	if !progress.Has(ProgressLeagueChampionDefeated) {
		t.Fatal("Champion fact did not remain visible after main story completion")
	}
}

func TestIndigoExteriorRemainsReadyAfterLeagueBlackout(t *testing.T) {
	var mem state.Mem
	mem[sym.CurMap] = redIndigoPlateauMap
	progress := redProgressStateFromRAM(&mem, state.InventoryState{}, state.StoryFacts{})
	if !progress.Has(redProgressIndigoPlateauReady) {
		t.Fatal("Indigo Plateau exterior did not preserve League readiness for blackout retry")
	}
}
