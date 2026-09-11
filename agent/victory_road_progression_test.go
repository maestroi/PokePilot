package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestOfferVictoryRoadProgressionAfterEarthBadge(t *testing.T) {
	obs := Observation{
		Map:        0x01,
		PartyCount: 1,
		Story: ProgressState{
			{ID: redProgressEarthBadge, Complete: true},
			{ID: redProgressIndigoPlateauReady, Complete: false},
		},
	}
	if got := redProgressionObjectives(obs); !offeredProgressID(got, redProgressIndigoPlateauReady) {
		t.Fatalf("Victory Road progression missing after Earth Badge: %v", got)
	}
}

func TestVictoryRoadProgressionWaitsForEarthAndStopsAtIndigo(t *testing.T) {
	before := Observation{Map: 0x01, PartyCount: 1}
	if got := redProgressionObjectives(before); offeredProgressID(got, redProgressIndigoPlateauReady) {
		t.Fatalf("Victory Road progression offered before Earth Badge: %v", got)
	}

	after := before
	after.Story = ProgressState{
		{ID: redProgressEarthBadge, Complete: true},
		{ID: redProgressIndigoPlateauReady, Complete: true},
	}
	if got := redProgressionObjectives(after); offeredProgressID(got, redProgressIndigoPlateauReady) {
		t.Fatalf("Victory Road progression re-offered after Indigo lobby readiness: %v", got)
	}
}

func TestRedProgressionAcceptsIndigoPlateauReadyFact(t *testing.T) {
	if !redProgressionKnown(redProgressIndigoPlateauReady) {
		t.Fatal("indigo_plateau_ready is not registered as executable Red progression")
	}
}

func TestRedProgressStateDerivesIndigoPlateauReadyFromLobby(t *testing.T) {
	var mem state.Mem
	mem[sym.CurMap] = redIndigoPlateauLobbyMap
	progress := redProgressStateFromRAM(&mem, state.InventoryState{}, state.StoryFacts{})
	if !progress.Has(redProgressIndigoPlateauReady) {
		t.Fatal("Indigo Plateau lobby did not project into indigo_plateau_ready progress")
	}
}

func TestRedProgressStateKeepsIndigoReadyAfterLeagueStarts(t *testing.T) {
	var mem state.Mem
	mem[sym.CurMap] = 0xF5
	progress := redProgressStateFromRAM(&mem, state.InventoryState{}, state.StoryFacts{LeagueChallengeStarted: true})
	if !progress.Has(redProgressIndigoPlateauReady) {
		t.Fatal("League challenge start did not preserve indigo_plateau_ready progress")
	}
}
