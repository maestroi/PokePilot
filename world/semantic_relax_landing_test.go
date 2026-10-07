package world

import (
	"errors"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
)

func TestSemanticRelaxLandingStopsBeforeUnrelatedDestinationComponent(t *testing.T) {
	pivot := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast}
	onward := Edge{Kind: EdgeConnection, From: 2, To: 3, Dir: dirEast}
	g := &Graph{
		Edges: map[uint8][]Edge{
			1: {pivot},
			2: {onward},
			3: nil,
		},
		componentAware: true,
		comps: map[uint8][][]int{
			1: {{1}},
			2: {{1, 2}}, // landing is component 1; onward exit is component 2
			3: {{1}},
		},
		exitComps: map[Edge][]int{
			pivot:  {1},
			onward: {2},
		},
		entryComps: map[Edge][]int{
			pivot:  {1},
			onward: {1},
		},
	}
	transition := gameruntime.Transition{
		ID:        "fake:local-cut",
		Requires:  []gameruntime.CapabilityID{"can_cut"},
		PivotOnly: true,
	}
	prereqs := RoutePrerequisites{
		Transitions:  map[Edge]gameruntime.Transition{pivot: transition},
		Capabilities: gameruntime.NewCapabilitySet("can_cut"),
	}

	plan, err := FindRoutePlanAtDestinationWithCapabilities(
		g, 1, 3, 0, 0, 0, 0, nil, prereqs,
	)
	if !errors.Is(err, ErrRouteReplanRequired) {
		t.Fatalf("error = %v, want ErrRouteReplanRequired", err)
	}
	if len(plan) != 1 || plan[0].Edge != pivot {
		t.Fatalf("plan = %+v, want only the semantic frontier", plan)
	}
	if plan[0].Transition == nil || plan[0].Transition.ID != transition.ID {
		t.Fatalf("frontier transition = %+v, want %q", plan[0].Transition, transition.ID)
	}
}

func TestSemanticRelaxLandingStillAllowsPhysicallySupportedDestination(t *testing.T) {
	pivot := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast}
	g := &Graph{
		Edges:          map[uint8][]Edge{1: {pivot}, 2: nil},
		componentAware: true,
		comps: map[uint8][][]int{
			1: {{1}},
			2: {{7, 8}},
		},
		exitComps:  map[Edge][]int{pivot: {1}},
		entryComps: map[Edge][]int{pivot: {7}},
	}
	prereqs := RoutePrerequisites{
		Transitions: map[Edge]gameruntime.Transition{
			pivot: {
				ID:        "fake:local-cut",
				PivotOnly: true,
			},
		},
	}

	plan, err := FindRoutePlanAtDestinationWithCapabilities(
		g, 1, 2, 0, 0, 0, 0, nil, prereqs,
	)
	if err != nil {
		t.Fatalf("physically supported landing should complete: %v", err)
	}
	if len(plan) != 1 || plan[0].Edge != pivot {
		t.Fatalf("plan = %+v, want pivot edge", plan)
	}
}

func TestSemanticReplanUsesRefreshedComponentTopology(t *testing.T) {
	onward := Edge{Kind: EdgeConnection, From: 2, To: 3, Dir: dirEast}
	before := &Graph{
		Edges:          map[uint8][]Edge{2: {onward}, 3: nil},
		componentAware: true,
		comps: map[uint8][][]int{
			2: {{1, 2}},
			3: {{1}},
		},
		exitComps:  map[Edge][]int{onward: {2}},
		entryComps: map[Edge][]int{onward: {1}},
	}
	if _, err := FindRoutePlanAtDestinationWithCapabilities(
		before, 2, 3, 0, 0, 0, 0, nil, RoutePrerequisites{},
	); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("pre-action route error = %v, want ErrNoRoute", err)
	}

	// Simulate the fresh live topology after the local action: the landing
	// region and onward exit are now physically connected.
	after := *before
	after.comps = map[uint8][][]int{
		2: {{1, 1}},
		3: {{1}},
	}
	after.exitComps = map[Edge][]int{onward: {1}}

	plan, err := FindRoutePlanAtDestinationWithCapabilities(
		&after, 2, 3, 0, 0, 0, 0, nil, RoutePrerequisites{},
	)
	if err != nil {
		t.Fatalf("replan on refreshed topology: %v", err)
	}
	if len(plan) != 1 || plan[0].Edge != onward {
		t.Fatalf("refreshed plan = %+v, want onward edge", plan)
	}
}

