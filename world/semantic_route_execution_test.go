package world

import (
	"errors"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
)

type fakeTransitionExecutor struct {
	edge       Edge
	transition gameruntime.Transition
	changed    bool
	err        error
}

func (f *fakeTransitionExecutor) ExecuteTransition(edge Edge, transition gameruntime.Transition) (TransitionExecutionResult, error) {
	f.edge, f.transition = edge, transition
	return TransitionExecutionResult{Changed: f.changed}, f.err
}

func TestSemanticRoutePlanPreservesExecutableTransitionAcrossBlockedComponent(t *testing.T) {
	edge := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast}
	g := &Graph{
		Edges:          map[uint8][]Edge{1: {edge}, 2: {}},
		componentAware: true,
		comps: map[uint8][][]int{
			1: {{1, 2}}, // player is component 1; edge exits component 2
			2: {{3}},
		},
		exitComps:  map[Edge][]int{edge: {2}},
		entryComps: map[Edge][]int{edge: {3}},
	}
	transition := gameruntime.Transition{ID: "fake:surf", From: "shore", To: "island", Requires: []gameruntime.CapabilityID{"can_surf"}}
	prereqs := RoutePrerequisites{
		Transitions:  map[Edge]gameruntime.Transition{edge: transition},
		Capabilities: gameruntime.NewCapabilitySet("can_surf"),
	}

	if _, err := FindRouteAtDestination(g, 1, 2, 0, 0, 0, 0, nil); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("ordinary route error = %v, want ErrNoRoute", err)
	}
	plan, err := FindRoutePlanAtDestinationWithCapabilities(g, 1, 2, 0, 0, 0, 0, nil, prereqs)
	if err != nil {
		t.Fatalf("semantic route: %v", err)
	}
	if len(plan) != 1 || plan[0].Edge != edge || plan[0].Transition == nil || plan[0].Transition.ID != transition.ID {
		t.Fatalf("plan = %+v, want one executable fake:surf step", plan)
	}
}

func TestSemanticRouteBlockedBeforeMovementWhenPivotCapabilityMissing(t *testing.T) {
	edge := Edge{Kind: EdgeConnection, From: 1, To: 2, Dir: dirEast}
	g := &Graph{
		Edges:          map[uint8][]Edge{1: {edge}, 2: {}},
		componentAware: true,
		comps:          map[uint8][][]int{1: {{1, 2}}, 2: {{3}}},
		exitComps:      map[Edge][]int{edge: {2}},
		entryComps:     map[Edge][]int{edge: {3}},
	}
	transition := gameruntime.Transition{ID: "fake:cut", From: "a", To: "b", Requires: []gameruntime.CapabilityID{"can_cut"}}
	_, err := FindRoutePlanAtDestinationWithCapabilities(g, 1, 2, 0, 0, 0, 0, nil, RoutePrerequisites{
		Transitions: map[Edge]gameruntime.Transition{edge: transition},
	})
	var blocked *RouteBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("error = %T %v, want *RouteBlockedError", err, err)
	}
	if got := blocked.MissingCapabilities(); len(got) != 1 || got[0] != gameruntime.CapabilityID("can_cut") {
		t.Fatalf("missing = %v, want [can_cut]", got)
	}
}

func TestPortableTransitionExecutorIsTypedAndAdapterAgnostic(t *testing.T) {
	edge := Edge{Kind: EdgeConnection, From: 7, To: 8, Dir: dirSouth}
	transition := gameruntime.Transition{ID: "fake:bridge", From: "north", To: "south"}
	fake := &fakeTransitionExecutor{changed: true}
	result, err := ExecuteTransition(fake, edge, transition)
	if err != nil || !result.Changed {
		t.Fatalf("ExecuteTransition = %+v, %v; want changed success", result, err)
	}
	if fake.edge != edge || fake.transition.ID != transition.ID {
		t.Fatalf("executor observed edge=%+v transition=%+v", fake.edge, fake.transition)
	}

	_, err = ExecuteTransition(nil, edge, transition)
	var execution *TransitionExecutionError
	if !errors.As(err, &execution) || !errors.Is(err, ErrTransitionExecutorUnavailable) {
		t.Fatalf("nil executor error = %T %v, want typed unavailable error", err, err)
	}
}
