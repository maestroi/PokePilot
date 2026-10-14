package skill

import (
	"errors"
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// TestVictoryRoadUnsolvableStrengthPadDoesNotRouteToKantoMart is the
// regression for triage ca67b38b8b1b63d9 after #2520 (run-m0idltj3yzhe2na7t29vkznya):
// from Victory Road 3F's exit-side ladder (26,8) with switches reset, routing
// still offered red:victory_road_strength at (2,0). Live boulder search proves
// no solution there; hard-denying that pad must yield ErrNoRoute to Vermilion
// and must not offer SUPER POTION at a Kanto mart the player cannot reach.
//
// POKEPILOT_VR_BUY_STATE is that run's
// round-425-frame-0003714769-buy-1-SUPER-POTION.state (Blue).
func TestVictoryRoadUnsolvableStrengthPadDoesNotRouteToKantoMart(t *testing.T) {
	path := os.Getenv("POKEPILOT_VR_BUY_STATE")
	romPath := os.Getenv("POKEMON_BLUE_ROM")
	if romPath == "" {
		romPath = os.Getenv("POKEPILOT_FARM_ROM")
	}
	if path == "" || romPath == "" {
		t.Skip("set POKEPILOT_VR_BUY_STATE and POKEMON_BLUE_ROM")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := emu.Open(romPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.LoadState(b); err != nil {
		t.Fatal(err)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if got := mem.U8(sym.CurMap); got != victoryRoad3FMap {
		t.Fatalf("prepared state on map %#02x, want Victory Road 3F %#02x", got, victoryRoad3FMap)
	}
	if x, y := mem.U8(sym.XCoord), mem.U8(sym.YCoord); x != 26 || y != 8 {
		t.Fatalf("prepared state at (%d,%d), want exit-side ladder (26,8)", x, y)
	}
	if state.HasEvent(&mem, eventVictoryRoad3BoulderOnSwitch1) {
		t.Fatal("prepared state already has the 3F switch; unsolvable-pad regression is not exercised")
	}

	strength := world.Edge{Kind: world.EdgeWarp, From: victoryRoad3FMap, To: victoryRoad2FMap, WarpX: 2, WarpY: 0}
	g, err := cachedRouteGraph(romData)
	if err != nil {
		t.Fatal(err)
	}
	prereqs := redRoutePrerequisites(g, romData, &mem)
	tr, ok := prereqs.Transitions[strength]
	if !ok {
		t.Fatal("expected red:victory_road_strength on the (2,0) pad")
	}
	if !tr.Gate || !containsCapability(tr.Requires, capCanSolveVictoryRoadStrength) {
		t.Fatalf("strength pad transition = %+v, want Gate requiring %s", tr, capCanSolveVictoryRoadStrength)
	}
	if prereqs.Capabilities.Has(capCanSolveVictoryRoadStrength) {
		t.Fatal("can_solve_victory_road_strength must never be projected")
	}

	vermilionMart := uint8(0x5B)
	x, y := int(mem.U8(sym.XCoord)), int(mem.U8(sym.YCoord))
	_, err = world.FindRoutePlanAtDestinationWithCapabilities(g, victoryRoad3FMap, vermilionMart, x, y, -1, -1, nil, prereqs)
	if !errors.Is(err, world.ErrNoRoute) {
		t.Fatalf("Vermilion Mart from unsolvable Strength pad: err=%v, want world.ErrNoRoute", err)
	}
	weighted, err := world.FindWeightedRoutePlanAtDestinationWithCapabilities(
		g, victoryRoad3FMap, vermilionMart, x, y, -1, -1, nil, prereqs, world.DefaultRouteCostPolicy())
	if err == nil || errors.Is(err, world.ErrRouteReplanRequired) {
		t.Fatalf("weighted router priced Vermilion Mart: %v %+v", err, weighted.Steps)
	}

	mart, ok, err := nearestStockMart(romData, &mem, 0x13) // SUPER POTION
	if err != nil {
		t.Fatalf("nearestStockMart: %v", err)
	}
	if ok {
		t.Fatalf("SUPER POTION offered at %s (map %#02x); want no reachable stocking mart", mart.name, mart.mapID)
	}
}

func containsCapability(ids []gameruntime.CapabilityID, want gameruntime.CapabilityID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
