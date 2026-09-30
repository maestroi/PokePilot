package skill

import (
	"os"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/world"
)

// victoryRoadStrengthEdges lists every real Victory Road floor crossing between
// 1F/2F and 2F/3F. The invariant below decides which exact pads may carry
// red:victory_road_strength by asking the executor whether that edge owns a
// boulder action; sibling ladders must remain ordinary geometry.
func victoryRoadStrengthEdges() []world.Edge {
	return []world.Edge{
		{Kind: world.EdgeWarp, From: victoryRoad1FMap, To: victoryRoad2FMap, WarpX: 1, WarpY: 1},
		{Kind: world.EdgeWarp, From: victoryRoad2FMap, To: victoryRoad1FMap, WarpX: 0, WarpY: 8},

		{Kind: world.EdgeWarp, From: victoryRoad2FMap, To: victoryRoad3FMap, WarpX: 23, WarpY: 7},
		{Kind: world.EdgeWarp, From: victoryRoad2FMap, To: victoryRoad3FMap, WarpX: 25, WarpY: 14},
		{Kind: world.EdgeWarp, From: victoryRoad2FMap, To: victoryRoad3FMap, WarpX: 27, WarpY: 7},
		{Kind: world.EdgeWarp, From: victoryRoad2FMap, To: victoryRoad3FMap, WarpX: 1, WarpY: 1},

		{Kind: world.EdgeWarp, From: victoryRoad3FMap, To: victoryRoad2FMap, WarpX: 23, WarpY: 7},
		{Kind: world.EdgeWarp, From: victoryRoad3FMap, To: victoryRoad2FMap, WarpX: 26, WarpY: 8},
		{Kind: world.EdgeWarp, From: victoryRoad3FMap, To: victoryRoad2FMap, WarpX: 27, WarpY: 15},
		{Kind: world.EdgeWarp, From: victoryRoad3FMap, To: victoryRoad2FMap, WarpX: 2, WarpY: 0},
	}
}

// TestVictoryRoadStrengthAnnotationImpliesAction locks the shared invariant the
// Victory Road 2F failures died on: red:victory_road_strength grants
// skipCanExit, which lets the router offer a floor crossing even when the
// player's walkable component cannot reach its pad. That privilege is only ever
// legitimate when the adapter can actually perform an action to open the pad.
// An annotated edge whose section lookup returns "no action" is a phantom: the
// executor returns success without doing anything, Traverse then cannot reach
// the pad, and a journey that planned through it dies.
//
// MEASURED on run-2fgn1nak1rmjho8sqyiogre6h and run-3dtp99mx0jn3ickoqlj1k6iue:
// the player resumed in Victory Road 2F's sealed ladder pocket at (25,14) and
// the router offered the descending 2F -> 1F pad at (0,8), which the pocket
// cannot reach and which no boulder objective owns.
func TestVictoryRoadStrengthAnnotationImpliesAction(t *testing.T) {
	for _, edge := range victoryRoadStrengthEdges() {
		transition, annotated := redRouteTransitionForEdge(edge)
		section, hasAction := victoryRoadSectionForTransition(edge)
		t.Logf("%02x->%02x warp(%d,%d): annotated=%v section=%v hasAction=%v",
			edge.From, edge.To, edge.WarpX, edge.WarpY, annotated, section, hasAction)

		if !annotated {
			if hasAction {
				t.Errorf("%02x->%02x has boulder action %v but is not annotated: the pivot it needs is unreachable",
					edge.From, edge.To, section)
			}
			continue
		}
		if transition.ID != "red:victory_road_strength" {
			t.Errorf("%02x->%02x transition = %q, want red:victory_road_strength", edge.From, edge.To, transition.ID)
		}
		if transition.Gate || transition.PortBypass || transition.PivotOnly {
			t.Errorf("%02x->%02x transition = %+v, want a plain skipCanExit pivot", edge.From, edge.To, transition)
		}
		if !hasAction {
			t.Errorf("%02x->%02x warp(%d,%d) is annotated with %q and so receives skipCanExit, but the adapter has no boulder action for it: the router may select a pad the player cannot reach (run-2fgn1nak1rmjho8sqyiogre6h)",
				edge.From, edge.To, edge.WarpX, edge.WarpY, transition.ID)
		}
	}
}

// TestVictoryRoad2FPocketRouteDoesNotUseUnreachableOneFPad is the routing half
// of the same invariant, on the real ROM graph: from the sealed pocket the
// first hop of a southbound journey must be a pad the player is standing on,
// never the 2F -> 1F pad at (0,8) that the pocket cannot walk to.
func TestVictoryRoad2FPocketRouteDoesNotUseUnreachableOneFPad(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	g, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	transitions := map[world.Edge]gameruntime.Transition{}
	caps := map[gameruntime.CapabilityID]bool{}
	for _, edges := range g.Edges {
		for _, e := range edges {
			tr, ok := redRouteTransitionForEdge(e)
			if !ok {
				continue
			}
			transitions[e] = tr
			for _, id := range tr.Requires {
				caps[id] = true
			}
		}
	}
	ids := make([]gameruntime.CapabilityID, 0, len(caps))
	for id := range caps {
		ids = append(ids, id)
	}
	prereqs := world.RoutePrerequisites{
		Transitions:  transitions,
		Capabilities: gameruntime.NewCapabilitySet(ids...),
	}
	plan, err := world.FindRoutePlanAtDestinationWithCapabilities(
		g, victoryRoad2FMap, semanticCinnabarMap, 25, 14, 11, 12, nil, prereqs)
	if err != nil {
		t.Fatalf("route plan from the Victory Road 2F pocket: %v", err)
	}
	if len(plan) == 0 {
		t.Fatalf("route plan from the Victory Road 2F pocket is empty")
	}
	first := plan[0]
	t.Logf("first hop: kind=%v %02x->%02x warp(%d,%d)", first.Edge.Kind, first.Edge.From, first.Edge.To, first.Edge.WarpX, first.Edge.WarpY)
	if first.Edge.From == victoryRoad2FMap && first.Edge.To == victoryRoad1FMap {
		t.Fatalf("first hop is the 2F -> 1F pad at (%d,%d), which the sealed pocket at (25,14) cannot reach",
			first.Edge.WarpX, first.Edge.WarpY)
	}
	if first.Edge.From != victoryRoad2FMap || first.Edge.To != victoryRoad3FMap {
		t.Fatalf("first hop = %+v, want the pocket's own 2F -> 3F ladder", first.Edge)
	}
	if first.Edge.WarpX != 25 || first.Edge.WarpY != 14 {
		t.Fatalf("first hop pad = (%d,%d), want the pocket ladder (25,14) the player is standing on",
			first.Edge.WarpX, first.Edge.WarpY)
	}
}
