package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// TestHallOfFamePostCompletionNeedsSoftResetEgress pins the shared invariant
// behind run-d6dokr184ky81: after the durable Hall-of-Fame bit is set, the
// world graph still offers HoF -> Champion -> Lance, but Lance's Champion
// entrance is a disconnected north pocket, so Kanto destinations stay
// world: no_route. Dig/Escape Rope are illegal on the HoF tileset. The only
// cartridge exit is soft-reset + CONTINUE; navigation must recognize that
// start boundary rather than pathfind from the stale CurMap.
func TestHallOfFamePostCompletionNeedsSoftResetEgress(t *testing.T) {
	romData := romBytesForLeagueGate(t)

	mem := leagueMem(t, true)
	mem[sym.CurMap] = hallOfFameMap
	mem[sym.XCoord] = 4
	mem[sym.YCoord] = 2
	mem[sym.UpdateSpritesEnabled] = 1
	mem[sym.CurMapTileset] = 7 // HALL_OF_FAME gym tileset; escape travel illegal

	if escapeTravelAllowed(mem) {
		t.Fatal("Hall of Fame tileset unexpectedly allows Dig/Escape Rope")
	}

	round := leagueRoutePrereqs(t, romData, mem)
	pallet, ok := Place("pallet town")
	if !ok {
		t.Fatal("pallet town place is not registered")
	}
	_, err := findRoutePlanForDestination(round.graph, hallOfFameMap, 4, 2, pallet, nil, round.prereqs)
	if err == nil {
		t.Fatal("HoF -> Pallet unexpectedly routed; soft-reset egress would be unnecessary")
	}

	// Post-Init title boundary: sprites $ff is the positive Init proof even
	// when CurMap still reads as Hall of Fame.
	mem[sym.UpdateSpritesEnabled] = initSpritesDisabled
	if mem.U8(sym.UpdateSpritesEnabled) != initSpritesDisabled {
		t.Fatal("test setup failed to project post-Init sprites")
	}
	if !state.MainStoryComplete(mem) {
		t.Fatal("test setup missing main-story completion")
	}
}
