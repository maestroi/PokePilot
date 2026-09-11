package skill

import (
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

func setRouteTestEvent(mem *state.Mem, event state.Event) {
	index := uint16(event)
	mem[sym.EventFlags+index/8] |= 1 << (index % 8)
}

func TestRedRouteCapabilitiesProjectEarlyStoryGates(t *testing.T) {
	mem := new(state.Mem)
	caps := redRouteCapabilities(nil, mem)
	if caps.Has(capCanLeaveViridianNorth) || caps.Has(capCanLeavePewterEast) {
		t.Fatalf("fresh state unexpectedly passes early route gates: %v", caps)
	}

	setRouteTestEvent(mem, state.EventGotPokedex)
	caps = redRouteCapabilities(nil, mem)
	if !caps.Has(capCanLeaveViridianNorth) {
		t.Fatalf("Pokedex story fact did not project %q: %v", capCanLeaveViridianNorth, caps)
	}
	if caps.Has(capCanLeavePewterEast) {
		t.Fatalf("Pokedex alone incorrectly passed Pewter east gate: %v", caps)
	}

	mem[sym.ObtainedBadges] |= 1 << uint8(state.BadgeBoulder)
	caps = redRouteCapabilities(nil, mem)
	if !caps.Has(capCanLeavePewterEast) {
		t.Fatalf("Boulder Badge did not project %q: %v", capCanLeavePewterEast, caps)
	}
}

func TestRedRouteTransitionsAttachEarlyProgressionToEdges(t *testing.T) {
	for _, tc := range []struct {
		name string
		edge world.Edge
		want gameruntime.CapabilityID
	}{
		{
			name: "viridian north",
			edge: world.Edge{Kind: world.EdgeConnection, From: semanticViridianCityMap, To: semanticRoute2Map},
			want: capCanLeaveViridianNorth,
		},
		{
			name: "pewter east",
			edge: world.Edge{Kind: world.EdgeConnection, From: semanticPewterCityMap, To: semanticRoute3Map},
			want: capCanLeavePewterEast,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transition, ok := redRouteTransitionForEdge(tc.edge)
			if !ok {
				t.Fatalf("edge %+v has no progression transition", tc.edge)
			}
			if !transition.Gate {
				t.Fatalf("transition %+v is an executable pivot; want a pure story gate", transition)
			}
			if len(transition.Requires) != 1 || transition.Requires[0] != tc.want {
				t.Fatalf("requires = %v, want [%s]", transition.Requires, tc.want)
			}
		})
	}
}

func TestEarlyProgressionGatesDoNotBlockReverseReturn(t *testing.T) {
	for _, edge := range []world.Edge{
		{Kind: world.EdgeConnection, From: semanticRoute2Map, To: semanticViridianCityMap},
		{Kind: world.EdgeConnection, From: semanticRoute3Map, To: semanticPewterCityMap},
	} {
		if transition, ok := redRouteTransitionForEdge(edge); ok &&
			(transition.ID == "red:viridian_north_pokedex" || transition.ID == "red:pewter_east_boulder") {
			t.Fatalf("reverse edge %+v was incorrectly story-gated: %+v", edge, transition)
		}
	}
}
