package skill

import (
	"os"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
	"github.com/maestroi/pokepilot/worldmodel"
)

// TestROMBoulderHomeTopologySplitsVictoryRoad3FFalseBridge locks the shared
// invariant behind run-11wa4vv6piic82mkqrmqxtwere (triage 0ab406fc7ad49f58):
// static Victory Road 3F collision walks from the west ladder (23,7) to the
// exit-side ladder (26,8) across BOULDER3's home tile (24,10). Live sprites
// block that tile, so a plan that prices the false bridge dies after the warp.
// Seeding ROM boulder homes into the route graph must split those components
// before GoTo ever selects the pad.
func TestROMBoulderHomeTopologySplitsVictoryRoad3FFalseBridge(t *testing.T) {
	path := os.Getenv("POKEMON_YELLOW_ROM")
	if path == "" {
		path = os.Getenv("POKEMON_RED_ROM")
	}
	if path == "" {
		t.Skip("POKEMON_YELLOW_ROM/POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	provider, err := routingProviderForROM(romData)
	if err != nil {
		t.Fatalf("routingProviderForROM: %v", err)
	}
	spec, err := provider.Grid(victoryRoad3FMap, nil, world.TraversalLand)
	if err != nil {
		t.Fatalf("Victory Road 3F grid: %v", err)
	}
	staticGrid, err := world.GridFromSpec(spec)
	if err != nil {
		t.Fatalf("Victory Road 3F GridFromSpec: %v", err)
	}
	if _, err := world.FindPath(staticGrid, 23, 7, 26, 8, nil); err != nil {
		t.Fatalf("static 3F path (23,7)->(26,8) = %v, want a walkable false bridge to exist before the overlay", err)
	}

	g, err := routeGraphForROM(romData)
	if err != nil {
		t.Fatalf("routeGraphForROM: %v", err)
	}
	g, err = withROMBoulderHomeTopology(g, romData, nil)
	if err != nil {
		t.Fatalf("withROMBoulderHomeTopology: %v", err)
	}
	overlaidGrid, err := world.GridFromSpec(spec)
	if err != nil {
		t.Fatalf("rebuild Victory Road 3F: %v", err)
	}
	h, err := provider.ParseMap(victoryRoad3FMap)
	if err != nil {
		t.Fatalf("parse Victory Road 3F: %v", err)
	}
	boulderSprite := gen1BoulderSpriteID(romData)
	blocked := 0
	for _, o := range h.Objects {
		if o.Movement == worldmodel.ObjectMovementStay && o.NativeSpriteID == uint16(boulderSprite) {
			overlaidGrid.Set(int(o.X), int(o.Y), false)
			blocked++
		}
	}
	if blocked == 0 {
		t.Fatalf("no ROM boulder homes found on Victory Road 3F (sprite %#02x)", boulderSprite)
	}
	if _, err := world.FindPath(overlaidGrid, 23, 7, 26, 8, nil); err == nil {
		t.Fatal("boulder-home overlay still walks (23,7)->(26,8): the false bridge was not split")
	}

	// Component routing must agree: the west ladder and exit-side ladder are
	// no longer the same entry component after the overlay.
	west := world.Edge{Kind: world.EdgeWarp, From: victoryRoad3FMap, To: victoryRoad2FMap, WarpX: 23, WarpY: 7}
	exitSide := world.Edge{Kind: world.EdgeWarp, From: victoryRoad3FMap, To: victoryRoad2FMap, WarpX: 26, WarpY: 8}
	same, known := g.EdgeEntrySharesComponentWith(west, 26, 8)
	if !known {
		t.Fatal("Victory Road 3F west ladder entry component is unknown after boulder-home overlay")
	}
	if same {
		t.Fatal("exit-side pad (26,8) still shares the west ladder's component after boulder-home overlay")
	}
	sameExit, knownExit := g.EdgeEntrySharesComponentWith(exitSide, 26, 8)
	if !knownExit || !sameExit {
		t.Fatalf("exit-side ladder entry does not contain its own pad (same=%v known=%v)", sameExit, knownExit)
	}
}

// TestVictoryRoadRouteDropsFalseThreeFBridge is the routing half of the same
// failure: standing on Victory Road 2F's west side with Strength usable, the
// static graph prices 2F (23,7) -> 3F (26,8) as a way out to Route 23 because
// it walks through boulder homes. After seeding those homes, that pad must not
// appear on the journey.
func TestVictoryRoadRouteDropsFalseThreeFBridge(t *testing.T) {
	path := os.Getenv("POKEMON_YELLOW_ROM")
	if path == "" {
		path = os.Getenv("POKEMON_RED_ROM")
	}
	if path == "" {
		t.Skip("POKEMON_YELLOW_ROM/POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	base, err := routeGraphForROM(romData)
	if err != nil {
		t.Fatalf("routeGraphForROM: %v", err)
	}
	mem := new(state.Mem)
	prereqsFor := func(g *world.Graph) world.RoutePrerequisites {
		prereqs := redRoutePrerequisites(g, romData, mem)
		caps := gameruntime.NewCapabilitySet(capCanMoveBoulders, capCanPassRoute23BadgeChecks, capCanSurf)
		for id, on := range prereqs.Capabilities {
			if on {
				caps[id] = true
			}
		}
		prereqs.Capabilities = caps
		return prereqs
	}
	usesFalseBridge := func(plan []world.RouteStep) bool {
		for _, step := range plan {
			e := step.Edge
			if e.From == victoryRoad2FMap && e.To == victoryRoad3FMap && e.WarpX == 23 && e.WarpY == 7 {
				return true
			}
		}
		return false
	}

	// Stand on the 1F ladder landing on 2F — the cleared-cave west component.
	plainPlan, err := world.FindRoutePlanAtDestinationWithCapabilities(
		base, victoryRoad2FMap, route23Map, 0, 8, 14, 31, nil, prereqsFor(base))
	if err != nil {
		t.Fatalf("plain route from Victory Road 2F west side to Route 23: %v", err)
	}
	if !usesFalseBridge(plainPlan) {
		t.Fatal("plain static route no longer uses 2F (23,7); the regression fixture lost its false bridge")
	}

	g, err := withROMBoulderHomeTopology(base, romData, nil)
	if err != nil {
		t.Fatalf("withROMBoulderHomeTopology: %v", err)
	}
	plan, err := world.FindRoutePlanAtDestinationWithCapabilities(
		g, victoryRoad2FMap, route23Map, 0, 8, 14, 31, nil, prereqsFor(g))
	if err == nil && usesFalseBridge(plan) {
		t.Fatal("boulder-home overlay still prices the false 2F (23,7) bridge to Route 23")
	}
	for i, step := range plan {
		e := step.Edge
		t.Logf("leg %d: %02x->%02x warp(%d,%d) kind=%v", i+1, e.From, e.To, e.WarpX, e.WarpY, e.Kind)
	}
}
