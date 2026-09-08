package world

import (
	"errors"
	"reflect"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
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
