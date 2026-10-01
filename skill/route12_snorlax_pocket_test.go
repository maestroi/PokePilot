package skill

import (
	"os"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
)

// TestRoute12AsleepSnorlaxPocketsSelectSnorlaxAction is the regression for
// run-1mey4xe5t2w04 (triage:1abf42a379bea7c3, farm-issue:2373). With the
// Poké Flute but Snorlax still asleep, SafariCatch routed Route 11 -> Route 12
// toward Fuchsia. The live sprite at (10,62) seals the Route 11 landing pocket
// and the north road away from Route 13's port, so a canExit-bound Snorlax
// action left those pockets no forward edge and GoTo walked back to Route 11
// until the navigation guard fired. Every pocket touches a flute stand, so each
// must select the Snorlax action on the one real Route 13 band.
func TestRoute12AsleepSnorlaxPocketsSelectSnorlaxAction(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	h, err := rom.ParseMap(romData, route12Map)
	if err != nil {
		t.Fatalf("ParseMap Route 12: %v", err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("Build Route 12 grid: %v", err)
	}
	snorlaxSlot := 0
	for i, o := range h.Objects {
		if o.X == 10 && o.Y == 62 {
			snorlaxSlot = i + 1
			break
		}
	}
	if snorlaxSlot == 0 {
		t.Fatal("Route 12 Snorlax object at (10,62) is missing")
	}
	observed := observedStationaryObjectBlockers(h, []gameruntime.LiveMapObject{{Slot: snorlaxSlot, X: 10, Y: 62}})
	g, err = overlayObservedMapTopology(g, grid, h, observed)
	if err != nil {
		t.Fatalf("overlay Route 12: %v", err)
	}

	prereqs := redRoutePrerequisites(g, romData, &state.Mem{})
	for edge, transition := range prereqs.Transitions {
		if edge.Kind == world.EdgeConnection && !transitionCreatesSeam(transition) && !g.ConnectionExitWalkable(edge) {
			start, end, _ := world.ConnectionBand(edge)
			t.Fatalf("%02x -> %02x padding band %d..%d carries non-seam action %q", edge.From, edge.To, start, end, transition.ID)
		}
	}
	prereqs.Capabilities[capCanClearSnorlax] = true

	gate, ok := Place("safari zone gate")
	if !ok {
		t.Fatal("safari zone gate place missing")
	}
	for _, pocket := range []struct {
		name string
		x, y int
	}{
		{"Route 11 landing", 0, 62},
		{"north road below the gate", 10, 22},
		{"south road", 11, 100},
	} {
		plan, err := world.FindRoutePlanAtDestinationWithCapabilities(
			g, route12Map, gate.Map, pocket.x, pocket.y, int(gate.X), int(gate.Y), nil, prereqs,
		)
		if err != nil {
			t.Fatalf("%s (%d,%d): FindRoute: %v", pocket.name, pocket.x, pocket.y, err)
		}
		if len(plan) == 0 {
			t.Fatalf("%s (%d,%d): empty route", pocket.name, pocket.x, pocket.y)
		}
		first := plan[0]
		if first.Edge.To != route13Map || first.Transition == nil || first.Transition.ID != "red:route12_snorlax" {
			t.Fatalf("%s (%d,%d): first step %02x -> %02x transition=%v, want Route 13 via red:route12_snorlax",
				pocket.name, pocket.x, pocket.y, first.Edge.From, first.Edge.To, first.Transition)
		}
		if !g.ConnectionExitWalkable(first.Edge) {
			start, end, _ := world.ConnectionBand(first.Edge)
			t.Fatalf("%s (%d,%d): Snorlax action selected padding band %d..%d", pocket.name, pocket.x, pocket.y, start, end)
		}
	}
}

func TestSatisfiedRoute12SnorlaxDropsSouthboundPivot(t *testing.T) {
	edge := world.Edge{Kind: world.EdgeConnection, From: route12Map, To: route13Map}
	transition, ok := redRouteTransitionForEdge(edge)
	if !ok || transition.ID != "red:route12_snorlax" || !transition.PortBypass || !transition.PivotOnly {
		t.Fatalf("Route 12 -> Route 13 transition = %+v ok=%v, want PortBypass+PivotOnly red:route12_snorlax", transition, ok)
	}
	mem := new(state.Mem)
	if redRouteTransitionEffectComplete(mem, transition) {
		t.Fatal("asleep Snorlax must keep the southbound action")
	}
	setEventFlag(mem, eventBeatRoute12Snorlax)
	if !redRouteTransitionEffectComplete(mem, transition) {
		t.Fatal("beaten Snorlax must drop the southbound PortBypass so ordinary port reachability applies")
	}
}
