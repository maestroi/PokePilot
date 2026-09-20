package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func controllableFastTravelMem() state.Mem {
	var mem state.Mem
	mem[sym.CurMapWidth] = 10
	mem[sym.CurMapHeight] = 9
	return mem
}

func setTownVisited(mem *state.Mem, mapID uint8) {
	mem[sym.TownVisitedFlag+uint16(mapID)/8] |= 1 << (mapID % 8)
}

func TestChooseFastTravelFlyOnlyToVisitedRequestedCity(t *testing.T) {
	mem := controllableFastTravelMem()
	mem[sym.CurMap] = 0x01
	mem[sym.CurMapTileset] = overworldTileset
	mem[sym.ObtainedBadges] = 1 << 2 // Thunder Badge
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1+sym.MonMoves] = fieldFlyMove
	setTownVisited(&mem, 0)
	setTownVisited(&mem, 6)

	got := chooseFastTravel(&mem, Destination{Map: 6, X: 41, Y: 10})
	if got.Kind != fastTravelFly || got.Map != 6 {
		t.Fatalf("choice = %+v, want Fly to Celadon", got)
	}

	mem[sym.TownVisitedFlag] &^= 1 << 6
	if got := chooseFastTravel(&mem, Destination{Map: 6, X: 41, Y: 10}); got.Kind != fastTravelNone {
		t.Fatalf("unvisited city choice = %+v, want none", got)
	}
}

func TestChooseFastTravelPrefersDigThenEscapeRopeForLastCenter(t *testing.T) {
	mem := controllableFastTravelMem()
	mem[sym.CurMap] = 0x3d
	mem[sym.CurMapTileset] = 17 // CAVERN
	mem[sym.LastBlackoutMap] = 0x02
	mem[sym.PartyCount] = 1
	mem[sym.PartyMon1+sym.MonMoves] = digMoveID
	mem[sym.NumBagItems] = 1
	mem[sym.BagItems] = escapeRopeItem
	mem[sym.BagItems+1] = 2

	got := chooseFastTravel(&mem, Destination{Map: 0x02, X: 13, Y: 26})
	if got.Kind != fastTravelDig {
		t.Fatalf("choice with Dig = %+v, want Dig", got)
	}

	mem[sym.PartyMon1+sym.MonMoves] = 0
	got = chooseFastTravel(&mem, Destination{Map: 0x02, X: 13, Y: 26})
	if got.Kind != fastTravelEscapeRope {
		t.Fatalf("choice without Dig = %+v, want Escape Rope", got)
	}

	if got := chooseFastTravel(&mem, Destination{Map: 0x03, X: 1, Y: 1}); got.Kind != fastTravelNone {
		t.Fatalf("different requested map choice = %+v, want none", got)
	}
}

func TestCyclingRoadExplicitInputSuppressesAutoDown(t *testing.T) {
	if !cyclingRoadAutoDown(route17Map, false, false) {
		t.Fatal("Route 17 idle input did not trigger downhill coast")
	}
	if cyclingRoadAutoDown(route17Map, false, true) {
		t.Fatal("explicit Route 17 input still triggered downhill coast")
	}
	if cyclingRoadAutoDown(route17Map, true, false) {
		t.Fatal("trainer battle did not suppress Route 17 downhill coast")
	}
}
