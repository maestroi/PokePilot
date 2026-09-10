package skill

import (
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

func TestRedRouteCapabilitiesProjectFieldMoves(t *testing.T) {
	for _, tc := range []struct {
		move FieldMove
		want gameruntime.CapabilityID
	}{
		{FieldCut, capCanCut},
		{FieldSurf, capCanSurf},
		{FieldStrength, capCanMoveBoulders},
	} {
		mem := fieldTestMem(tc.move, true, true, false)
		caps := redRouteCapabilities(nil, mem)
		if !caps.Has(tc.want) {
			t.Errorf("%s usable in Red RAM but semantic capability %q absent: %v", tc.move, tc.want, caps)
		}
	}
}

func TestRedRouteCapabilitiesProjectPokeFluteStoryFact(t *testing.T) {
	mem := new(state.Mem)
	mem[sym.NumBagItems] = 1
	mem[sym.BagItems] = 0x49
	mem[sym.BagItems+1] = 1
	caps := redRouteCapabilities(nil, mem)
	if !caps.Has(capCanClearSnorlax) {
		t.Fatalf("Poke Flute in Red inventory did not project %q: %v", capCanClearSnorlax, caps)
	}
}

func TestRedRouteCapabilitiesProjectSaffronGateOpen(t *testing.T) {
	mem := new(state.Mem)
	mem[sym.StatusFlags1] = 1 << 6 // BIT_GAVE_SAFFRON_GUARDS_DRINK
	caps := redRouteCapabilities(nil, mem)
	if !caps.Has(capCanEnterSaffron) {
		t.Fatalf("BIT_GAVE_SAFFRON_GUARDS_DRINK set but %q not projected: %v", capCanEnterSaffron, caps)
	}
}

func TestRedRouteTransitionsMapRepresentativeGates(t *testing.T) {
	for _, tc := range []struct {
		name string
		edge world.Edge
		want gameruntime.CapabilityID
	}{
		{"cut", world.Edge{Kind: world.EdgeWarp, From: semanticVermilionCityMap, To: vermilionGymMap}, capCanCut},
		{"surf", world.Edge{Kind: world.EdgeConnection, From: semanticRoute21Map, To: semanticCinnabarMap}, capCanSurf},
		{"story", world.Edge{Kind: world.EdgeConnection, From: route12Map, To: route13Map}, capCanClearSnorlax},
		{"strength", world.Edge{Kind: world.EdgeWarp, From: victoryRoad1FMap, To: victoryRoad2FMap}, capCanMoveBoulders},
		{"saffron border", world.Edge{Kind: world.EdgeConnection, From: semanticSaffronCityMap, To: semanticRoute5Map}, capCanEnterSaffron},
		{"saffron guardhouse", world.Edge{Kind: world.EdgeWarp, From: route5GateMap, To: semanticRoute5Map, WarpX: 3, WarpY: 5}, capCanEnterSaffron},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transition, ok := redRouteTransitionForEdge(tc.edge)
			if !ok {
				t.Fatalf("edge %+v has no semantic transition", tc.edge)
			}
			if len(transition.Requires) != 1 || transition.Requires[0] != tc.want {
				t.Fatalf("requires = %v, want [%s]", transition.Requires, tc.want)
			}
			if transition.From == "" || transition.To == "" {
				t.Fatalf("semantic locations not projected: %+v", transition)
			}
		})
	}
}