// A frontier must not make a target look reachable when nothing beyond it can
// lead there: the only way on is an action whose capability is missing.
func TestSemanticFrontierDoesNotHideBlockedDestination(t *testing.T) {
	pivot := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast}
	onward := Edge{Kind: EdgeConnection, From: 2, To: 3, Dir: dirEast}
	locked := Edge{Kind: EdgeConnection, From: 3, To: 4, Dir: dirEast}
	g := &Graph{
		Edges:          map[uint8][]Edge{1: {pivot}, 2: {onward}, 3: {locked}, 4: nil},
		componentAware: true,
		comps: map[uint8][][]int{
			1: {{1}},
			2: {{1, 2}},
			3: {{1}},
			4: {{1}},
		},
		exitComps:  map[Edge][]int{pivot: {1}, onward: {2}, locked: {1}},
		entryComps: map[Edge][]int{pivot: {1}, onward: {1}, locked: {1}},
	}
	prereqs := RoutePrerequisites{
		Transitions: map[Edge]gameruntime.Transition{
			pivot:  {ID: "fake:local-cut", Requires: []gameruntime.CapabilityID{"can_cut"}, PivotOnly: true},
			locked: {ID: "fake:sleeping-blocker", Requires: []gameruntime.CapabilityID{"can_wake"}},
		},
		Capabilities: gameruntime.NewCapabilitySet("can_cut"),
	}

	_, err := FindRoutePlanAtDestinationWithCapabilities(g, 1, 4, 0, 0, 0, 0, nil, prereqs)
	if errors.Is(err, ErrRouteReplanRequired) || !errors.Is(err, ErrNoRoute) {
		t.Fatalf("error = %v, want no-route (the frontier cannot reach map 4)", err)
	}
}

// A missing PivotOnly capability keeps its edge as ordinary geometry, so a
// component-blind check past an unrelated frontier treated a Surf-only shore as
// connected and GoTo walked to the frontier before failing on the shore
// (run-d6dokr184ky81: a Route 2 Cut tree made the Power Plant look routable
// without Surf). Only the frontier's own landing is unknown topology; the
// shore's port on map 3 is still unreachable by walking, so the answer must be
// the structured Surf blockage before any movement.
func TestSemanticFrontierDoesNotBridgeMissingPivotOnlyShore(t *testing.T) {
	pivot := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast}
	onward := Edge{Kind: EdgeConnection, From: 2, To: 3, Dir: dirEast}
	shore := Edge{Kind: EdgeWarp, From: 3, To: 4}
	g := &Graph{
		Edges:          map[uint8][]Edge{1: {pivot}, 2: {onward}, 3: {shore}, 4: nil},
		componentAware: true,
		comps: map[uint8][][]int{
			1: {{1}},
			2: {{1, 2}},
			3: {{1, 2}}, // land is component 1; the shore port is island component 2
			4: {{1}},
		},
		exitComps:  map[Edge][]int{pivot: {1}, onward: {2}, shore: {2}},
		entryComps: map[Edge][]int{pivot: {1}, onward: {1}, shore: {1}},
	}
	prereqs := RoutePrerequisites{
		Transitions: map[Edge]gameruntime.Transition{
			pivot: {ID: "fake:local-cut", Requires: []gameruntime.CapabilityID{"can_cut"}, PivotOnly: true},
			shore: {ID: "fake:surf-shore", Requires: []gameruntime.CapabilityID{"can_surf"}, PivotOnly: true, PortBypass: true},
		},
		Capabilities: gameruntime.NewCapabilitySet("can_cut"),
	}

	_, err := FindRoutePlanAtDestinationWithCapabilities(g, 1, 4, 0, 0, -1, -1, nil, prereqs)
	var blocked *RouteBlockedError
	if errors.Is(err, ErrRouteReplanRequired) || !errors.As(err, &blocked) {
		t.Fatalf("error = %T %v, want *RouteBlockedError for the Surf shore", err, err)
	}
	if missing := blocked.MissingCapabilities(); len(missing) != 1 || missing[0] != "can_surf" {
		t.Fatalf("missing = %v, want [can_surf]", missing)
	}

	// With Surf the shore is an executable pivot again, so the frontier defers.
	prereqs.Capabilities = gameruntime.NewCapabilitySet("can_cut", "can_surf")
	if _, err := FindRoutePlanAtDestinationWithCapabilities(g, 1, 4, 0, 0, -1, -1, nil, prereqs); !errors.Is(err, ErrRouteReplanRequired) {
		t.Fatalf("with can_surf error = %v, want ErrRouteReplanRequired", err)
	}
}
