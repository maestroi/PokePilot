package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// TestGameCornerPosterStairIsClosedUntilFound pins farm run
// run-2fjudkv8c4i4y2147qkbldx57h (triage:992e962266d5186c). Speedrun routing
// from GAME_CORNER (17,5) priced the poster warp (17,4) as a one-tile exit,
// then stopped at the Rocket B1F trainer door because that pivot was the
// cheapest semantic frontier. The stair is a wall until
// EVENT_FOUND_ROCKET_HIDEOUT, so Traverse bonked for 180 frames.
func TestGameCornerPosterStairIsClosedUntilFound(t *testing.T) {
	edge := world.Edge{
		Kind:  world.EdgeWarp,
		From:  gameCornerMap,
		To:    rocketHideoutB1FMap,
		WarpX: gameCornerWarpX,
		WarpY: gameCornerWarpY,
	}
	transition := requireTransition(t, edge, "red:rocket_hideout_entrance", capCanEnterRocketHideout)
	if !transition.Gate {
		t.Fatalf("poster stair modeled as an executable pivot: %+v", transition)
	}

	closed := new(state.Mem)
	if caps := redRouteCapabilities(nil, closed); caps.Has(capCanEnterRocketHideout) {
		t.Fatalf("closed poster stair projected %q: %v", capCanEnterRocketHideout, caps)
	}
	open := new(state.Mem)
	setFoundRocketHideout(open)
	if caps := redRouteCapabilities(nil, open); !caps.Has(capCanEnterRocketHideout) {
		t.Fatalf("EVENT_FOUND_ROCKET_HIDEOUT did not project %q: %v", capCanEnterRocketHideout, caps)
	}

	// The reverse stair and the street doors are ordinary warps.
	reverse := world.Edge{
		Kind:  world.EdgeWarp,
		From:  rocketHideoutB1FMap,
		To:    gameCornerMap,
		WarpX: rocketB1FGameCornerWarpX,
		WarpY: rocketB1FGameCornerWarpY,
	}
	if got, ok := redRouteTransitionForEdge(reverse); ok && got.ID == "red:rocket_hideout_entrance" {
		t.Fatalf("B1F exit stair was gated: %+v", got)
	}
	for _, door := range []world.Edge{
		{Kind: world.EdgeWarp, From: gameCornerMap, To: celadonCityMap, WarpX: 15, WarpY: 17},
		{Kind: world.EdgeWarp, From: gameCornerMap, To: celadonCityMap, WarpX: 16, WarpY: 17},
	} {
		if got, ok := redRouteTransitionForEdge(door); ok && got.ID == "red:rocket_hideout_entrance" {
			t.Fatalf("Game Corner street door was gated: %+v", got)
		}
	}
}

// TestSpeedrunRouteLeavesClosedPosterStair is the same failure measured on
// the live graph: with Cut usable and the poster still closed, a speedrun
// plan from (17,5) toward Vermilion must leave through Celadon instead of
// pushing into the wall at (17,4).
func TestSpeedrunRouteLeavesClosedPosterStair(t *testing.T) {
	romData := badgeFourROM(t)
	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	mem := cutCapableMem()
	prereqs := redRoutePrerequisites(g, romData, mem)
	if prereqs.Capabilities.Has(capCanEnterRocketHideout) {
		t.Fatal("closed-door fixture projected the poster stair")
	}
	dest, ok := Place("vermilion city")
	if !ok {
		t.Fatal("missing vermilion city place")
	}
	result, err := world.FindWeightedRoutePlanAtDestinationWithCapabilities(
		g, gameCornerMap, dest.Map, int(gameCornerWarpX), int(gameCornerWarpY)+1, -1, -1, nil, prereqs,
		redGlobalRouteCostPolicy(prereqs),
	)
	if err != nil && !errors.Is(err, world.ErrRouteReplanRequired) {
		t.Fatalf("weighted route: %v", err)
	}
	for i, step := range result.Steps {
		if step.Edge.From == gameCornerMap && step.Edge.To == rocketHideoutB1FMap &&
			step.Edge.WarpX == gameCornerWarpX && step.Edge.WarpY == gameCornerWarpY {
			t.Fatalf("leg %d pushes the closed poster stair: %+v", i+1, step.Edge)
		}
	}
	if len(result.Steps) == 0 || result.Steps[0].Edge.To != celadonCityMap {
		t.Fatalf("first leg = %+v, want the Game Corner door back to Celadon", result.Steps)
	}
}

func cutCapableMem() *state.Mem {
	mem := new(state.Mem)
	mem[sym.ObtainedBadges] = 1<<state.BadgeBoulder | 1<<state.BadgeCascade | 1<<state.BadgeThunder
	mem[sym.PartyCount] = 1
	base := sym.PartyMon1
	mem[base+sym.MonSpecies] = 44
	copy(mem.Slice(base+sym.MonMoves, 4), []byte{cutMove, 0, 0, 0})
	mem[sym.NumBagItems] = 1
	mem[sym.BagItems] = hm01Item
	mem[sym.BagItems+1] = 1
	mem[sym.BagItems+2] = 0xff
	return mem
}

func setFoundRocketHideout(mem *state.Mem) {
	event := state.EventFoundRocketHideout
	addr := sym.EventFlags + uint16(event)/8
	mem[addr] |= 1 << (uint16(event) % 8)
}
