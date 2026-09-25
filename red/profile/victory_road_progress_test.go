package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestVictoryRoadClearedForProgressSurvivesRoute23EventResetByGeography(t *testing.T) {
	facts := state.StoryFacts{Route23BadgeChecksComplete: true, Route23BadgeChecksPassed: 7}

	// (14,32) is where the 2F exit drops the player (run-s6v9q3t2w5rl).
	for _, p := range [][2]uint8{{18, 30}, {14, 32}} {
		var north state.Mem
		north[sym.CurMap] = profileRoute23Map
		north[sym.XCoord], north[sym.YCoord] = p[0], p[1]
		if !victoryRoadClearedForProgress(&north, facts) {
			t.Fatalf("Route 23 north-of-cave position %v did not preserve Victory Road clear progress", p)
		}
	}

	for _, p := range [][2]uint8{{4, 31}, {4, 32}, {14, 40}} {
		var south state.Mem
		south[sym.CurMap] = profileRoute23Map
		south[sym.XCoord], south[sym.YCoord] = p[0], p[1]
		if victoryRoadClearedForProgress(&south, facts) {
			t.Fatalf("Route 23 south-of-cave position %v incorrectly preserved Victory Road clear progress", p)
		}
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
