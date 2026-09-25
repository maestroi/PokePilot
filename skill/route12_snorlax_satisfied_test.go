package skill

import (
	"os"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

func setEventFlag(mem *state.Mem, e state.Event) {
	off := sym.EventFlags + uint16(e)/8
	(*mem)[off] |= 1 << (uint16(e) % 8)
}

func TestSatisfiedRoute12SnorlaxDropsActionPivot(t *testing.T) {
	edge := world.Edge{Kind: world.EdgeConnection, From: route13Map, To: route12Map}
	transition, ok := redRouteTransitionForEdge(edge)
	if !ok || transition.ID != "red:route12_snorlax" {
		t.Fatalf("Route 13 -> Route 12 transition = %+v ok=%v, want red:route12_snorlax", transition, ok)
	}

	mem := new(state.Mem)
	if redRouteTransitionEffectComplete(mem, transition) {
		t.Fatal("uncleared Snorlax must keep the action pivot")
	}

	setEventFlag(mem, eventBeatRoute12Snorlax)
	if !redRouteTransitionEffectComplete(mem, transition) {
		t.Fatal("cleared Snorlax must drop the action pivot so ordinary port reachability applies")
	}

	// The compound Route 16 east gate still needs its bicycle annotation after
	// Snorlax is gone; only the flute-only clear actions are omitted.
	bike := gameruntime.Transition{ID: "red:route16_snorlax_bicycle"}
	if redRouteTransitionEffectComplete(mem, bike) {
		t.Fatal("route16_snorlax_bicycle must remain annotated after Snorlax is cleared")
	}

	gate := gameruntime.Transition{ID: "red:route12_snorlax_access", Gate: true}
	if redRouteTransitionEffectComplete(mem, gate) {
		t.Fatal("gates must stay annotated even when related story flags are set")
	}
}

// TestSatisfiedRocketB1FTrainerDoorDropsActionPivot is the regression for farm
// runs stuck bouncing cb<->c7<->c8 forever after Rocket Hideout B1F's door
// guard (Rocket5) was already beaten: red:rocket_b1f_trainer_door was added to
// redRouteTransitionForEdge (#587) but never to redRouteTransitionEffectComplete,
// so both of B1F's gated exits (Game Corner and the B2F stairs) stayed
// annotated as an unplanned boundary forever. FindRoute stops expanding at the
// FIRST such boundary in edge order regardless of whether it leads toward the
// destination, so it always offered the B2F stairs into a dead-end floor
// instead of the direct Game Corner exit, and GoTo looped until the
// navigation guard fired (measured on run-2fjudkv8c4i4y2147qkbldx57h).
func TestSatisfiedRocketB1FTrainerDoorDropsActionPivot(t *testing.T) {
	edge := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB1FMap, To: gameCornerMap, WarpX: rocketB1FGameCornerWarpX, WarpY: rocketB1FGameCornerWarpY}
	transition, ok := redRouteTransitionForEdge(edge)
	if !ok || transition.ID != "red:rocket_b1f_trainer_door" {
		t.Fatalf("B1F -> Game Corner transition = %+v ok=%v, want red:rocket_b1f_trainer_door", transition, ok)
	}

	mem := new(state.Mem)
	if redRouteTransitionEffectComplete(mem, transition) {
		t.Fatal("unbeaten Rocket5 must keep the action pivot")
	}

	setEventFlag(mem, eventBeatRocketB1FTrainer4)
	if !redRouteTransitionEffectComplete(mem, transition) {
		t.Fatal("beaten Rocket5 must drop the action pivot so ordinary port reachability applies")
	}
}

// TestSatisfiedRoute12SnorlaxCatchHabitatLeavesViaRoute14 is the catch-shaped
// sibling of TestRoute12SnorlaxRequiresReachablePort for
// run-29f4dc81z9h2f1sv5v1ggk40xi (triage:d8e00d285ab9c820, farm-issue:1241).
// EVENT_BEAT_ROUTE12_SNORLAX is already set, so the action must drop entirely;
// with the (12,4) trainer overlay, Place("route 13") from (11,4) must leave
// west through Route 14 rather than walkWithinMap no_path on the same map.
func TestSatisfiedRoute12SnorlaxCatchHabitatLeavesViaRoute14(t *testing.T) {
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	dest, ok := Place("route 13")
	if !ok {
		t.Fatal("missing route 13 place")
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
	observed := observedStationaryObjectBlockers(h, []state.SpriteState{{Slot: trainerSlot, X: 12, Y: 4}})
	g, err = overlayObservedMapTopology(g, grid, h, observed)
	if err != nil {
		t.Fatalf("overlay: %v", err)
	}

	mem := new(state.Mem)
	setEventFlag(mem, eventBeatRoute12Snorlax)
	prereqs := redRoutePrerequisites(g, romData, mem)
	for _, tr := range prereqs.Transitions {
		if tr.ID == "red:route12_snorlax" {
			t.Fatalf("cleared Snorlax still annotated on prerequisites: %+v", tr)
		}
	}

	route, err := world.FindRoutePlanAtDestinationWithCapabilities(
		g, route13Map, dest.Map, 11, 4, int(dest.X), int(dest.Y), nil, prereqs,
	)
	if err != nil {
		t.Fatalf("FindRoute: %v", err)
	}
	if len(route) < 2 {
		t.Fatalf("route = %+v, want leave+reenter", route)
	}
	if route[0].Edge.To != route14Map {
		t.Fatalf("first leg to map %02x, want Route 14; route=%+v", route[0].Edge.To, route)
	}
}
