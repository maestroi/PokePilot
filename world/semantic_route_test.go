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

	// From the disconnected component, pure PivotOnly cannot help: it only
	// relaxes the far landing. Missing can_pivot is not the reason this is
	// unroutable, so do not invent a RouteBlockedError for it.
	_, err = FindRoutePlanAtDestinationWithCapabilities(g, 1, 2, 2, 0, 0, 0, nil, prereqs)
	if !errors.Is(err, ErrNoRoute) {
		t.Fatalf("disconnected pure pivot-only error = %v, want ErrNoRoute", err)
	}
	var blocked *RouteBlockedError
	if errors.As(err, &blocked) {
		t.Fatalf("disconnected pure pivot-only mislabeled as capability blockage: %+v", blocked)
	}

	// Once the capability exists, PivotOnly still does not invent FROM-side
	// port reachability — the obstacle lives on the adjacent map. PortBypass
	// (or a normal FROM-side action) is what bridges a static split on this
	// map. A satisfied PivotOnly from the wrong component must stay unroutable.
	prereqs.Capabilities = gameruntime.NewCapabilitySet("can_pivot")
	if _, err = FindRoutePlanAtDestinationWithCapabilities(g, 1, 2, 2, 0, 0, 0, nil, prereqs); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("enabled pivot-only from unreachable component routed: %v", err)
	}

	// PortBypass+PivotOnly is the FROM-side bridge. Without the capability,
	// preserve structured prerequisite evidence; with it, the edge is usable.
	prereqs.Capabilities = nil
	prereqs.Transitions = map[Edge]gameruntime.Transition{pivot: {
		ID:         "optional_component_pivot",
		Requires:   []gameruntime.CapabilityID{"can_pivot"},
		PivotOnly:  true,
		PortBypass: true,
	}}
	_, err = FindRoutePlanAtDestinationWithCapabilities(g, 1, 2, 2, 0, 0, 0, nil, prereqs)
	if !errors.As(err, &blocked) {
		t.Fatalf("disconnected port-bypass pivot error = %T %v, want *RouteBlockedError", err, err)
	}
	if got, want := blocked.MissingCapabilities(), []gameruntime.CapabilityID{"can_pivot"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("missing = %v, want %v", got, want)
	}

	prereqs.Capabilities = gameruntime.NewCapabilitySet("can_pivot")
	plan, err = FindRoutePlanAtDestinationWithCapabilities(g, 1, 2, 2, 0, 0, 0, nil, prereqs)
	if err != nil {
		t.Fatalf("enabled port-bypass pivot route: %v", err)
	}
	if len(plan) != 1 || plan[0].Transition == nil || plan[0].Transition.ID != "optional_component_pivot" {
		t.Fatalf("enabled port-bypass plan = %+v, want executable transition", plan)
	}
}

// TestPivotOnlyReentryDoesNotUnlockUnreachableExits is the generic shape of
// farm #1261. A TO-side pivot may relax landing on the adjacent map so the
// walker can continue past that map's static split. It must not treat
// leave-and-immediately-return as a teleport onto a different component of
// the origin map. Otherwise an east-seam standing tile plans
// "leave, re-enter, take a plaza-only exit" and GoTo oscillates forever.
func TestPivotOnlyReentryDoesNotUnlockUnreachableExits(t *testing.T) {
	toNeighbor := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast}
	fromNeighbor := Edge{Kind: EdgeConnection, From: 2, To: 1, Dir: dirWest}
	toDest := Edge{Kind: EdgeConnection, From: 1, To: 3, Dir: dirWest}
	g := &Graph{
		componentAware: true,
		Edges: map[uint8][]Edge{
			1: {toNeighbor, toDest},
			2: {fromNeighbor},
			3: {},
		},
		comps: map[uint8][][]int{
			1: {{1, 0, 2}},
			2: {{1}},
			3: {{1}},
		},
		tiles: map[uint8]dim{1: {w: 3, h: 1}, 2: {w: 1, h: 1}, 3: {w: 1, h: 1}},
		exitComps: map[Edge][]int{
			toNeighbor:   {2},
			fromNeighbor: {1},
			toDest:       {1},
		},
		entryComps: map[Edge][]int{
			toNeighbor:   {1},
			fromNeighbor: {2},
			toDest:       {1},
		},
	}
	prereqs := RoutePrerequisites{
		Capabilities: gameruntime.NewCapabilitySet("can_cut"),
		Transitions: map[Edge]gameruntime.Transition{
			toNeighbor: {
				ID:        "cut",
				Requires:  []gameruntime.CapabilityID{"can_cut"},
				PivotOnly: true,
			},
			fromNeighbor: {
				ID:        "cut",
				Requires:  []gameruntime.CapabilityID{"can_cut"},
				PivotOnly: true,
			},
		},
	}

	plan, err := FindRoutePlanAtDestinationWithCapabilities(g, 1, 3, 2, 0, 0, 0, nil, prereqs)
	if !errors.Is(err, ErrNoRoute) {
		t.Fatalf("east seam routed to a plaza-only destination via pivot bounce: err=%v plan=%+v", err, plan)
	}
}

