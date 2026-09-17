package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestVictoryRoadClearBoundaryUsesLiveSwitchInsideCave(t *testing.T) {
	var mem state.Mem
	mem[sym.CurMap] = victoryRoad2FMap
	const event = uint16(0x53f)
	mem[sym.EventFlags+event/8] |= 1 << (event % 8)
	if !victoryRoadClearBoundary(&mem, redWram(), state.StoryFacts{}) {
		t.Fatal("final 2F east-switch event did not satisfy cave-clear boundary")
	}
}

func TestVictoryRoadClearBoundarySurvivesRoute23ResetNorthOfCave(t *testing.T) {
	facts := state.StoryFacts{Route23BadgeChecksComplete: true, Route23BadgeChecksPassed: 7}
	var mem state.Mem
	mem[sym.CurMap] = route23Map
	mem[sym.YCoord] = route23NorthCaveY
	if !victoryRoadClearBoundary(&mem, redWram(), facts) {
		t.Fatal("Route 23 north-of-cave checkpoint lost cave-clear stage")
	}
	mem[sym.YCoord] = route23NorthCaveY + 1
	if victoryRoadClearBoundary(&mem, redWram(), facts) {
		t.Fatal("Route 23 south-of-cave checkpoint incorrectly retained cave-clear stage")
	}
}

func TestVictoryRoadClearBoundaryRequiresRoute23ChecksAfterSwitchReset(t *testing.T) {
	var mem state.Mem
	mem[sym.CurMap] = indigoPlateauLobbyMap
	if victoryRoadClearBoundary(&mem, redWram(), state.StoryFacts{}) {
		t.Fatal("Indigo geography without Route 23 completion proved cave clear")
	}
}
