package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestOfferEliteFourProgressionAfterIndigoReady(t *testing.T) {
	obs := Observation{
		Map:        0xAE,
		PartyCount: 1,
		Story: ProgressState{
			{ID: redProgressIndigoPlateauReady, Complete: true},
			{ID: ProgressMainStoryComplete, Complete: false},
		},
	}
	if got := redProgressionObjectives(obs); !offeredProgressID(got, ProgressMainStoryComplete) {
		t.Fatalf("Elite Four progression missing from prepared Indigo state: %v", got)
	}
}

func TestEliteFourProgressionWaitsForIndigoAndStopsAtHallOfFame(t *testing.T) {
	before := Observation{Map: 0x01, PartyCount: 1}
	if got := redProgressionObjectives(before); offeredProgressID(got, ProgressMainStoryComplete) {
		t.Fatalf("Elite Four progression offered before Indigo readiness: %v", got)
	}

	championOnly := before
	championOnly.Story = ProgressState{
		{ID: redProgressIndigoPlateauReady, Complete: true},
		{ID: ProgressLeagueChampionDefeated, Complete: true},
		{ID: ProgressMainStoryComplete, Complete: false},
	}
	if got := redProgressionObjectives(championOnly); !offeredProgressID(got, ProgressMainStoryComplete) {
		t.Fatalf("Champion-only state must keep the Hall of Fame progression active: %v", got)
	}

	done := championOnly
	done.Story = append(done.Story, ProgressFact{ID: ProgressMainStoryComplete, Complete: true})
	if got := redProgressionObjectives(done); offeredProgressID(got, ProgressMainStoryComplete) {
		t.Fatalf("Elite Four progression re-offered after Hall of Fame completion: %v", got)
	}
}

func TestRedProgressionAcceptsMainStoryCompletionFact(t *testing.T) {
	if !redProgressionKnown(ProgressMainStoryComplete) {
		t.Fatal("main_story_complete is not registered as executable Red progression")
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
