package world

import (
	"errors"
	"reflect"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/worldmodel"
)

func TestSemanticRouteReportsMissingCapabilityThenUnlocks(t *testing.T) {
	e12 := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast}
	e23 := Edge{Kind: EdgeConnection, From: 2, To: 3, Dir: dirEast}
	g := &Graph{Edges: map[uint8][]Edge{
		1: {e12},
		2: {e23},
		3: {},
	}}
	transition := gameruntime.Transition{
		ID:       "bridge_gate",
		From:     "field",
		To:       "bridge",
		Requires: []gameruntime.CapabilityID{"can_cross_bridge"},
	}
	prereqs := RoutePrerequisites{
		Transitions: map[Edge]gameruntime.Transition{e23: transition},
	}

	_, err := FindRouteAtDestinationWithCapabilities(g, 1, 3, 0, 0, 0, 0, nil, prereqs)
	if !errors.Is(err, ErrNoRoute) {
		t.Fatalf("error = %v, want errors.Is(ErrNoRoute)", err)
	}
	var blocked *RouteBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("error = %T %v, want *RouteBlockedError", err, err)
	}
	if got, want := blocked.MissingCapabilities(), []gameruntime.CapabilityID{"can_cross_bridge"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("missing = %v, want %v", got, want)
	}

	prereqs.Capabilities = gameruntime.NewCapabilitySet("can_cross_bridge")
	got, err := FindRouteAtDestinationWithCapabilities(g, 1, 3, 0, 0, 0, 0, nil, prereqs)
	if err != nil {
		t.Fatalf("unlocked route: %v", err)
	}
	if !reflect.DeepEqual(got, []Edge{e12, e23}) {
		t.Fatalf("route = %v, want [%v %v]", got, e12, e23)
	}
}

// TestSemanticGateStaysSubjectToComponentReachability separates the two
// things a capability can mean. An ACTION (Cut, Surf, Strength) creates
// traversal, so a satisfied one is a pivot: routing may take its port even
// though ordinary walking cannot reach it. A GATE creates nothing — something
// merely stops standing in the way — so satisfying it must not make the far
// map's disconnected rooms look adjacent.
//
// The graph here is Mt. Moon B2F in miniature: two components on the map the
// gated edge leaves from, and a start in the component the port is NOT in.
// Satisfying the gate must still leave that unroutable; the same edge marked
// as an action is the pivot case, and is expected to route.
func TestSemanticGateStaysSubjectToComponentReachability(t *testing.T) {
	gated := Edge{Kind: EdgeWarp, From: 1, To: 2, WarpX: 0, WarpY: 0}
	g := &Graph{
		componentAware: true,
		Edges:          map[uint8][]Edge{1: {gated}, 2: {}},
		// Map 1 is two rooms: the port's room (component 1) and the room
		// the player stands in (component 2), with no walk between them.
		comps:      map[uint8][][]int{1: {{1, 0, 2}}, 2: {{1}}},
		tiles:      map[uint8]dim{1: {w: 3, h: 1}, 2: {w: 1, h: 1}},
		warps:      map[uint8][]worldmodel.Warp{1: {{X: 0, Y: 0, DestMap: 2}}, 2: {{X: 0, Y: 0, DestMap: 1}}},
		exitComps:  map[Edge][]int{gated: {1}},
		entryComps: map[Edge][]int{gated: {1}},
	}
	prereqs := RoutePrerequisites{
		Capabilities: gameruntime.NewCapabilitySet("can_pass"),
		Transitions: map[Edge]gameruntime.Transition{gated: {
			ID:       "gate",
			Requires: []gameruntime.CapabilityID{"can_pass"},
			Gate:     true,
		}},
	}
	if _, err := FindRouteAtDestinationWithCapabilities(g, 1, 2, 2, 0, 0, 0, nil, prereqs); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("satisfied gate routed out of a room the port is not in: %v", err)
	}

	action := prereqs
	action.Transitions = map[Edge]gameruntime.Transition{gated: {
		ID:       "action",
		Requires: []gameruntime.CapabilityID{"can_pass"},
	}}
	if _, err := FindRouteAtDestinationWithCapabilities(g, 1, 2, 2, 0, 0, 0, nil, action); err != nil {
		t.Fatalf("satisfied action is a pivot and must route: %v", err)
	}
}

