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
	"github.com/maestroi/pokepilot/worldmodel"
)

// TestVictoryRoadStrengthReachablePadIsNotHardDenied is the regression for
// triage 1871e17bb0a1de74 / run-2jt54fzwd1aey2c0vzq5vpkr1k: from Victory Road
// 1F (3,8) the north Strength warp is already walkable while the east switch
// puzzle has no solution. Hard-denying that pad stranded VirtualTrade on
// can_solve_victory_road_strength even though executeVictoryRoadStrength would
// treat the crossing as a no-op via warpEdgeReachable.
//
// POKEPILOT_VR_1F_STRENGTH_REACHABLE_STATE is that run's
// round-072-frame-0009240772-train-LAPRAS-to-level-71.state.
func TestVictoryRoadStrengthReachablePadIsNotHardDenied(t *testing.T) {
	path := os.Getenv("POKEPILOT_VR_1F_STRENGTH_REACHABLE_STATE")
	romPath := os.Getenv("POKEMON_RED_ROM")
	if path == "" || romPath == "" {
		t.Skip("set POKEPILOT_VR_1F_STRENGTH_REACHABLE_STATE and POKEMON_RED_ROM")
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
	if got := mem.U8(sym.CurMap); got != victoryRoad1FMap {
		t.Fatalf("prepared state on map %#02x, want Victory Road 1F %#02x", got, victoryRoad1FMap)
	}
	if x, y := mem.U8(sym.XCoord), mem.U8(sym.YCoord); x != 3 || y != 8 {
		t.Fatalf("prepared state at (%d,%d), want (3,8)", x, y)
	}

	strength := world.Edge{Kind: world.EdgeWarp, From: victoryRoad1FMap, To: victoryRoad2FMap, WarpX: 1, WarpY: 1}
	if !warpEdgeReachable(m, romData, strength) {
		t.Fatal("prepared state must already walk onto the 1F Strength warp")
	}
	spec, ok := VictoryRoadBoulderSpec(VictoryRoad1FSwitch)
	if !ok {
		t.Fatal("missing 1F boulder spec")
	}
	if boulderPuzzleEventComplete(&mem, spec) {
		t.Fatal("prepared state already has the 1F switch; reachable-pad regression is not exercised")
	}
	puzzle, err := boulderPuzzleFromMem(romData, &mem, spec)
	if err != nil {
		t.Fatalf("boulderPuzzleFromMem: %v", err)
	}
	if _, err := world.PlanPushPuzzle(puzzle); !errors.Is(err, world.ErrPushPuzzleNoSolution) {
		t.Fatalf("PlanPushPuzzle = %v, want ErrPushPuzzleNoSolution so the hard-deny path is what we refuse", err)
	}

	g, err := cachedRouteGraph(romData)
	if err != nil {
		t.Fatal(err)
	}
	prereqs := redRoutePrerequisites(g, romData, &mem)
	tr, ok := prereqs.Transitions[strength]
	if !ok {
		t.Fatal("expected red:victory_road_strength on the (1,1) pad")
	}
	if tr.Gate || containsCapability(tr.Requires, capCanSolveVictoryRoadStrength) {
		t.Fatalf("reachable Strength pad transition = %+v, want no can_solve_victory_road_strength hard-deny", tr)
	}
	if !prereqs.Capabilities.Has(capCanMoveBoulders) {
		t.Fatal("can_move_boulders must still be projected for the ordinary Strength action")
	}

	x, y := int(mem.U8(sym.XCoord)), int(mem.U8(sym.YCoord))
	if _, err := world.FindRoutePlanAtDestinationWithCapabilities(
		g, victoryRoad1FMap, indigoPlateauLobbyMap, x, y, -1, -1, nil, prereqs); err != nil {
		t.Fatalf("Indigo Plateau from reachable Strength pad: %v", err)
	}
	center, name, err := nearestPokemonCenterAt(m, romData, victoryRoad1FMap)
	if err != nil {
		t.Fatalf("nearestPokemonCenterAt: %v", err)
	}
	if name == "" || center.Map == 0 {
		t.Fatalf("nearestPokemonCenterAt = %q %+v, want a concrete Center", name, center)
	}
	if containsCapability(prereqs.Transitions[strength].Requires, capCanSolveVictoryRoadStrength) {
		t.Fatal("postcondition: Strength pad must remain undenied after Center selection")
	}
}

func TestStrengthWarpPadReachableIgnoresUnsolvableSwitch(t *testing.T) {
	// Tiny 5x3 corridor: player at (1,1) can walk to warp (3,1). A fixed wall
	// seals the switch tile so PlanPushPuzzle would be irrelevant; the helper
	// only answers warp reachability.
	walk := make([]bool, 5*3)
	for i := range walk {
		walk[i] = true
	}
	walk[4] = false // (4,0) wall
	grid, err := world.GridFromSpec(worldmodel.GridSpec{
		Width: 5, Height: 3, Walkable: walk,
		CollisionTile: make([]uint8, 5*3),
		FieldTile:     make([]uint8, 5*3),
	})
	if err != nil {
		t.Fatalf("GridFromSpec: %v", err)
	}
	puzzle := world.PushPuzzle{
		Grid:   grid,
		Player: world.Point{X: 1, Y: 1},
		Fixed:  map[[2]int]bool{{4, 1}: true},
		Movables: []world.Movable{
			{ID: 1, Pos: world.Point{X: 2, Y: 0}},
		},
	}
	edge := world.Edge{WarpX: 3, WarpY: 1}
	if !strengthWarpPadReachable(puzzle, edge) {
		t.Fatal("expected warp (3,1) reachable past the movable on another row")
	}
	edge.WarpY = 0
	edge.WarpX = 4
	if strengthWarpPadReachable(puzzle, edge) {
		t.Fatal("warp on a wall tile must not count as reachable")
	}
}

func TestNearestPokemonCenterFallbackSkipsUnusableHop(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
	}
	g, err := routeGraphForROM(romData)
	if err != nil {
		t.Fatal(err)
	}
	// Standing on VR1F with the Strength climb hard-denied and no tile-level
	// route out: the one-hop fallback must not pretend VR2F is a via map.
	prereqs := world.RoutePrerequisites{
		Transitions: map[world.Edge]gameruntime.Transition{
			{Kind: world.EdgeWarp, From: victoryRoad1FMap, To: victoryRoad2FMap, WarpX: 1, WarpY: 1}: {
				ID:       "red:victory_road_strength",
				Gate:     true,
				Requires: []gameruntime.CapabilityID{capCanSolveVictoryRoadStrength},
			},
		},
		Capabilities: gameruntime.NewCapabilitySet(capCanMoveBoulders),
	}
	// Without a standing tile the fallback stays static (Route 16 house case).
	if _, _, err := nearestPokemonCenterInGraph(g, victoryRoad1FMap, -1, -1, prereqs); err != nil && !errors.Is(err, ErrPCNoKnownCenter) {
		t.Fatalf("static VR1F Center selection: %v", err)
	}
}
