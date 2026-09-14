package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestVictoryRoadClearedForProgressSurvivesRoute23EventResetByGeography(t *testing.T) {
	facts := state.StoryFacts{Route23BadgeChecksComplete: true, Route23BadgeChecksPassed: 7}

	var north state.Mem
	north[sym.CurMap] = profileRoute23Map
	north[sym.YCoord] = profileRoute23NorthCaveY
	if !victoryRoadClearedForProgress(&north, facts) {
		t.Fatal("Route 23 north-of-cave position did not preserve Victory Road clear progress")
	}

	var south state.Mem
	south[sym.CurMap] = profileRoute23Map
	south[sym.YCoord] = profileRoute23NorthCaveY + 1
	if victoryRoadClearedForProgress(&south, facts) {
		t.Fatal("Route 23 south-of-cave position incorrectly preserved Victory Road clear progress")
	}

	var lobby state.Mem
	lobby[sym.CurMap] = indigoPlateauLobbyMap
	if !victoryRoadClearedForProgress(&lobby, facts) {
		t.Fatal("Indigo lobby did not preserve Victory Road clear progress after switch reset")
	}

	if victoryRoadClearedForProgress(&lobby, state.StoryFacts{}) {
		t.Fatal("Indigo geography without Route 23 completion incorrectly proved Victory Road clear")
	}
}

func TestVictoryRoadClearedForProgressIsMonotonicAfterLeagueStart(t *testing.T) {
	var mem state.Mem
	mem[sym.CurMap] = 0x01
	if !victoryRoadClearedForProgress(&mem, state.StoryFacts{LeagueChallengeStarted: true}) {
		t.Fatal("League start did not preserve Victory Road clear progress")
	}
}

func TestProjectStoryIndigoReadyRequiresRecoveredLobby(t *testing.T) {
	var mem state.Mem
	mem[sym.CurMap] = indigoPlateauLobbyMap
	facts := state.StoryFacts{Route23BadgeChecksComplete: true, Route23BadgeChecksPassed: 7}
	if ProjectStory(&mem, facts).Has(ProgressIndigoPlateauReady) {
		t.Fatal("empty Indigo lobby projected ready")
	}

	mem[sym.PartyCount] = 1
	base := sym.PartyMon1
	mem[base+sym.MonSpecies] = 1
	mem[base+sym.MonHP+1] = 20
	mem[base+sym.MonMaxHP+1] = 20
	if !ProjectStory(&mem, facts).Has(ProgressIndigoPlateauReady) {
		t.Fatal("fully recovered Indigo lobby did not project ready")
	}

	mem[base+sym.MonHP+1] = 19
	if ProjectStory(&mem, facts).Has(ProgressIndigoPlateauReady) {
		t.Fatal("damaged Indigo lobby projected ready")
	}
}
