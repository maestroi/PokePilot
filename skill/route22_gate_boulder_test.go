package skill

import (
	"errors"
	"os"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// TestRoute22GateBlocksRoute23WithoutBoulderBadge pins run-21a180fnt0cmdmv37zchvhxg:
// before Brock, "go to route 23" walked into the Route 22 Gate guard, looped on
// "I can't let you pass", and stagnation-killed the run as text_box_loop. The
// north warp must report can_pass_route22_gate instead of pricing Route 23 as
// an ordinary walk.
func TestRoute22GateBlocksRoute23WithoutBoulderBadge(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	g, err := cachedRouteGraph(romData)
	if err != nil {
		t.Fatalf("route graph: %v", err)
	}
	route23, ok := Place("route 23")
	if !ok {
		t.Fatal("route 23 place is not registered")
	}

	var mem state.Mem
	// Standing just south of the guard trigger inside the gate — the live
	// failure tile from the stall/failure frames (map 0xc1 at (4,3)).
	mem[sym.CurMap], mem[sym.XCoord], mem[sym.YCoord] = route22GateMap, 4, 3
	setRouteTestEvent(&mem, state.EventGotPokedex)
	prereqs := redRoutePrerequisites(g, romData, &mem)
	if prereqs.Capabilities.Has(capCanPassRoute22Gate) {
		t.Fatalf("pre-Boulder state still projects %q", capCanPassRoute22Gate)
	}

	_, err = world.FindRoutePlanAtDestinationWithCapabilities(
		g, route22GateMap, route23.Map, 4, 3, int(route23.X), int(route23.Y), nil, prereqs)
	var blocked *world.RouteBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("route 23 from gate without Boulder = %v, want RouteBlockedError", err)
	}
	if !routeBlockageNames(blocked, "red:route22_gate_boulder", capCanPassRoute22Gate) {
		t.Fatalf("blockage = %+v, want red:route22_gate_boulder missing %s",
			blocked.Blockages, capCanPassRoute22Gate)
	}
}

// TestRoute22GateOpensRoute23WithBoulderBadge is the positive half: once Brock
// is beaten the same standing tile must price the Route 23 Place arrival.
func TestRoute22GateOpensRoute23WithBoulderBadge(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	g, err := cachedRouteGraph(romData)
	if err != nil {
		t.Fatalf("route graph: %v", err)
	}
	route23, ok := Place("route 23")
	if !ok {
		t.Fatal("route 23 place is not registered")
	}

	var mem state.Mem
	mem[sym.CurMap], mem[sym.XCoord], mem[sym.YCoord] = route22GateMap, 4, 3
	setRouteTestEvent(&mem, state.EventGotPokedex)
	mem[sym.ObtainedBadges] = 1 << uint8(state.BadgeBoulder)
	prereqs := redRoutePrerequisites(g, romData, &mem)
	if !prereqs.Capabilities.Has(capCanPassRoute22Gate) {
		t.Fatalf("Boulder Badge did not project %q: %v", capCanPassRoute22Gate, prereqs.Capabilities)
	}
	if _, err := world.FindRoutePlanAtDestinationWithCapabilities(
		g, route22GateMap, route23.Map, 4, 3, int(route23.X), int(route23.Y), nil, prereqs); err != nil {
		t.Fatalf("route 23 from gate with Boulder Badge: %v", err)
	}
}

func TestKnownClosedRouteGateTextRecognizesRoute22Guard(t *testing.T) {
	if !knownClosedRouteGateText("The rules are rules. I can't let you pass.") {
		t.Fatal("Route 22 Gate refusal was not recognized as a closed route gate")
	}
	if knownClosedRouteGateText("You can pass here only if you have the CascadeBadge!") {
		t.Fatal("Route 23 badge-check prose must not share the Route 22 Gate closed-gate matcher")
	}
}
