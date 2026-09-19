package skill

import (
	"strings"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
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
	//
	// Only the Saffron-facing half of each guardhouse is a route gate. The
	// outside-facing half must stay routable while the drink flag is absent:
	// OpenSaffronGate deliberately enters Route 7's west half, walks to the
	// trigger at x=3, and gives the guard the drink from there. Treating every
	// route<->guardhouse warp as gated makes the prerequisite impossible to
	// satisfy because the skill cannot even enter the room that owns it.
	route5GateMap uint8 = 0x46
	route6GateMap uint8 = 0x49
	route8GateMap uint8 = 0x4F

	route5SaffronWarpY     uint8 = 33
	route5GateSaffronWarpY uint8 = 5
	route6SaffronWarpY     uint8 = 1
	route6GateSaffronWarpY uint8 = 0
	route7SaffronWarpX     uint8 = 18
	route7GateSaffronWarpX uint8 = 5
	route8SaffronWarpX     uint8 = 1
	route8GateSaffronWarpX uint8 = 0

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
	t := gameruntime.Transition{
		ID:       id,
		From:     semanticMapPlace(edge.From),
		To:       semanticMapPlace(edge.To),
		Requires: requires,
	}
	for _, requirement := range requires {
		if requirement == capCanSurf {
			t.PortBypass = true
			break
		}
	}
	return t
}

// redRouteTransitionForEdge maps representative existing Red gates onto the
// portable transition model. The router never sees these map ids; they are
// adapter facts attached to ordinary geometric edges.
func saffronGuardhouseCrossingEdge(edge world.Edge) bool {
	if edge.Kind != world.EdgeWarp {
		return false
	}
	switch {
	case edge.From == semanticRoute5Map && edge.To == route5GateMap:
		return edge.WarpY == route5SaffronWarpY
	case edge.From == route5GateMap && edge.To == semanticRoute5Map:
		return edge.WarpY == route5GateSaffronWarpY
	case edge.From == semanticRoute6Map && edge.To == route6GateMap:
		return edge.WarpY == route6SaffronWarpY
	case edge.From == route6GateMap && edge.To == semanticRoute6Map:
		return edge.WarpY == route6GateSaffronWarpY
	case edge.From == semanticRoute7Map && edge.To == route7GateMap:
		return edge.WarpX == route7SaffronWarpX
	case edge.From == route7GateMap && edge.To == semanticRoute7Map:
		return edge.WarpX == route7GateSaffronWarpX
	case edge.From == semanticRoute8Map && edge.To == route8GateMap:
		return edge.WarpX == route8SaffronWarpX
	case edge.From == route8GateMap && edge.To == semanticRoute8Map:
		return edge.WarpX == route8GateSaffronWarpX
	default:
		return false
	}
}

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
		// The immutable ROM collision splits Route 9 into a Cerulean-side
		// component and a Route 10-side component, joined only by the live Cut
		// tree. Once can_cut is available this semantic action must act as a
		// pivot so routing can cross that static split. But the connection
		// itself is not the tree: a player already on the Cerulean-side
		// component can leave Route 9 again without Cut. Issue #815 exposed the
		// old all-or-nothing behavior by stranding a checkpoint at Route 9
		// (0,0) while recovery tried to return to Route 4.
		t := semanticTransition("red:route9_cut", edge, capCanCut)
		t.PivotOnly = true
		return t, true
	case pair(semanticSaffronCityMap, semanticRoute5Map),
		pair(semanticSaffronCityMap, semanticRoute6Map),
		pair(semanticSaffronCityMap, semanticRoute7Map),
		pair(semanticSaffronCityMap, semanticRoute8Map),
		saffronGuardhouseCrossingEdge(edge):
		// The route's plain border connection into Saffron and only the
		// Saffron-facing guardhouse warps are gated by the drink. The outside
		// guardhouse entrance stays available so the progression skill can enter
		// the room and trigger the guard script that satisfies this prerequisite.
		t := semanticTransition("red:saffron_guard_drink", edge, capCanEnterSaffron)
		t.Gate = true
		return t, true
	case pair(semanticVermilionCityMap, vermilionGymMap):
		// The same Cut tree sits on both sides of this warp in the immutable
		// ROM collision: entering blocks on it exactly as leaving does, since
		// the static graph never sees the tree as cut (that state lives only
		// in the currently-loaded map's live WRAM, not in the precomputed
		// components used for the far side of a multi-map route). Treating
		// only city->gym as the pivot left comp3 (yard around (12,19)) and
		// comp1 (the street) provably disconnected when leaving, so
		// PostSurgeCeladonProgression died with "world: no route" right after
		// Surge on every run (run-3djisxgsy3dgzpnsde2inzyuh round 7).
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
	case edge.From == rocketHideoutB1FMap && edge.Kind == world.EdgeWarp &&
		((edge.To == gameCornerMap && edge.WarpX == rocketB1FGameCornerWarpX && edge.WarpY == rocketB1FGameCornerWarpY) ||
			(edge.To == rocketHideoutB2FMap && edge.WarpX == rocketB1FStairsWarpX && edge.WarpY == rocketB1FStairsWarpY)):
		// RocketHideoutB1FDoorCallbackScript (pokered
		// scripts/RocketHideoutB1F.asm) replaces the block at (24,16)/(25,16)
		// between a Door block and a Floor block, keyed on
		// EVENT_BEAT_ROCKET_HIDEOUT_1_TRAINER_4. The elevator lands on the
		// SAME side of that door as the guarding grunt (Rocket5, (28,18)):
		// MEASURED, the live collision grid marks those two tiles solid
		// before the fight and walkable immediately after, with no map
		// reload needed. No item or badge gates the fight itself, so this
		// action has no Requires — it is always performable from here, same
		// as the B4F guards' fightStoryTrainerAt.
		return semanticTransition("red:rocket_b1f_trainer_door", edge), true
	default:
		return gameruntime.Transition{}, false
	}
}

