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
	capCanBoardSSAnne             gameruntime.CapabilityID = "can_board_ss_anne"
	capCanLeaveViridianNorth      gameruntime.CapabilityID = "can_leave_viridian_north"
	capCanLeavePewterEast         gameruntime.CapabilityID = "can_leave_pewter_east"
	capCanEnterSaffron            gameruntime.CapabilityID = "can_enter_saffron"
)

const (
	semanticPalletTownMap    uint8 = 0x00
	semanticViridianCityMap  uint8 = 0x01
	semanticPewterCityMap    uint8 = 0x02
	semanticCeruleanCityMap  uint8 = 0x03
	semanticLavenderTownMap  uint8 = 0x04
	semanticVermilionCityMap uint8 = 0x05
	semanticCinnabarMap      uint8 = 0x08
	semanticSaffronCityMap   uint8 = 0x0A
	semanticRoute2Map        uint8 = 0x0D
	semanticRoute3Map        uint8 = 0x0E
	semanticRoute5Map        uint8 = 0x10
	semanticRoute6Map        uint8 = 0x11
	semanticRoute7Map        uint8 = 0x12
	semanticRoute8Map        uint8 = 0x13
	semanticRoute9Map        uint8 = 0x14
	semanticRoute11Map       uint8 = 0x16
	semanticRoute21Map       uint8 = 0x20

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
	route8GateMap uint8 = 0x4F

	ceruleanTrashedHouseMap        uint8 = 0x3e
	ceruleanTrashedHouseFrontWarpX uint8 = 27
	ceruleanTrashedHouseFrontWarpY uint8 = 11

	// Vermilion's harbor guard checks the S.S. Ticket immediately before
	// these two city -> dock warps. The reverse dock -> city edge remains
	// open, including after HM01 starts the departure sequence.
	semanticVermilionDockMap uint8 = 0x5E
	vermilionDockWarpY       uint8 = 31
	vermilionDockWarpX1      uint8 = 18
	vermilionDockWarpX2      uint8 = 19

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
	if facts.PokedexAcquired {
		caps[capCanLeaveViridianNorth] = true
	}
	if state.DecodeProgress(mem).Has(state.BadgeBoulder) {
		caps[capCanLeavePewterEast] = true
	}
	if facts.MtMoonFossilAcquired {
		caps[capCanExitMtMoon] = true
	}
	if facts.SSTicketAcquired {
		caps[capCanPassCeruleanRobbedHouse] = true
		// Receiving HM01 is the point of no return for the ship: the next
		// dock exit runs the departure script, after which Vermilion's guard
		// refuses harbor access. Do not let the immutable ROM graph advertise
		// stale S.S. Anne destinations once that progression is complete.
		if !facts.HM01Acquired {
			caps[capCanBoardSSAnne] = true
		}
	}
	if facts.PokeFluteAcquired {
		caps[capCanClearSnorlax] = true
	}
	if facts.SaffronGateOpen {
		caps[capCanEnterSaffron] = true
	}
	addAuditedRedRouteCapabilities(mem, caps)
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
	if transition, ok := redAuditedRouteTransitionForEdge(edge); ok {
		return transition, true
	}
	pair := func(a, b uint8) bool {
		return (edge.From == a && edge.To == b) || (edge.From == b && edge.To == a)
	}
	switch {
	case edge.From == semanticViridianCityMap && edge.To == semanticRoute2Map && edge.Kind == world.EdgeConnection:
		// Viridian's old man physically blocks the north road until Oak's
		// parcel/Pokedex story has completed. This must be an EDGE gate, not
		// only a "Route 2" destination filter: once a farther place becomes
		// known, routing to Pewter/Forest would otherwise plan straight through
		// the still-closed road.
		t := semanticTransition("red:viridian_north_pokedex", edge, capCanLeaveViridianNorth)
		t.Gate = true
		return t, true
	case edge.From == semanticPewterCityMap && edge.To == semanticRoute3Map && edge.Kind == world.EdgeConnection:
		// The Pewter east-exit NPC blocks Route 3 until Brock is beaten. Like
		// Viridian's old man, the lock belongs to the edge so every downstream
		// destination inherits it; filtering only the named Route 3 waypoint
		// is not enough once Mt. Moon/Cerulean are known.
		t := semanticTransition("red:pewter_east_boulder", edge, capCanLeavePewterEast)
		t.Gate = true
		return t, true
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
	case edge.From == semanticVermilionCityMap && edge.To == semanticVermilionDockMap &&
		edge.Kind == world.EdgeWarp && edge.WarpY == vermilionDockWarpY &&
		(edge.WarpX == vermilionDockWarpX1 || edge.WarpX == vermilionDockWarpX2):
		// The sailor one row north of the harbor warps checks S.S. Ticket and
		// pushes the player back when it is absent. Represent that scripted
		// guard as a gate on the forward warp so reachability can explain why
		// the ship is inaccessible instead of repeatedly walking into text.
		t := semanticTransition("red:ss_anne_ticket", edge, capCanBoardSSAnne)
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
	case edge.From == semanticVermilionCityMap && edge.To == vermilionGymMap:
		// Cut is only an entry prerequisite. Leaving the Gym must never be
		// blocked by the party losing/replacing its Cut carrier later.
		return semanticTransition("red:vermilion_gym_cut", edge, capCanCut), true
	case pair(semanticPalletTownMap, semanticRoute21Map),
		pair(semanticRoute21Map, semanticCinnabarMap):
		return semanticTransition("red:route21_surf", edge, capCanSurf), true
	case edge.To == route12Map &&
		(edge.From == semanticRoute11Map || edge.From == semanticLavenderTownMap):
		// Route 12's Snorlax is a live object at (10,62), inside the map rather
		// than on either connection. The immutable graph therefore sees a fake
		// Route 11 -> Route 12 -> Lavender shortcut before the Poké Flute and
		// can strand Red beside the sleeper. Treat entering this corridor from
		// either early-game side as a precondition gate. The reverse edges stay
		// open so a legacy/pre-fix checkpoint already on Route 12 can escape.
		t := semanticTransition("red:route12_snorlax_access", edge, capCanClearSnorlax)
		t.Gate = true
		return t, true
	case pair(route12Map, route13Map):
		// Once the Poké Flute exists this action owns the actual wake/battle
		// before the Fuchsia route continues south.
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
