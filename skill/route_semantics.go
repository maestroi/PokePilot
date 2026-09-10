package skill

import (
	"strings"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
)

const (
	capCanCut                     gameruntime.CapabilityID = "can_cut"
	capCanSurf                    gameruntime.CapabilityID = "can_surf"
	capCanMoveBoulders            gameruntime.CapabilityID = "can_move_boulders"
	capCanClearSnorlax            gameruntime.CapabilityID = "can_clear_snorlax"
	capCanExitMtMoon              gameruntime.CapabilityID = "can_exit_mt_moon"
	capCanPassCeruleanRobbedHouse gameruntime.CapabilityID = "can_pass_cerulean_robbed_house"
	capCanEnterSaffron            gameruntime.CapabilityID = "can_enter_saffron"
)

const (
	semanticPalletTownMap    uint8 = 0x00
	semanticCeruleanCityMap  uint8 = 0x03
	semanticVermilionCityMap uint8 = 0x05
	semanticCinnabarMap      uint8 = 0x08
	semanticRoute9Map        uint8 = 0x14
	semanticRoute21Map       uint8 = 0x20
	semanticSaffronCityMap   uint8 = 0x0A
	semanticRoute5Map        uint8 = 0x10
	semanticRoute6Map        uint8 = 0x11
	semanticRoute7Map        uint8 = 0x12
	semanticRoute8Map        uint8 = 0x13

	// The four guardhouses ringing Saffron (map type GATE) each block
	// passage until BIT_GAVE_SAFFRON_GUARDS_DRINK is set; giving any one
	// guard a drink sets the flag and opens all four permanently
	// (pokered/scripts/Route5Gate.asm and its Route6/7/8 siblings). Each
	// guardhouse's own warps all resolve back to its route (its object data
	// has no warp into Saffron; Saffron's own warp list has no warp into any
	// guardhouse either), so the guardhouse is a self-contained room that
	// bridges two ends of the SAME route map, and the guard blocks crossing
	// it on foot-collision coordinates the static block map does not
	// encode. The real border crossing into Saffron is the plain map
	// connection declared on both headers.
	route5GateMap uint8 = 0x46
	route6GateMap uint8 = 0x49
	route7GateMap uint8 = 0x4C
	route8GateMap uint8 = 0x4F

	ceruleanTrashedHouseMap        uint8 = 0x3e
	ceruleanTrashedHouseFrontWarpX uint8 = 27
	ceruleanTrashedHouseFrontWarpY uint8 = 11

	// The ladder out of Mt. Moon B2F's fossil corridor
	// (pokered/data/maps/objects/MtMoonB2F.asm: warp_event 5, 7).
	mtMoonB2FMap       uint8 = 0x3D
	mtMoonB2FExitWarpX uint8 = 5
	mtMoonB2FExitWarpY uint8 = 7
)

// redRouteCapabilities is the Red adapter projection from RAM/party mechanics
// into the portable route vocabulary. A field move counts only when Travel can
// use it now, including the existing safe auto-teach path. Story/item encoding
// stays entirely on this side of the boundary.
func redRouteCapabilities(romData []byte, mem *state.Mem) gameruntime.CapabilitySet {
	caps := gameruntime.NewCapabilitySet()
	field := []struct {
		move FieldMove
		id   gameruntime.CapabilityID
	}{
		{FieldCut, capCanCut},
		{FieldSurf, capCanSurf},
		{FieldStrength, capCanMoveBoulders},
	}
	for _, entry := range field {
		capability := FieldCapabilityFor(mem, entry.move)
		if capability.Usable || CanPrepareFieldMove(romData, mem, entry.move) {
			caps[entry.id] = true
		}
	}

	inv := state.DecodeInventory(mem)
	facts := state.DecodeStoryFacts(mem, inv)
	if facts.MtMoonFossilAcquired {
		caps[capCanExitMtMoon] = true
	}
	if facts.SSTicketAcquired {
		caps[capCanPassCeruleanRobbedHouse] = true
	}
	if facts.PokeFluteAcquired {
		caps[capCanClearSnorlax] = true
	}
	if facts.SaffronGateOpen {
		caps[capCanEnterSaffron] = true
	}
	return caps
}

func semanticMapPlace(mapID uint8) gameruntime.PlaceID {
	return gameruntime.CanonicalID(strings.ReplaceAll(state.MapName(mapID), "_", " "))
}

func semanticTransition(id string, edge world.Edge, requires ...gameruntime.CapabilityID) gameruntime.Transition {
	return gameruntime.Transition{
		ID:       id,
		From:     semanticMapPlace(edge.From),
		To:       semanticMapPlace(edge.To),
		Requires: requires,
	}
}

