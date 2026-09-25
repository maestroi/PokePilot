package skill

import (
	"errors"
	"os"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
)

// TestRoute12SnorlaxRequiresReachablePort pins the Route 13 west-pocket
// failure shared by:
//   - run-1q6cjygnjsm5a3tcrcf6mdityp (triage:d9d7e0200d20d0dd)
//   - run-1dcoirnu6on9p2kox4vsamfo12 (triage:8ebf8a44a95c3566, farm-issue:1187)
//   - run-29f4dc81z9h2f1sv5v1ggk40xi (triage:d8e00d285ab9c820, farm-issue:1241;
//     catch same-map Place("route 13") with Snorlax already cleared — see
//     TestSatisfiedRoute12SnorlaxCatchHabitatLeavesViaRoute14)
//   - run-2pw78yuh93bj133cj3fwkz4rey (triage:5f4d7b3680d0f610, farm-issue:1282;
//     same go_to vermilion route_replan_exhausted from (11,4), last leg
//     misreported as missing can_ride_cycling_road)
//   - run-b7qumpdylcya24q44h61rkzgm (triage:683e05e984651648, farm-issue:1287;
//     go_to vermilion old rod house blocked route_prerequisite_missing with
//     can_ride_cycling_road / can_enter_saffron — see
//     TestRoute13TrainerPocketRoutesOldRodHouseViaRoute14)
//
// A stationary trainer at (12,4) splits (11,4) from the walkable Route 12 seam.
// A free FROM-side pivot offered that unreachable north connection and
// exhausted the re-plan budget on go_to vermilion. The Snorlax action must
// remain executable, but PivotOnly keeps ordinary port reachability so this
// pocket escapes through Route 14 first.
func TestRoute12SnorlaxRequiresReachablePort(t *testing.T) {
	edge := world.Edge{Kind: world.EdgeConnection, From: route13Map, To: route12Map}
	transition, ok := redRouteTransitionForEdge(edge)
	if !ok || transition.ID != "red:route12_snorlax" {
		t.Fatalf("Route 13 -> Route 12 transition = %+v ok=%v, want red:route12_snorlax", transition, ok)
	}
	if transition.Gate || !transition.PivotOnly {
		t.Fatalf("red:route12_snorlax must be an executable PivotOnly action, not a passive gate/free pivot: %+v", transition)
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
	observed := observedStationaryObjectBlockers(h13, []game.LiveMapObject{{Slot: trainerSlot, X: 12, Y: 4}})
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

// TestRoute13TrainerPocketRoutesOldRodHouseViaRoute14 is the old-rod sibling
// of TestRoute12SnorlaxRequiresReachablePort for
// run-b7qumpdylcya24q44h61rkzgm (triage:683e05e984651648, farm-issue:1287).
// With flute but no bicycle/saffron, the west trainer pocket must leave via
// Route 14 rather than report route_prerequisite_missing for
// can_ride_cycling_road on go_to vermilion old rod house.
func TestRoute13TrainerPocketRoutesOldRodHouseViaRoute14(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	dest, ok := Place("vermilion old rod house")
	if !ok {
		t.Fatal("vermilion old rod house place missing")
	}
	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	h, err := rom.ParseMap(romData, route13Map)
	if err != nil {
		t.Fatalf("ParseMap: %v", err)
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		t.Fatalf("Build grid: %v", err)
	}
	trainerSlot := 0
	for i, o := range h.Objects {
		if o.X == 12 && o.Y == 4 {
			trainerSlot = i + 1
			break
		}
	}
	if trainerSlot == 0 {
		t.Fatal("Route 13 object at (12,4) is missing")
	}
	observed := observedStationaryObjectBlockers(h, []game.LiveMapObject{{Slot: trainerSlot, X: 12, Y: 4}})
	g, err = overlayObservedMapTopology(g, grid, h, observed)
	if err != nil {
		t.Fatalf("overlay: %v", err)
	}

	prereqs := redRoutePrerequisites(g, romData, &state.Mem{})
	caps := prereqs.Capabilities
	if caps == nil {
		caps = gameruntime.CapabilitySet{}
	}
	caps[capCanClearSnorlax] = true
	delete(caps, capCanRideCyclingRoad)
	delete(caps, capCanEnterSaffron)
	prereqs.Capabilities = caps

	plan, err := world.FindRoutePlanAtDestinationWithCapabilities(
		g, route13Map, dest.Map, 11, 4, int(dest.X), int(dest.Y), nil, prereqs,
	)
	var blocked *world.RouteBlockedError
	if errors.As(err, &blocked) {
		for _, id := range blocked.MissingCapabilities() {
			if id == capCanRideCyclingRoad || id == capCanEnterSaffron {
				t.Fatalf("trainer pocket reported unrelated prerequisite %q: %v", id, err)
			}
		}
		t.Fatalf("FindRoute blocked: %v", err)
	}
	if err != nil {
		t.Fatalf("FindRoute: %v", err)
	}
	if len(plan) == 0 {
		t.Fatal("empty route")
	}
	if plan[0].Edge.To != route14Map {
		t.Fatalf("first leg to map %02x, want Route 14; plan=%+v", plan[0].Edge.To, plan)
	}
}