// redRouteTransitionEffectComplete reports that a semantic action's durable
// postcondition already holds, so the edge must use ordinary walking geometry
// instead of pivot privilege. A satisfied Snorlax clear still left
// red:route12_snorlax attached; FindRoute then waived reachability to Route 13's
// north port and offered an unwalkable first hop from the west-side trainer
// pocket (run-4h4isxsvaskt1c7mslvxzsrr6). Gates and PortBypass seams stay
// annotated: they are still the portable description of the edge.
func redRouteTransitionEffectComplete(mem *state.Mem, transition gameruntime.Transition) bool {
	if transition.Gate || transition.PortBypass {
		return false
	}
	switch transition.ID {
	case "red:route12_snorlax":
		return state.HasEvent(mem, eventBeatRoute12Snorlax)
	case "red:route16_snorlax":
		return state.HasEvent(mem, eventBeatRoute16Snorlax)
	default:
		return false
	}
}

// redRoutePrerequisites attaches adapter-owned transition facts to the concrete
// graph while keeping the routing algorithm generic.
func redRoutePrerequisites(g *world.Graph, romData []byte, mem *state.Mem) world.RoutePrerequisites {
	transitions := make(map[world.Edge]gameruntime.Transition)
	for _, edges := range g.Edges {
		for _, edge := range edges {
			if transition, ok := redRouteTransitionForEdge(edge); ok {
				// Component-scoped ROM connections retain in-bounds padding bands
				// so actions that truly create a seam (Surf) can own them. Every
				// other semantic action must still use a physically real border
				// port; otherwise an interior Cut/Snorlax/switch action can turn
				// solid padding into an executable map transition.
				if edge.Kind == world.EdgeConnection && !transition.PortBypass && !g.ConnectionExitWalkable(edge) {
					continue
				}
				if redRouteTransitionEffectComplete(mem, transition) {
					continue
				}
				transitions[edge] = transition
			}
		}
	}
	return world.RoutePrerequisites{
		Transitions:  transitions,
		Capabilities: redRouteCapabilities(romData, mem),
	}
}

// ReachableMaps reports which native map IDs GoTo could actually route the
// player to right now, applying the same capability gating GoTo enforces
// during travel (Cut, Surf, Snorlax, badges, story flags, ...). Planners that
// pick a destination without this check can offer a target GoTo will then
// refuse outright (e.g. a Route 12 catch habitat behind an uncleared
// Snorlax), burning a full round on an objective that can never complete.
//
// A nil emu or empty ROM returns (nil, nil): "unknown" rather than "nothing
// reachable", so callers that cannot supply live state keep their prior,
// capability-blind behavior instead of suppressing everything.
func ReachableMaps(m *emu.Emu, romData []byte) (map[uint8]bool, error) {
	if m == nil || len(romData) == 0 {
		return nil, nil
	}
	g, err := world.BuildGraph(romData)
	if err != nil {
		return nil, err
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	prereqs := redRoutePrerequisites(g, romData, &mem)
	cur := mem.U8(sym.CurMap)
	x, y := mem.U8(sym.XCoord), mem.U8(sym.YCoord)

	candidates := map[uint8]bool{cur: true}
	for from, edges := range g.Edges {
		candidates[from] = true
		for _, edge := range edges {
			candidates[edge.To] = true
		}
	}

	reachable := map[uint8]bool{cur: true}
	for mapID := range candidates {
		if reachable[mapID] {
			continue
		}
		if _, err := world.FindRoutePlanAtDestinationWithCapabilities(
			g, cur, mapID, int(x), int(y), -1, -1, nil, prereqs,
		); err == nil {
			reachable[mapID] = true
		}
	}
	return reachable, nil
}
