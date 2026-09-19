package skill

import (
	"os"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
)

// TestRoute12SnorlaxIsGateNotPivot pins the Route 13 west-pocket failure from
// run-1q6cjygnjsm5a3tcrcf6mdityp (triage:d9d7e0200d20d0dd): a stationary
// trainer at (12,4) splits (11,4) from the walkable Route 12 seam. When
// red:route12_snorlax was a free pivot, FindRoute still offered that north
// connection first and Traverse exhausted the re-plan budget (last leg then
// misreported as the distant Cycling Road bicycle gate). As a gate it must
// leave via Route 14 and only take the north seam once ordinary walking can
// reach it.
func TestRoute12SnorlaxIsGateNotPivot(t *testing.T) {
	edge := world.Edge{Kind: world.EdgeConnection, From: route13Map, To: route12Map}
	transition, ok := redRouteTransitionForEdge(edge)
	if !ok || transition.ID != "red:route12_snorlax" {
		t.Fatalf("Route 13 -> Route 12 transition = %+v ok=%v, want red:route12_snorlax", transition, ok)
	}
	if !transition.Gate {
		t.Fatalf("red:route12_snorlax must be a gate (blocker), not a free pivot: %+v", transition)
	}

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
	h13, err := rom.ParseMap(romData, route13Map)
	if err != nil {
		t.Fatalf("ParseMap Route 13: %v", err)
	}
	grid13, err := world.Build(romData, h13)
	if err != nil {
		t.Fatalf("Build Route 13 grid: %v", err)
	}
	trainerSlot := 0
	for i, o := range h13.Objects {
		if o.X == 12 && o.Y == 4 {
			trainerSlot = i + 1
			break
		}
	}
	if trainerSlot == 0 {
		t.Fatal("Route 13 object at (12,4) is missing")
	}
	observed := observedStationaryObjectBlockers(h13, []state.SpriteState{{Slot: trainerSlot, X: 12, Y: 4}})
	g, err = overlayObservedMapTopology(g, grid13, h13, observed)
	if err != nil {
		t.Fatalf("overlay Route 13: %v", err)
	}

	var mem state.Mem
	prereqs := redRoutePrerequisites(g, romData, &mem)
	caps := prereqs.Capabilities
	if caps == nil {
		caps = gameruntime.CapabilitySet{}
	}
	caps[capCanClearSnorlax] = true
	prereqs.Capabilities = caps

	plan, err := world.FindRoutePlanAtDestinationWithCapabilities(
		g, route13Map /* vermilion */, 0x05, 11, 4, -1, -1, nil, prereqs,
	)
	if err != nil {
		t.Fatalf("FindRoute from Route 13 pocket: %v", err)
	}
	if len(plan) == 0 {
		t.Fatal("empty route from Route 13 pocket")
	}
	first := plan[0].Edge
	if first.From != route13Map || first.To != route12Map {
		return // escaped via another map first — the desired shape
	}
	start, end, _ := world.ConnectionBand(first)
	t.Fatalf("first leg still took unreachable Route 12 seam band %d..%d with transition %v",
		start, end, plan[0].Transition)
}