// redRouteTransitionForEdge maps representative existing Red gates onto the
// portable transition model. The router never sees these map ids; they are
// adapter facts attached to ordinary geometric edges.
func redRouteTransitionForEdge(edge world.Edge) (gameruntime.Transition, bool) {
	pair := func(a, b uint8) bool {
		return (edge.From == a && edge.To == b) || (edge.From == b && edge.To == a)
	}
	switch {
	case edge.From == mtMoonB2FMap && edge.Kind == world.EdgeWarp &&
		edge.WarpX == mtMoonB2FExitWarpX && edge.WarpY == mtMoonB2FExitWarpY:
		// A gate, not an action: nothing is performed to open the fossil
		// corridor, its Super Nerd simply stops standing in it. B2F's rooms
		// are disconnected from one another, so treating this ladder as a
		// pivot once the story flag is set would route between rooms that
		// share nothing but a floor.
		t := semanticTransition("red:mt_moon_exit", edge, capCanExitMtMoon)
		t.Gate = true
		return t, true
	case edge.From == semanticCeruleanCityMap && edge.To == ceruleanTrashedHouseMap &&
		edge.Kind == world.EdgeWarp && edge.WarpX == ceruleanTrashedHouseFrontWarpX &&
		edge.WarpY == ceruleanTrashedHouseFrontWarpY:
		// Before Bill gives the S.S. Ticket the guard occupies the approach
		// tile in front of the robbed house. Bill's script moves that guard;
		// the house's rear hole then changes walkable component and opens the
		// real road south to Route 5. This is a precondition, not an action.
		t := semanticTransition("red:cerulean_robbed_house", edge, capCanPassCeruleanRobbedHouse)
		t.Gate = true
		return t, true
	case pair(semanticCeruleanCityMap, semanticRoute9Map):
		// The east connection itself is open, but the named Route 9
		// destination lies beyond the Cut tree. Mark the connection as a gate
		// so reachability reports Cut up front; Travel's existing live-tree
		// recovery performs the actual Cut after entering Route 9.
		t := semanticTransition("red:route9_cut", edge, capCanCut)
		t.Gate = true
		return t, true
	case pair(semanticSaffronCityMap, semanticRoute5Map),
		pair(semanticSaffronCityMap, semanticRoute6Map),
		pair(semanticSaffronCityMap, semanticRoute7Map),
		pair(semanticSaffronCityMap, semanticRoute8Map),
		pair(route5GateMap, semanticRoute5Map),
		pair(route6GateMap, semanticRoute6Map),
		pair(route7GateMap, semanticRoute7Map),
		pair(route8GateMap, semanticRoute8Map):
		// Both the route's plain border connection into Saffron and its
		// guardhouse's interior floor are ordinary geometry; the guard
		// standing in the doorway is the precondition, not the router's
		// business to route around by picking a longer real edge. Without
		// the drink flag this must fail closed as a missing capability, not
		// walk the agent up to the guard's dialogue to discover it live.
		t := semanticTransition("red:saffron_guard_drink", edge, capCanEnterSaffron)
		t.Gate = true
		return t, true
	case pair(semanticVermilionCityMap, vermilionGymMap):
		return semanticTransition("red:vermilion_gym_cut", edge, capCanCut), true
	case pair(semanticPalletTownMap, semanticRoute21Map),
		pair(semanticRoute21Map, semanticCinnabarMap):
		return semanticTransition("red:route21_surf", edge, capCanSurf), true
	case pair(route12Map, route13Map):
		return semanticTransition("red:route12_snorlax", edge, capCanClearSnorlax), true
	case pair(victoryRoad1FMap, victoryRoad2FMap),
		pair(victoryRoad2FMap, victoryRoad3FMap):
		return semanticTransition("red:victory_road_strength", edge, capCanMoveBoulders), true
	default:
		return gameruntime.Transition{}, false
	}
}

// redRoutePrerequisites attaches adapter-owned transition facts to the concrete
// graph while keeping the routing algorithm generic.
func redRoutePrerequisites(g *world.Graph, romData []byte, mem *state.Mem) world.RoutePrerequisites {
	transitions := make(map[world.Edge]gameruntime.Transition)
	for _, edges := range g.Edges {
		for _, edge := range edges {
			if transition, ok := redRouteTransitionForEdge(edge); ok {
				transitions[edge] = transition
			}
		}
	}
	return world.RoutePrerequisites{
		Transitions:  transitions,
		Capabilities: redRouteCapabilities(romData, mem),
	}
}
