package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func victoryRoadStageObservation() Observation {
	return Observation{
		Map:        0x01,
		PartyCount: 1,
		Story: ProgressState{
			{ID: redProgressEarthBadge, Complete: true},
		},
	}
}

func TestOfferVictoryRoadStagesInOrderAfterEarthBadge(t *testing.T) {
	obs := victoryRoadStageObservation()
	got := redProgressionObjectives(obs)
	if !offeredProgressID(got, ProgressRoute22RivalResolved) {
		t.Fatalf("Route 22 rival stage missing after Earth Badge: %v", got)
	}
	for _, later := range []ProgressID{ProgressRoute23BadgeChecks, redProgressVictoryRoadCleared, redProgressIndigoPlateauReady} {
		if offeredProgressID(got, later) {
			t.Fatalf("later Victory Road stage %q leaked before Route 22 rival: %v", later, got)
		}
	}

	obs.Story = append(obs.Story, ProgressFact{ID: ProgressRoute22RivalResolved, Complete: true})
	got = redProgressionObjectives(obs)
	if !offeredProgressID(got, ProgressRoute23BadgeChecks) {
		t.Fatalf("Route 23 stage missing after rival: %v", got)
	}
	if offeredProgressID(got, redProgressVictoryRoadCleared) || offeredProgressID(got, redProgressIndigoPlateauReady) {
		t.Fatalf("cave/lobby stages leaked before Route 23: %v", got)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: ProgressRoute23BadgeChecks, Complete: true, Value: 7})
	got = redProgressionObjectives(obs)
	if !offeredProgressID(got, redProgressVictoryRoadCleared) {
		t.Fatalf("Victory Road cave stage missing after Route 23: %v", got)
	}
	if offeredProgressID(got, redProgressIndigoPlateauReady) {
		t.Fatalf("Indigo recovery leaked before cave clear: %v", got)
	}

	obs.Story = append(obs.Story, ProgressFact{ID: redProgressVictoryRoadCleared, Complete: true})
	got = redProgressionObjectives(obs)
	if !offeredProgressID(got, redProgressIndigoPlateauReady) {
		t.Fatalf("Indigo recovery stage missing after cave clear: %v", got)
	}
}

func TestVictoryRoadStagesWaitForEarthAndStopAtIndigo(t *testing.T) {
	before := Observation{Map: 0x01, PartyCount: 1}
	for _, id := range []ProgressID{ProgressRoute22RivalResolved, ProgressRoute23BadgeChecks, redProgressVictoryRoadCleared, redProgressIndigoPlateauReady} {
		if got := redProgressionObjectives(before); offeredProgressID(got, id) {
			t.Fatalf("Victory Road stage %q offered before Earth Badge: %v", id, got)
		}
	}

	after := before
	after.Story = ProgressState{
		{ID: redProgressEarthBadge, Complete: true},
		{ID: ProgressRoute22RivalResolved, Complete: true},
		{ID: ProgressRoute23BadgeChecks, Complete: true, Value: 7},
		{ID: redProgressVictoryRoadCleared, Complete: true},
		{ID: redProgressIndigoPlateauReady, Complete: true},
	}
	got := redProgressionObjectives(after)
	for _, id := range []ProgressID{ProgressRoute22RivalResolved, ProgressRoute23BadgeChecks, redProgressVictoryRoadCleared, redProgressIndigoPlateauReady} {
		if offeredProgressID(got, id) {
			t.Fatalf("Victory Road stage %q re-offered after Indigo readiness: %v", id, got)
		}
	}
}

func TestRedProgressionAcceptsVictoryRoadStageFacts(t *testing.T) {
	for _, id := range []ProgressID{
		ProgressRoute22RivalResolved,
		ProgressRoute23BadgeChecks,
		redProgressVictoryRoadCleared,
		redProgressIndigoPlateauReady,
	} {
		if !redProgressionKnown(id) {
			t.Fatalf("Victory Road progression fact %q is not registered as executable Red progression", id)
		}
	}
}

func TestRedProgressStateRequiresRecoveredIndigoLobby(t *testing.T) {
	var mem state.Mem
	mem[sym.CurMap] = redIndigoPlateauLobbyMap
	if progress := redProgressStateFromRAM(&mem, state.InventoryState{}, state.StoryFacts{}); progress.Has(redProgressIndigoPlateauReady) {
		t.Fatal("empty/unrecovered Indigo lobby incorrectly projected indigo_plateau_ready")
	}

	mem[sym.PartyCount] = 1
	base := sym.PartyMon1
	mem[base+sym.MonSpecies] = 1
	mem[base+sym.MonHP] = 0
	mem[base+sym.MonHP+1] = 20
	mem[base+sym.MonMaxHP] = 0
	mem[base+sym.MonMaxHP+1] = 20
	progress := redProgressStateFromRAM(&mem, state.InventoryState{}, state.StoryFacts{})
	if !progress.Has(redProgressIndigoPlateauReady) {
		t.Fatal("fully recovered Indigo lobby did not project indigo_plateau_ready")
	}
}

func TestRedProgressStateProjectsVictoryRoadClearEvent(t *testing.T) {
	var mem state.Mem
	const event = uint16(0x53f)
	mem[sym.EventFlags+event/8] |= 1 << (event % 8)
	progress := redProgressStateFromRAM(&mem, state.InventoryState{}, state.StoryFacts{})
	if !progress.Has(redProgressVictoryRoadCleared) {
		t.Fatal("final Victory Road 2F east-switch event did not project victory_road_cleared")
	}
}

func TestRedProgressStateKeepsIndigoReadyAfterLeagueStarts(t *testing.T) {
	var mem state.Mem
	mem[sym.CurMap] = 0xF5
	progress := redProgressStateFromRAM(&mem, state.InventoryState{}, state.StoryFacts{LeagueChallengeStarted: true})
	if !progress.Has(redProgressIndigoPlateauReady) {
		t.Fatal("League challenge start did not preserve indigo_plateau_ready progress")
	}
	if !progress.Has(redProgressVictoryRoadCleared) {
		t.Fatal("League challenge start did not preserve victory_road_cleared progress")
	}
}
