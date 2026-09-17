package skill

import (
	"os"
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
		caps := redRouteCapabilities(nil, mem, redWram())
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
	caps := redRouteCapabilities(nil, mem, redWram())
	if !caps.Has(capCanClearSnorlax) {
		t.Fatalf("Poke Flute in Red inventory did not project %q: %v", capCanClearSnorlax, caps)
	}
}

func TestRedRouteCapabilitiesProjectSaffronGateOpen(t *testing.T) {
	mem := new(state.Mem)
	mem[sym.StatusFlags1] = 1 << 6 // BIT_GAVE_SAFFRON_GUARDS_DRINK
	caps := redRouteCapabilities(nil, mem, redWram())
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

// TestRoute12SnorlaxTransitionOnlyOwnsTheWalkableBand is the regression for
// MEASURED run-h7ow811287kpyo0ekyn8f32b round 3 (progress saffron_gate_open):
// Route 12 <-> Route 13's border is split into several connectionEdges
// component-paired bands, most of which are non-walkable border padding
// retained only so *some* edge of the map pair can carry a semantic
// transition (see ConnectionExitWalkable's doc comment). Because
// redRouteTransitionForEdge matches on map pair alone, "red:route12_snorlax"
// used to land on every one of those bands, including the padding ones. Once
// Snorlax was clearable, findRoute's semantic bypass let the router pick a
// padding band exactly as readily as the real crossing, producing "no
// reachable walkable tile" and exhausting the re-plan budget instead of
// reaching Route 12. redRoutePrerequisites must keep the transition only on
// the band that is actually walkable ground.
func TestRoute12SnorlaxTransitionOnlyOwnsTheWalkableBand(t *testing.T) {
	if testing.Short() {
		t.Skip("ROM-backed")
	}
	romPath := os.Getenv("POKEMON_RED_ROM")
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	graph, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	var borderEdges []world.Edge
	for _, e := range graph.Edges[route13Map] {
		if e.Kind == world.EdgeConnection && e.To == route12Map {
			borderEdges = append(borderEdges, e)
		}
	}
	if len(borderEdges) < 2 {
		t.Fatalf("Route 13 -> Route 12 has %d connection edge(s), want multiple component-scoped bands", len(borderEdges))
	}
	var walkable, unwalkable int
	for _, e := range borderEdges {
		if graph.ConnectionExitWalkable(e) {
			walkable++
		} else {
			unwalkable++
		}
	}
	if walkable == 0 || unwalkable == 0 {
		t.Fatalf("expected a mix of walkable and non-walkable bands, got walkable=%d unwalkable=%d", walkable, unwalkable)
	}

	// Poke Flute ownership alone grants capCanClearSnorlax (see
	// TestRedRouteCapabilitiesProjectPokeFluteStoryFact), which is what let the
	// buggy version of this code attach the transition to every band.
	mem := new(state.Mem)
	mem[sym.NumBagItems] = 1
	mem[sym.BagItems] = 0x49
	mem[sym.BagItems+1] = 1

	prereqs := redRoutePrerequisites(graph, romData, mem, redWram())
	for _, e := range borderEdges {
		transition, attached := prereqs.Transitions[e]
		if !graph.ConnectionExitWalkable(e) {
			if attached {
				t.Fatalf("non-walkable band %+v got %q attached; must stay unowned so canExit rejects it", e, transition.ID)
			}
			continue
		}
		if !attached || transition.ID != "red:route12_snorlax" {
			t.Fatalf("walkable band %+v should own red:route12_snorlax, got attached=%v transition=%+v", e, attached, transition)
		}
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
