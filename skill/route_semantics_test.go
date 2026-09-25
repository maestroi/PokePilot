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
		{"route12 entry", world.Edge{Kind: world.EdgeConnection, From: semanticRoute11Map, To: route12Map}, capCanClearSnorlax},
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

// TestRedRouteTransitionGatesRocketB1FDoor is the regression for the
// stuck "go to celadon city" farm runs whose elevator ride into Rocket
// Hideout B1F lands beside the door RocketHideoutB1FDoorCallbackScript keeps
// locked until Rocket5 ((28,18)) is beaten: GoTo's plain FindPath saw the
// live-collision wall, banned both exits as unwalkable, and the only
// remaining move was back into the elevator — an endless cb<->c7 bounce
// (measured on run-xwsj1xbe1ftyt4v1493l1u1l). Both of B1F's exits behind that
// door (the Game Corner warp and the B2F stair warp) must carry the action
// transition with no capability requirement, since nothing but reaching the
// grunt gates the fight; B1F's OTHER, ungated B2F warp (the south one at
// (21,24)) and the elevator edges must not.
func TestRedRouteTransitionGatesRocketB1FDoor(t *testing.T) {
	for _, tc := range []struct {
		name string
		edge world.Edge
		want bool
	}{
		{"game corner exit", world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB1FMap, To: gameCornerMap, WarpX: rocketB1FGameCornerWarpX, WarpY: rocketB1FGameCornerWarpY}, true},
		{"b2f stair exit", world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB1FMap, To: rocketHideoutB2FMap, WarpX: rocketB1FStairsWarpX, WarpY: rocketB1FStairsWarpY}, true},
		{"b2f south warp, ungated", world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB1FMap, To: rocketHideoutB2FMap, WarpX: 21, WarpY: 24}, false},
		{"elevator entry", world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB1FMap, To: rocketHideoutElevatorMap, WarpX: 24, WarpY: 19}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transition, ok := redRouteTransitionForEdge(tc.edge)
			if ok != tc.want {
				t.Fatalf("redRouteTransitionForEdge(%+v) ok=%v, want %v (transition=%+v)", tc.edge, ok, tc.want, transition)
			}
			if !tc.want {
				return
			}
			if transition.ID != "red:rocket_b1f_trainer_door" {
				t.Fatalf("transition ID = %q, want red:rocket_b1f_trainer_door", transition.ID)
			}
			if transition.Gate {
				t.Fatalf("transition is a Gate; want an action (nothing precedes fighting Rocket5)")
			}
			if len(transition.Requires) != 0 {
				t.Fatalf("Requires = %v, want none: fighting Rocket5 needs no prior item/badge", transition.Requires)
			}
		})
	}
}
