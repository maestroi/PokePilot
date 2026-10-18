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

// TestRequirementFreePuzzleCaveFrontierDoesNotLureOutdoorDestination locks the
// shared invariant behind run-d6dokr184ky81: a requirement-free PivotOnly warp
// (Seafoam-style puzzle cave) must not stay as a one-hop ErrRouteReplanRequired
// frontier when a mainland path still reaches the outdoor destination. Otherwise
// mart selection prices the cave mouth as nearer than the real shop.
func TestRequirementFreePuzzleCaveFrontierDoesNotLureOutdoorDestination(t *testing.T) {
	cave := Edge{Kind: EdgeWarp, From: 1, To: 2, WarpX: 4, WarpY: 5}
	mainland := Edge{Kind: EdgeConnection, From: 1, To: 3, Dir: dirWest}
	caveToFar := Edge{Kind: EdgeConnection, From: 2, To: 4, Dir: dirEast}
	farToDest := Edge{Kind: EdgeConnection, From: 4, To: 5, Dir: dirEast}
	mainToDest := Edge{Kind: EdgeConnection, From: 3, To: 5, Dir: dirNorth}
	g := &Graph{
		Edges: map[uint8][]Edge{
			1: {cave, mainland},
			2: {caveToFar},
			3: {mainToDest},
			4: {farToDest},
			5: nil,
		},
		componentAware: true,
		comps: map[uint8][][]int{
			1: {{1}},
			2: {{1}},
			3: {{1}},
			4: {{1}},
			5: {{1}},
		},
		exitComps: map[Edge][]int{
			cave: {1}, mainland: {1}, caveToFar: {1}, farToDest: {1}, mainToDest: {1},
		},
		entryComps: map[Edge][]int{
			cave: {1}, mainland: {1}, caveToFar: {1}, farToDest: {1}, mainToDest: {1},
		},
	}
	prereqs := RoutePrerequisites{
		Transitions: map[Edge]gameruntime.Transition{
			cave: {ID: "fake:puzzle_cave", PivotOnly: true},
		},
	}

	plan, err := FindRoutePlanAtDestinationWithCapabilities(g, 1, 5, 0, 0, 0, 0, nil, prereqs)
	if err != nil {
		t.Fatalf("outdoor destination with mainland alternative: %v (plan=%+v)", err, plan)
	}
	for i, step := range plan {
		if step.Edge == cave || (step.Transition != nil && step.Transition.ID == "fake:puzzle_cave") {
			t.Fatalf("plan still enters the puzzle-cave lure at leg %d: %+v", i, plan)
		}
	}
	if len(plan) != 2 || plan[0].Edge != mainland || plan[1].Edge != mainToDest {
		t.Fatalf("plan = %+v, want mainland -> dest", plan)
	}
}

// TestRequirementFreePuzzleCaveFrontierKeptWhenOnlyEntrance locks the complement:
// when the destination is only reachable by entering the requirement-free cave
// mouth, the frontier must remain so GoTo can step through and replan inside.
func TestRequirementFreePuzzleCaveFrontierKeptWhenOnlyEntrance(t *testing.T) {
	cave := Edge{Kind: EdgeWarp, From: 1, To: 2, WarpX: 4, WarpY: 5}
	inside := Edge{Kind: EdgeConnection, From: 2, To: 3, Dir: dirEast}
	g := &Graph{
		Edges: map[uint8][]Edge{
			1: {cave},
			2: {inside},
			3: nil,
		},
		componentAware: true,
		comps: map[uint8][][]int{
			1: {{1}},
			2: {{1}},
			3: {{1}},
		},
		exitComps:  map[Edge][]int{cave: {1}, inside: {1}},
		entryComps: map[Edge][]int{cave: {1}, inside: {1}},
	}
	prereqs := RoutePrerequisites{
		Transitions: map[Edge]gameruntime.Transition{
			cave: {ID: "fake:puzzle_cave", PivotOnly: true},
		},
	}

	plan, err := FindRoutePlanAtDestinationWithCapabilities(g, 1, 3, 0, 0, 0, 0, nil, prereqs)
	if !errors.Is(err, ErrRouteReplanRequired) {
		t.Fatalf("error = %v, want ErrRouteReplanRequired", err)
	}
	if len(plan) != 1 || plan[0].Edge != cave {
		t.Fatalf("plan = %+v, want cave-mouth frontier", plan)
	}
	if plan[0].Transition == nil || plan[0].Transition.ID != "fake:puzzle_cave" {
		t.Fatalf("frontier transition = %+v, want fake:puzzle_cave", plan[0].Transition)
	}
}

// TestRequirementFreePuzzleCaveSiblingMouthsStillCountAsCaveOnlyEntrance pins
// Route 20's two Seafoam doors: dropping only the first frontier must not make
// the second door look like a mainland path to an interior floor.
func TestRequirementFreePuzzleCaveSiblingMouthsStillCountAsCaveOnlyEntrance(t *testing.T) {
	caveA := Edge{Kind: EdgeWarp, From: 1, To: 2, WarpX: 4, WarpY: 5}
	caveB := Edge{Kind: EdgeWarp, From: 1, To: 2, WarpX: 8, WarpY: 9}
	inside := Edge{Kind: EdgeConnection, From: 2, To: 3, Dir: dirEast}
	g := &Graph{
		Edges: map[uint8][]Edge{
			1: {caveA, caveB},
			2: {inside},
			3: nil,
		},
		componentAware: true,
		comps: map[uint8][][]int{
			1: {{1}},
			2: {{1}},
			3: {{1}},
		},
		exitComps:  map[Edge][]int{caveA: {1}, caveB: {1}, inside: {1}},
		entryComps: map[Edge][]int{caveA: {1}, caveB: {1}, inside: {1}},
	}
	prereqs := RoutePrerequisites{
		Transitions: map[Edge]gameruntime.Transition{
			caveA: {ID: "fake:puzzle_cave", PivotOnly: true},
			caveB: {ID: "fake:puzzle_cave", PivotOnly: true},
		},
	}

	plan, err := FindRoutePlanAtDestinationWithCapabilities(g, 1, 3, 0, 0, 0, 0, nil, prereqs)
	if !errors.Is(err, ErrRouteReplanRequired) {
		t.Fatalf("error = %v plan=%+v, want cave-mouth ErrRouteReplanRequired", err, plan)
	}
	if len(plan) != 1 {
		t.Fatalf("plan = %+v, want one cave-mouth frontier", plan)
	}
	if plan[0].Edge != caveA && plan[0].Edge != caveB {
		t.Fatalf("plan = %+v, want one of the cave mouths", plan)
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