// TestUnknownStartComponentStillHonorsDestinationLanding is the other half of
// #1261. Cerulean (19,28) has no static walkable component, so the old
// planner discarded the Route 4 (10,10) target and treated any landing on
// map 0x0F as arrival. Missing start-tile evidence must not erase the
// destination component.
func TestUnknownStartComponentStillHonorsDestinationLanding(t *testing.T) {
	wrong := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast}
	g := &Graph{
		componentAware: true,
		Edges:          map[uint8][]Edge{1: {wrong}, 2: {}},
		// Tile (0,0) is unwalkable (no start component). Dest (1,0) is
		// component 2; the only edge lands in component 1.
		comps:      map[uint8][][]int{1: {{0, 0}}, 2: {{1, 2}}},
		tiles:      map[uint8]dim{1: {w: 2, h: 1}, 2: {w: 2, h: 1}},
		exitComps:  map[Edge][]int{wrong: {1}},
		entryComps: map[Edge][]int{wrong: {1}},
	}

	plan, err := FindRouteAtDestination(g, 1, 2, 0, 0, 1, 0, nil)
	if !errors.Is(err, ErrNoRoute) {
		t.Fatalf("unknown start component accepted a landing that is not the dest tile: err=%v plan=%+v", err, plan)
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

// TestPortBypassPivotOnlyRejectsPhantomConnectionBands is the generic shape of
// farm triage bd4bb51aa6b8d736 (run-3fl35ipa0wah83axgm9bz5mqc9): an interior
// Cut annotated PortBypass+PivotOnly must not skip canExit on padding bands
// whose exitComps are empty. Those bands land with unknown component and
// unlock every exit on the far map, so Rock Tunnel's north pocket invents a
// Route 9 Cut hop onto Route 10 south instead of traversing B1F.
func TestPortBypassPivotOnlyRejectsPhantomConnectionBands(t *testing.T) {
	real := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast, BandStart: 0, BandEnd: 0, BandScoped: true}
	phantom := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast, BandStart: 1, BandEnd: 1, BandScoped: true}
	toDest := Edge{Kind: EdgeConnection, From: 2, To: 3, Dir: dirSouth, BandStart: 0, BandEnd: 0, BandScoped: true}
	g := &Graph{
		componentAware: true,
		Edges: map[uint8][]Edge{
			1: {real, phantom},
			2: {toDest},
			3: {},
		},
		comps: map[uint8][][]int{
			1: {{1}},
			2: {{1, 0}, {0, 2}}, // north component 1, south component 2
			3: {{2}},
		},
		tiles: map[uint8]dim{1: {w: 1, h: 1}, 2: {w: 2, h: 2}, 3: {w: 1, h: 1}},
		exitComps: map[Edge][]int{
			real:    {1},
			phantom: {}, // padding: no walkable exit tile
			toDest:  {2}, // only south component of map 2 reaches dest
		},
		entryComps: map[Edge][]int{
			real:    {1}, // lands on map 2 north
			phantom: {},
			toDest:  {2},
		},
	}
	prereqs := RoutePrerequisites{
		Capabilities: gameruntime.NewCapabilitySet("can_cut"),
		Transitions: map[Edge]gameruntime.Transition{
			real: {
				ID:         "cut_bridge",
				Requires:   []gameruntime.CapabilityID{"can_cut"},
				PivotOnly:  true,
				PortBypass: true,
			},
			phantom: {
				ID:         "cut_bridge",
				Requires:   []gameruntime.CapabilityID{"can_cut"},
				PivotOnly:  true,
				PortBypass: true,
			},
		},
	}

	// Standing on map 1, dest is map 3 which is only reachable via map 2 south.
	// The real PortBypass hop lands on map 2 north and must NOT unlock south.
	_, err := FindRoutePlanAtDestinationWithCapabilities(g, 1, 3, 0, 0, 0, 0, nil, prereqs)
	if !errors.Is(err, ErrNoRoute) {
		t.Fatalf("phantom PortBypass+PivotOnly invented a south exit: err=%v", err)
	}
}
