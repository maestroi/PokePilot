package world

import (
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
)

// gatedWarpFixture builds a two-map graph whose start map has a semantic gated
// warp pair to map 2: gatedA sits in a component the player cannot reach and
// gatedB sits in the player's own component. Both carry the same non-gate
// transition, so both enjoy skipCanExit — the privilege that lets a boulder
// switch or scripted door be selected before its passage exists.
type gatedWarpFixture struct {
	graph   *Graph
	prereqs RoutePrerequisites
	gatedA  Edge
	gatedB  Edge
}

func newGatedWarpFixture(landingA, landingB []int, exitA, exitB []int) gatedWarpFixture {
	gatedA := Edge{Kind: EdgeWarp, From: 1, To: 2, WarpX: 1, WarpY: 0}
	gatedB := Edge{Kind: EdgeWarp, From: 1, To: 2, WarpX: 2, WarpY: 0}
	g := &Graph{
		componentAware: true,
		Edges: map[uint8][]Edge{
			1: {gatedA, gatedB},
			2: nil,
		},
		// Map 1 is one row: (0,0) and (2,0) are the player's component, (1,0)
		// and (3,0) are not walkable. Map 2 holds the two landing regions.
		comps: map[uint8][][]int{
			1: {{1, 0, 1, 0}},
			2: {{1, 2}},
		},
		tiles: map[uint8]dim{1: {w: 4, h: 1}, 2: {w: 2, h: 1}},
		exitComps: map[Edge][]int{
			gatedA: exitA,
			gatedB: exitB,
		},
		entryComps: map[Edge][]int{
			gatedA: landingA,
			gatedB: landingB,
		},
	}
	prereqs := RoutePrerequisites{
		Capabilities: gameruntime.NewCapabilitySet("can_push_boulders"),
		Transitions: map[Edge]gameruntime.Transition{
			gatedA: {ID: "strength_switch", Requires: []gameruntime.CapabilityID{"can_push_boulders"}},
			gatedB: {ID: "strength_switch", Requires: []gameruntime.CapabilityID{"can_push_boulders"}},
		},
	}
	return gatedWarpFixture{graph: g, prereqs: prereqs, gatedA: gatedA, gatedB: gatedB}
}

// TestGatedWarpPrefersReachableSiblingPad locks the shared invariant behind
// farm run-3dtp99mx0jn3ickoqlj1k6iue: the player resumed inside Victory Road
// 2F's sealed ladder pocket, standing on the warp to 3F, while the same map
// also holds three other warps to 3F whose pads the pocket cannot reach.
// skipCanExit let the search offer the first of those (ROM order) instead, the
// Red strength executor was handed an edge it could not open, and the journey
// died as transition_execution_failed ("push puzzle has no solution") even
// though a sibling pad the player was standing on needed no action at all.
//
// Rule: an unreachable gated warp must not outrank a sibling warp of the same
// map pair whose pad the player can reach and which lands in the same
// destination region.
func TestGatedWarpPrefersReachableSiblingPad(t *testing.T) {
	f := newGatedWarpFixture([]int{1}, []int{1}, []int{2}, []int{1})

	plan, err := FindRoutePlanAtDestinationWithCapabilities(f.graph, 1, 2, 0, 0, -1, -1, nil, f.prereqs)
	if err != nil {
		t.Fatalf("route to map 2: %v", err)
	}
	if len(plan) != 1 || plan[0].Edge != f.gatedB {
		t.Fatalf("plan=%+v, want the reachable sibling pad %+v (not the unreachable %+v)", plan, f.gatedB, f.gatedA)
	}
	if plan[0].Transition == nil {
		t.Fatalf("plan lost the semantic transition identity: %+v", plan[0])
	}
}

// TestGatedWarpKeepsDistinctLandingRegionSelectable is the guard against
// over-pruning: when the sibling lands in a DIFFERENT destination region, the
// unreachable pad is not a substitute. Selecting it can be exactly what the
// semantic pivot is for (its action opens that region), so it must stay
// selectable.
func TestGatedWarpKeepsDistinctLandingRegionSelectable(t *testing.T) {
	f := newGatedWarpFixture([]int{2}, []int{1}, []int{2}, []int{1})

	plan, err := FindRoutePlanAtDestinationWithCapabilities(f.graph, 1, 2, 0, 0, 1, 0, nil, f.prereqs)
	if err != nil {
		t.Fatalf("route to map 2 (1,0): %v", err)
	}
	if len(plan) != 1 || plan[0].Edge != f.gatedA {
		t.Fatalf("plan=%+v, want the pivot pad %+v whose landing region reaches (1,0)", plan, f.gatedA)
	}
}

// TestGatedWarpStaysSelectableWithoutReachableSibling locks the fallback the
// privilege exists for: from Victory Road 2F's west side the boulder switch
// has to be solved before either pad is walkable, so no sibling is reachable
// and the router must still offer the gated warp rather than fail the map.
func TestGatedWarpStaysSelectableWithoutReachableSibling(t *testing.T) {
	f := newGatedWarpFixture([]int{2}, []int{2}, []int{2}, []int{2})

	plan, err := FindRoutePlanAtDestinationWithCapabilities(f.graph, 1, 2, 0, 0, -1, -1, nil, f.prereqs)
	if err != nil {
		t.Fatalf("route to map 2: %v", err)
	}
	if len(plan) != 1 || plan[0].Edge != f.gatedA {
		t.Fatalf("plan=%+v, want the gated warp %+v to stay selectable without a reachable sibling", plan, f.gatedA)
	}
}
