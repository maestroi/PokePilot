package skill

import (
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// TestIndigoLobbyNurseReplay uses the round-11 checkpoint from
// run-jxh8lk19wv6on. The artifact is kept by the farm, not in Git.
// The old exact destination was the solid counter at (7,6).
func TestIndigoLobbyNurseReplay(t *testing.T) {
	statePath := os.Getenv("INDIGO_LOBBY_REPRO_STATE")
	if statePath == "" {
		t.Skip("set INDIGO_LOBBY_REPRO_STATE to the run-jxh8lk19wv6on round-11 .state artifact")
	}
	b, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	const stateSHA = "df76d05bd0c51dddb62d4dfe2de8d5fd74784391699312a6d123bfc95e6dedd4"
	if got := fmt.Sprintf("%x", sha256.Sum256(b)); got != stateSHA {
		t.Fatalf("replay state SHA-256 = %s, want %s", got, stateSHA)
	}
	m, err := emu.Open(os.Getenv("POKEMON_RED_ROM"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.LoadState(b); err != nil {
		t.Fatal(err)
	}
	if err := VictoryRoadPrepareIndigo(m, m.ROM(), StatAwareMove(m.ROM())); err != nil {
		t.Fatal(err)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if got := mem.U8(sym.CurMap); got != indigoPlateauLobbyMap {
		t.Fatalf("finished on map %#02x, want Indigo lobby %#02x", got, indigoPlateauLobbyMap)
	}
	if !allPartyCenterRecovered(&mem) {
		t.Fatal("Indigo nurse did not fully recover the party")
	}
}

func TestVictoryRoadClearBoundaryUsesLiveSwitchInsideCave(t *testing.T) {
	var mem state.Mem
	mem[sym.CurMap] = victoryRoad2FMap
	const event = uint16(0x53f)
	mem[sym.EventFlags+event/8] |= 1 << (event % 8)
	if !victoryRoadClearBoundary(&mem, state.StoryFacts{}) {
		t.Fatal("final 2F east-switch event did not satisfy cave-clear boundary")
	}
}

func TestVictoryRoadClearBoundaryInBattleStillNeedsSettlement(t *testing.T) {
	var mem state.Mem
	mem[sym.CurMap] = victoryRoad2FMap
	mem[sym.CurMapWidth] = 1
	mem[sym.CurMapHeight] = 1
	const event = uint16(0x53f)
	mem[sym.EventFlags+event/8] |= 1 << (event % 8)

	if !victoryRoadClearBoundaryReady(&mem, state.StoryFacts{}) {
		t.Fatal("clean final-switch checkpoint should be ready to return")
	}

	// The switch event is durable before ownership necessarily returns to the
	// overworld. #1988 captured exactly this shape: progress said
	// victory_road_cleared while a battle still owned the screen.
	mem[sym.IsInBattle] = 1
	if !victoryRoadClearBoundary(&mem, state.StoryFacts{}) {
		t.Fatal("battle unexpectedly erased durable Victory Road progress")
	}
	if victoryRoadClearBoundaryReady(&mem, state.StoryFacts{}) {
		t.Fatal("event-positive checkpoint returned while battle still owned the screen")
	}

	// Battle teardown has a second transient window after wIsInBattle clears.
	// state.Controllable intentionally keeps that window dirty too.
	mem[sym.IsInBattle] = 0
	mem[sym.StatusFlags4] = 1 << 5
	if victoryRoadClearBoundaryReady(&mem, state.StoryFacts{}) {
		t.Fatal("event-positive checkpoint returned during post-battle teardown")
	}

	mem[sym.StatusFlags4] = 0
	if !victoryRoadClearBoundaryReady(&mem, state.StoryFacts{}) {
		t.Fatal("settled final-switch checkpoint did not become ready")
	}
}

func TestVictoryRoadClearBoundarySurvivesRoute23ResetNorthOfCave(t *testing.T) {
	facts := state.StoryFacts{Route23BadgeChecksComplete: true, Route23BadgeChecksPassed: 7}
	var mem state.Mem
	mem[sym.CurMap] = route23Map
	mem[sym.XCoord], mem[sym.YCoord] = 14, 32 // just outside the 2F exit
	if !victoryRoadClearBoundary(&mem, facts) {
		t.Fatal("Route 23 north-of-cave checkpoint lost cave-clear stage")
	}
	mem[sym.XCoord], mem[sym.YCoord] = 4, 32 // just outside the 1F entrance
	if victoryRoadClearBoundary(&mem, facts) {
		t.Fatal("Route 23 south-of-cave checkpoint incorrectly retained cave-clear stage")
	}
}

func TestVictoryRoadClearBoundaryRequiresRoute23ChecksAfterSwitchReset(t *testing.T) {
	var mem state.Mem
	mem[sym.CurMap] = indigoPlateauLobbyMap
	if victoryRoadClearBoundary(&mem, state.StoryFacts{}) {
		t.Fatal("Indigo geography without Route 23 completion proved cave clear")
	}
}