func TestMissingPivotOnlyCapabilityFallsBackToOrdinaryGeometry(t *testing.T) {
	pivot := Edge{Kind: EdgeWarp, From: 1, To: 2, WarpX: 0, WarpY: 0}
	g := &Graph{
		componentAware: true,
		Edges:          map[uint8][]Edge{1: {pivot}, 2: {}},
		// Component 1 can reach the underlying edge normally; component 2 can
		// reach it only when the semantic action is available as a pivot.
		comps:      map[uint8][][]int{1: {{1, 0, 2}}, 2: {{1}}},
		tiles:      map[uint8]dim{1: {w: 3, h: 1}, 2: {w: 1, h: 1}},
		warps:      map[uint8][]worldmodel.Warp{1: {{X: 0, Y: 0, DestMap: 2}}, 2: {{X: 0, Y: 0, DestMap: 1}}},
		exitComps:  map[Edge][]int{pivot: {1}},
		entryComps: map[Edge][]int{pivot: {1}},
	}
	prereqs := RoutePrerequisites{
		Transitions: map[Edge]gameruntime.Transition{pivot: {
			ID:        "optional_component_pivot",
			Requires:  []gameruntime.CapabilityID{"can_pivot"},
			PivotOnly: true,
		}},
	}

	// Already on the edge's ordinary component: the missing capability must
	// not remove a physically reachable escape edge, and execution must not
	// receive a semantic transition it cannot satisfy.
	plan, err := FindRoutePlanAtDestinationWithCapabilities(g, 1, 2, 0, 0, 0, 0, nil, prereqs)
	if err != nil {
		t.Fatalf("ordinary pivot-only fallback: %v", err)
	}
	if len(plan) != 1 || plan[0].Edge != pivot || plan[0].Transition != nil {
		t.Fatalf("ordinary pivot-only plan = %+v, want one ordinary edge", plan)
	}

	// From the disconnected component the capability is still genuinely
	// required, so preserve the structured prerequisite diagnosis.
	_, err = FindRoutePlanAtDestinationWithCapabilities(g, 1, 2, 2, 0, 0, 0, nil, prereqs)
	var blocked *RouteBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("disconnected pivot-only error = %T %v, want *RouteBlockedError", err, err)
	}
	if got, want := blocked.MissingCapabilities(), []gameruntime.CapabilityID{"can_pivot"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("missing = %v, want %v", got, want)
	}

	// Once the capability exists the same edge becomes an executable pivot and
	// may bridge the static component split.
	prereqs.Capabilities = gameruntime.NewCapabilitySet("can_pivot")
	plan, err = FindRoutePlanAtDestinationWithCapabilities(g, 1, 2, 2, 0, 0, 0, nil, prereqs)
	if err != nil {
		t.Fatalf("enabled pivot-only route: %v", err)
	}
	if len(plan) != 1 || plan[0].Transition == nil || plan[0].Transition.ID != "optional_component_pivot" {
		t.Fatalf("enabled pivot-only plan = %+v, want executable transition", plan)
	}
}

// TestPivotOnlyPreservesDestinationLandingComponents is the Route 9/10 contract:
// a PivotOnly Cut on the source map must not discard the destination map's
// static landing component and invent reachability across that map's own split
// (Route 10 north vs south toward Lavender).
func TestPivotOnlyPreservesDestinationLandingComponents(t *testing.T) {
	cross := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast}
	south := Edge{Kind: EdgeConnection, From: 2, To: 3, Dir: dirSouth}
	cave := Edge{Kind: EdgeWarp, From: 2, To: 3, WarpX: 0, WarpY: 0}
	g := &Graph{
		componentAware: true,
		Edges: map[uint8][]Edge{
			1: {cross},
			2: {south, cave},
			3: {},
		},
		// Map 2 has north (comp 1) and south (comp 2). Cross from map 1 lands
		// on north. South exit is only on south. Cave bridges north -> south dest.
		comps: map[uint8][][]int{
			1: {{1}},
			2: {{1}, {2}},
			3: {{1}},
		},
		tiles: map[uint8]dim{1: {w: 1, h: 1}, 2: {w: 1, h: 2}, 3: {w: 1, h: 1}},
		exitComps: map[Edge][]int{
			cross: {1},
			south: {2},
			cave:  {1},
		},
		entryComps: map[Edge][]int{
			cross: {1}, // lands on north of map 2
			south: {1},
			cave:  {1},
		},
	}
	prereqs := RoutePrerequisites{
		Capabilities: gameruntime.NewCapabilitySet("can_cut"),
		Transitions: map[Edge]gameruntime.Transition{
			cross: {
				ID:        "source_cut_pivot",
				Requires:  []gameruntime.CapabilityID{"can_cut"},
				PivotOnly: true,
			},
		},
	}

	plan, err := FindRoutePlanAtDestinationWithCapabilities(g, 1, 3, 0, 0, 0, 0, nil, prereqs)
	if err != nil {
		t.Fatalf("pivot-only route: %v", err)
	}
	sawCave := false
	sawSouth := false
	for _, step := range plan {
		if step.Edge == cave {
			sawCave = true
		}
		if step.Edge == south {
			sawSouth = true
		}
	}
	if !sawCave || sawSouth {
		t.Fatalf("plan=%+v: PivotOnly must keep the north landing and use the cave, not invent the south exit", plan)
	}
}

func TestSemanticRouteDoesNotInventPrerequisiteForGeometricFailure(t *testing.T) {
	gated := Edge{Kind: EdgeConnection, From: 9, To: 10, Dir: dirEast}
	g := &Graph{Edges: map[uint8][]Edge{
		1:  {},
		9:  {gated},
		10: {},
	}}
	prereqs := RoutePrerequisites{
		Transitions: map[Edge]gameruntime.Transition{
			gated: {
				ID:       "unrelated_gate",
				From:     "elsewhere",
				To:       "other",
				Requires: []gameruntime.CapabilityID{"irrelevant"},
			},
		},
	}
	_, err := FindRouteAtDestinationWithCapabilities(g, 1, 10, 0, 0, 0, 0, nil, prereqs)
	var blocked *RouteBlockedError
	if errors.As(err, &blocked) {
		t.Fatalf("unreachable geometry was mislabeled as semantic blockage: %+v", blocked)
	}
	if !errors.Is(err, ErrNoRoute) {
		t.Fatalf("error = %v, want ErrNoRoute", err)
	}
}
