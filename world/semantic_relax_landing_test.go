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
