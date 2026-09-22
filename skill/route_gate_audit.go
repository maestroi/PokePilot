package skill

import (
	"fmt"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const (
	capCanRideCyclingRoad        gameruntime.CapabilityID = "can_ride_cycling_road"
	capCanPassRoute23BadgeChecks gameruntime.CapabilityID = "can_pass_route23_badge_checks"
	capCanPassLanceExit          gameruntime.CapabilityID = "can_pass_lance_exit"

	// Red's Celadon City object table contains a historical/unused warp at
	// (39,19) directly to the department store 5F. The decomp explicitly marks
	// it "inaccessible": there is no door there in the playable map. Keep a
	// never-projected capability for that one phantom edge so semantic routing
	// cannot use the wall as a shortcut while the raw ROM warp table remains
	// intact for destination-warp indexing.
	capCanUseInaccessibleWarp gameruntime.CapabilityID = "can_use_inaccessible_warp"

	// The Game Corner poster stair is a real warp_event, but the map script
	// replaces its block with a wall until EVENT_FOUND_ROCKET_HIDEOUT.
	capCanEnterRocketHideout gameruntime.CapabilityID = "can_enter_rocket_hideout"

	bicycleItem uint8 = 0x06

	route16Map uint8 = 0x1B
	// Snorlax's Route 16 home tile from the ROM object table (probe: sprite 67
	// at (26,10)). He blocks the lower road only; the upper passage Cut tree
	// at (34,9) joins the Fly-house side to the Celadon edge east of him.
	route16SnorlaxX        = 26
	route16SnorlaxY        = 10
	// The Cut tree on the upper passage. Solid in the static grid; walkable
	// while the player can Cut, it joins the Fly-house side to the east road.
	route16CutTreeX = 34
	route16CutTreeY = 9
	route17Map       uint8 = 0x1C
	route18Map       uint8 = 0x1D
	route19Map       uint8 = 0x1E
	route20Map       uint8 = 0x1F
	route16Gate1FMap uint8 = 0xBA
	route18Gate1FMap uint8 = 0xBE

	celadonMart5FMap             uint8 = 0x88
	celadonInaccessibleMartWarpX uint8 = 39
	celadonInaccessibleMartWarpY uint8 = 19

	silphCo1FInaccessibleStairWarpX uint8 = 16
	silphCo1FInaccessibleStairWarpY uint8 = 10

	route23VictoryRoadWarpX     uint8 = 4
	route23VictoryRoadWarpY     uint8 = 31
	route23VictoryRoadApproachY uint8 = 32

	eventFightRoute16Snorlax state.Event = 0x4C8
	eventBeatRoute16Snorlax  state.Event = 0x4C9
	eventBeatLance           state.Event = 0x8FE
)

func addAuditedRedRouteCapabilities(mem *state.Mem, caps gameruntime.CapabilitySet) {
	if _, count := bagEntry(mem, bicycleItem); count > 0 {
		caps[capCanRideCyclingRoad] = true
	}
	if state.DecodeProgress(mem).BadgeCount == 8 {
		caps[capCanPassRoute23BadgeChecks] = true
	}
	if state.HasEvent(mem, eventBeatLance) {
		caps[capCanPassLanceExit] = true
	}
	// A completed Snorlax encounter is durable proof that this save already
	// acquired the Poké Flute. Resume/checkpoint reconstruction can lose the
	// derived inventory story fact while retaining event flags; without this
	// recovery the semantic router rejects Route 12 <-> Route 13, then tries
	// the unrelated Cycling Road escape and reports a missing Bicycle.
	if state.HasEvent(mem, eventBeatRoute12Snorlax) || state.HasEvent(mem, eventBeatRoute16Snorlax) {
		caps[capCanClearSnorlax] = true
	}
}

// withAsleepRoute16Snorlax splits Route 16's lower road on Snorlax's home
// tile while EVENT_BEAT_ROUTE16_SNORLAX is clear. The static collision grid
// treats that tile as walkable, so without this split the west component
// canExit the Celadon connection straight through him. The upper passage and
// the east-of-Snorlax road stay separate components; Cut joins those.
//
// The returned graph is a snapshot. The cached ROM graph is not mutated.
func withAsleepRoute16Snorlax(g *world.Graph, romData []byte, mem *state.Mem) (*world.Graph, error) {
	if g == nil || mem == nil || state.HasEvent(mem, eventBeatRoute16Snorlax) {
		return g, nil
	}
	h, err := rom.ParseMap(romData, route16Map)
	if err != nil {
		return nil, err
	}
	grid, err := world.Build(romData, h)
	if err != nil {
		return nil, err
	}
	grid.Set(route16SnorlaxX, route16SnorlaxY, false)
	openRoute16CutPassage(grid, romData, mem)
	return g.WithMapGrid(route16Map, grid)
}

// openRoute16CutPassage opens the Cut tree at (34,9) in a Route 16 grid while
// the player can Cut. The static collision grid keeps the tree solid, so the
// graph only sees the upper-passage-to-east-road passage once the capability
// is held; without it the two halves stay separate components.
func openRoute16CutPassage(grid *world.Grid, romData []byte, mem *state.Mem) {
	if redRouteCapabilities(romData, mem).Has(capCanCut) {
		grid.Set(route16CutTreeX, route16CutTreeY, true)
	}
}

func redAuditedRouteTransitionForEdge(edge world.Edge) (gameruntime.Transition, bool) {
	if transition, ok := redSideRouteTransitionForEdge(edge); ok {
		return transition, true
	}
	pair := func(a, b uint8) bool {
		return (edge.From == a && edge.To == b) || (edge.From == b && edge.To == a)
	}
	bikeGate := func(id string, requires ...gameruntime.CapabilityID) (gameruntime.Transition, bool) {
		t := semanticTransition(id, edge, requires...)
		t.Gate = true
		return t, true
	}

	switch {
	case edge.Kind == world.EdgeWarp && edge.From == route23Map && edge.To == victoryRoad1FMap &&
		edge.WarpX == route23VictoryRoadWarpX && edge.WarpY == route23VictoryRoadWarpY:
		// Route 23 is not one immutable walking component. The League approach
		// crosses three full-width Surf bands and seven scripted badge guards
		// before the south Victory Road entrance can be reached. The runtime
		// already owns that measured traversal in VictoryRoadProgression; expose
		// the same fact to semantic routing so the portable graph does not stop
		// at the south Route 23 component even with all progression complete.
		return semanticTransition("red:route23_league_approach", edge, capCanSurf, capCanPassRoute23BadgeChecks), true

	case edge.Kind == world.EdgeWarp && edge.From == lanceRoomMap && edge.To == championsRoomMap &&
		edge.WarpX == lanceExitStand.X && edge.WarpY == 0:
		// Lance's north exit is a scripted progression boundary. The generic
		// immutable collision graph cannot prove the post-battle approach to the
		// Champion warp, but Elite Four progression already owns that exact
		// crossing after EVENT_BEAT_LANCE. Model the durable battle result as a
		// capability and let this semantic action own the warp port itself.
		t := semanticTransition("red:lance_to_champion", edge, capCanPassLanceExit)
		t.PortBypass = true
		return t, true

	case edge.Kind == world.EdgeWarp && edge.From == celadonCityMap && edge.To == celadonMart5FMap &&
		edge.WarpX == celadonInaccessibleMartWarpX && edge.WarpY == celadonInaccessibleMartWarpY:
		// pokered/data/maps/objects/CeladonCity.asm declares this warp but
		// annotates it "; inaccessible". Static collision leaves a walkable
		// component beside the coordinate, so the generic graph can mistake the
		// wall for a solid stair and route through it. A permanent semantic gate
		// removes only this source edge while preserving the real way to 5F via
		// the department-store entrance, stairs/elevator, and all raw warp ids.
		return bikeGate("red:celadon_inaccessible_mart_warp", capCanUseInaccessibleWarp)

	case edge.Kind == world.EdgeWarp && edge.From == silphCo1FMap && edge.To == silphCo3FMap &&
		edge.WarpX == silphCo1FInaccessibleStairWarpX && edge.WarpY == silphCo1FInaccessibleStairWarpY:
		// pokered/data/maps/objects/SilphCo1F.asm declares this warp but
		// annotates it "; inaccessible", the same leftover-ROM-data pattern as
		// the Celadon Mart 5F warp above. Static collision leaves a walkable
		// room beside the coordinate, so the generic graph mistakes it for a
		// working stairway to 3F and Traverse spends its whole budget bouncing
		// off a warp the real game never lets fire. Gate only this source
		// edge; the real way to 3F stays open via the elevator and 2F stairs.
		return bikeGate("red:silph_co_1f_inaccessible_stair_warp", capCanUseInaccessibleWarp)

	case edge.Kind == world.EdgeWarp && edge.From == route16Map && edge.To == route16Gate1FMap &&
		edge.WarpX == 24 && (edge.WarpY == 10 || edge.WarpY == 11):
		// The east entrance to Route 16's lower gate is reached from Celadon.
		// Snorlax sits immediately east of the gate, and the guard inside the
		// lower corridor separately requires a Bicycle. Both are preconditions
		// on ordinary geometry: the warp still lands in the lower corridor
		// only. Modeling this as an action pivot used to relax that landing
		// and let routing pretend the lower door reaches the upper
		// pedestrian/Fly-house exits through a wall (farm triage
		// 0ece8bd130597547 / run-1g7kwah5oygzl29jkvhmhsmby6). Keep it a Gate
		// so component reachability stays honest; execution still clears
		// Snorlax when the annotated edge is taken.
		return bikeGate("red:route16_snorlax_bicycle", capCanClearSnorlax, capCanRideCyclingRoad)

	case edge.Kind == world.EdgeWarp && edge.From == route16Map && edge.To == route16Gate1FMap &&
		edge.WarpX == 17 && (edge.WarpY == 10 || edge.WarpY == 11):
		// West-side entry is already past Snorlax; only the Cycling Road guard
		// applies. Do not gate Route 16's separate upper pedestrian/Fly-house
		// passage at y=4/5.
		return bikeGate("red:cycling_road_bicycle", capCanRideCyclingRoad)

	case edge.Kind == world.EdgeWarp && edge.From == route16Gate1FMap && edge.To == route16Map &&
		(edge.WarpX == 0 || edge.WarpX == 7) && (edge.WarpY == 8 || edge.WarpY == 9):
		return bikeGate("red:cycling_road_bicycle", capCanRideCyclingRoad)

	case edge.Kind == world.EdgeWarp && edge.From == route18Map && edge.To == route18Gate1FMap &&
		(edge.WarpX == 33 || edge.WarpX == 40) && (edge.WarpY == 8 || edge.WarpY == 9):
		return bikeGate("red:cycling_road_bicycle", capCanRideCyclingRoad)

	case edge.Kind == world.EdgeWarp && edge.From == route18Gate1FMap && edge.To == route18Map &&
		(edge.WarpX == 0 || edge.WarpX == 7) && (edge.WarpY == 4 || edge.WarpY == 5):
		return bikeGate("red:cycling_road_bicycle", capCanRideCyclingRoad)

	case edge.Kind == world.EdgeConnection && edge.From == route16Map && edge.To == celadonCityMap:
		// Snorlax sleeps on the lower road at (26,10). The Celadon connection's
		// east component is already past him, and Route 16's upper passage
		// reaches that component by Cut at (34,9) — measured from the upper
		// gate landing (24,5) to the east edge (39,10) in one Cut while (26,10)
		// is blocked. A hard deny on this whole connection made that side
		// report can_clear_snorlax anyway, so a Fly-house save with Cut and no
		// Poké Flute was unroutable to Celadon (run-1c4k0nk8lwwcc2hr8dhy65m5o0).
		//
		// PivotOnly keeps the edge on ordinary geometry when the flute is
		// missing, so the east component still walks into Celadon. PortBypass
		// lets a west-of-Snorlax tile that cannot reach the port select this
		// action once the flute is held; the executor then wakes him. The
		// asleep sprite must also split the lower road (withAsleepRoute16Snorlax)
		// or the west component canExit through his tile and walks into him.
		t := semanticTransition("red:route16_snorlax", edge, capCanClearSnorlax)
		t.PivotOnly = true
		t.PortBypass = true
		return t, true

	case pair(route19Map, route20Map), pair(route20Map, cinnabarIslandMap):
		// The southern sea route is every bit as Surf-gated as Route 21. Keep
		// Route 19 itself walkable from Fuchsia; Surf starts at the Route 19/20
		// seam and remains required through the Cinnabar connection.
		return semanticTransition("red:southern_sea_surf", edge, capCanSurf), true

	case edge.Kind == world.EdgeWarp && edge.From == gameCornerMap && edge.To == rocketHideoutB1FMap &&
		edge.WarpX == gameCornerWarpX && edge.WarpY == gameCornerWarpY:
		// pokered/scripts/GameCorner.asm GameCornerSetRocketHideoutDoorTile
		// writes block $2a over (17,4) until EVENT_FOUND_ROCKET_HIDEOUT.
		// The warp table entry remains, so a speedrun cost search prices a
		// one-tile push into that wall and then stops at Rocket B1F's
		// trainer-door frontier — the nearest semantic pivot, not a path to
		// the destination (run-2fjudkv8c4i4y2147qkbldx57h). Gate only this
		// source edge. The reverse stair stays ordinary, and RocketHideout
		// still owns pressing the poster and crossing the revealed step.
		return bikeGate("red:rocket_hideout_entrance", capCanEnterRocketHideout)

	case edge.Kind == world.EdgeWarp && pair(celadonCityMap, celadonGymMap):
		// Erika's door is behind the Cut tree in Celadon City, and a resumed
		// run inside the gym can also need Cut to reach the exit through the
		// interior garden. Keep this as a plain action pivot: skipCanExit lets
		// Traverse own those live Cut approaches in either direction.
		//
		// Do NOT mark this EdgeWarp PortBypass. PortBypass also relaxes the
		// destination landing and turns the gym into a semantic replan boundary.
		// The router can then choose this one-exit room as a "safe prefix" toward
		// unrelated destinations and bounce in/out until navigation stalls
		// (#1586-#1588). Erika's challenge instead stages entry at the gym map
		// before solving the separate interior Cut maze locally.
		return semanticTransition("red:celadon_gym_cut", edge, capCanCut), true
	}
	return gameruntime.Transition{}, false
}

func (x *redRouteTransitionExecutor) executeAuditedRouteTransition(edge world.Edge, transition gameruntime.Transition) (world.TransitionExecutionResult, bool, error) {
	if result, handled, err := x.executeSideRouteTransition(edge, transition); handled {
		return result, true, err
	}
	switch transition.ID {
	case "red:route23_league_approach":
		var mem state.Mem
		state.Snapshot(x.m, &mem)
		caps := redRouteCapabilities(x.romData, &mem)
		missing := make([]gameruntime.CapabilityID, 0, 2)
		if !caps.Has(capCanSurf) {
			missing = append(missing, capCanSurf)
		}
		if !caps.Has(capCanPassRoute23BadgeChecks) {
			missing = append(missing, capCanPassRoute23BadgeChecks)
		}
		if len(missing) != 0 {
			return world.TransitionExecutionResult{}, true, &gameruntime.TransitionBlockage{Transition: transition, Missing: missing}
		}
		if x.policy == nil {
			return world.TransitionExecutionResult{}, true, fmt.Errorf("%w: Route 23 League approach", ErrRouteTransitionNeedsBattlePolicy)
		}
		if got := x.m.Peek8(sym.CurMap); got != route23Map {
			return world.TransitionExecutionResult{}, true, fmt.Errorf("skill: Route 23 League approach started on map %#02x, want %#02x", got, route23Map)
		}
		_, y := playerXY(x.m)
		if y <= route23VictoryRoadApproachY {
			// The player is already on the north cave-door component. Leave the
			// actual warp traversal to the generic edge executor.
			return world.TransitionExecutionResult{}, true, nil
		}
		for _, barrierY := range route23SurfBarrierRows {
			if err := crossRoute23SurfBandNorth(x.m, x.romData, x.policy, barrierY); err != nil {
				return world.TransitionExecutionResult{}, true, fmt.Errorf("skill: Route 23 League approach: %w", err)
			}
		}
		approach := Destination{Map: route23Map, X: route23VictoryRoadWarpX, Y: route23VictoryRoadApproachY}
		if _, err := TravelFlee(x.m, x.romData, approach, x.policy, victoryRoadTravelBattles); err != nil {
			return world.TransitionExecutionResult{}, true, fmt.Errorf("skill: Route 23 League approach: pass badge guards: %w", err)
		}
		facts := currentStoryFacts(x.m)
		if !facts.Route23BadgeChecksComplete || facts.Route23BadgeChecksPassed != 7 {
			return world.TransitionExecutionResult{}, true, fmt.Errorf("skill: Route 23 League approach completed with badge checks %d/7", facts.Route23BadgeChecksPassed)
		}
		return world.TransitionExecutionResult{Changed: true}, true, nil

	case "red:lance_to_champion":
		var mem state.Mem
		state.Snapshot(x.m, &mem)
		if !redRouteCapabilities(x.romData, &mem).Has(capCanPassLanceExit) {
			return world.TransitionExecutionResult{}, true, &gameruntime.TransitionBlockage{
				Transition: transition,
				Missing:    []gameruntime.CapabilityID{capCanPassLanceExit},
			}
		}
		if x.policy == nil {
			return world.TransitionExecutionResult{}, true, fmt.Errorf("%w: Lance to Champion", ErrRouteTransitionNeedsBattlePolicy)
		}
		if err := enterLeagueRoom(x.m, x.romData, x.policy, lanceExitStand, championsRoomMap, true); err != nil {
			return world.TransitionExecutionResult{}, true, fmt.Errorf("skill: Lance to Champion: %w", err)
		}
		return world.TransitionExecutionResult{Changed: true}, true, nil

	case "red:cycling_road_bicycle":
		var mem state.Mem
		state.Snapshot(x.m, &mem)
		if !redRouteCapabilities(x.romData, &mem).Has(capCanRideCyclingRoad) {
			return world.TransitionExecutionResult{}, true, &gameruntime.TransitionBlockage{
				Transition: transition,
				Missing:    []gameruntime.CapabilityID{capCanRideCyclingRoad},
			}
		}
		return world.TransitionExecutionResult{}, true, nil

	case "red:route12_snorlax_access":
		// This is a pure precondition used to stop the static graph from
		// entering Route 12 through Route 11/Lavender before the Poké Flute.
		// The actual wake/battle remains owned by red:route12_snorlax once the
		// later Fuchsia progression intentionally crosses the corridor.
		var mem state.Mem
		state.Snapshot(x.m, &mem)
		if !redRouteCapabilities(x.romData, &mem).Has(capCanClearSnorlax) {
			return world.TransitionExecutionResult{}, true, &gameruntime.TransitionBlockage{
				Transition: transition,
				Missing:    []gameruntime.CapabilityID{capCanClearSnorlax},
			}
		}
		return world.TransitionExecutionResult{}, true, nil

	case "red:route16_snorlax_bicycle":
		var mem state.Mem
		state.Snapshot(x.m, &mem)
		caps := redRouteCapabilities(x.romData, &mem)
		missing := make([]gameruntime.CapabilityID, 0, 2)
		if !caps.Has(capCanClearSnorlax) {
			missing = append(missing, capCanClearSnorlax)
		}
		if !caps.Has(capCanRideCyclingRoad) {
			missing = append(missing, capCanRideCyclingRoad)
		}
		if len(missing) != 0 {
			return world.TransitionExecutionResult{}, true, &gameruntime.TransitionBlockage{Transition: transition, Missing: missing}
		}
		changed, err := x.clearRoute16Snorlax()
		return world.TransitionExecutionResult{Changed: changed}, true, err

	case "red:route16_snorlax":
		var mem state.Mem
		state.Snapshot(x.m, &mem)
		if !redRouteCapabilities(x.romData, &mem).Has(capCanClearSnorlax) {
			return world.TransitionExecutionResult{}, true, &gameruntime.TransitionBlockage{
				Transition: transition,
				Missing:    []gameruntime.CapabilityID{capCanClearSnorlax},
			}
		}
		changed, err := x.clearRoute16Snorlax()
		return world.TransitionExecutionResult{Changed: changed}, true, err

	case "red:southern_sea_surf":
		result, err := x.executeSurf(edge)
		return result, true, err

	case "red:celadon_gym_cut":
		var mem state.Mem
		state.Snapshot(x.m, &mem)
		if !redRouteCapabilities(x.romData, &mem).Has(capCanCut) {
			return world.TransitionExecutionResult{}, true, &gameruntime.TransitionBlockage{
				Transition: transition,
				Missing:    []gameruntime.CapabilityID{capCanCut},
			}
		}
		// The semantic pivot only relaxes static component routing. Traverse's
		// target-specific field approach owns the actual Cut, and after a reverse
		// gym exit ordinary GoTo replans the city-side destination before cutting.
		return world.TransitionExecutionResult{}, true, nil
	}
	return world.TransitionExecutionResult{}, false, nil
}

func (x *redRouteTransitionExecutor) clearRoute16Snorlax() (bool, error) {
	var before state.Mem
	state.Snapshot(x.m, &before)
	if state.HasEvent(&before, eventBeatRoute16Snorlax) {
		return false, nil
	}
	if x.policy == nil {
		return false, fmt.Errorf("%w: Route 16 Snorlax", ErrRouteTransitionNeedsBattlePolicy)
	}
	if before.U8(sym.CurMap) != route16Map {
		return false, fmt.Errorf("skill: Route 16 Snorlax transition started outside Route 16")
	}

	// The Poké Flute effect accepts one tile east or west of Snorlax. Choose
	// the stand on the player's current side so the approach itself never has
	// to path through the sleeping sprite.
	xPos, _ := playerXY(x.m)
	standX := uint8(27)
	if xPos <= 25 {
		standX = 25
	}
	stand := Destination{Map: route16Map, X: standX, Y: 10}
	// The west stand (25,10) is sealed behind the bike-gated gate corridor, so
	// a Fly-house player without a bicycle cannot reach it. When the coordinate
	// stand is unreachable, approach from the east stand instead: the graph
	// routes through the upper gate passage and the Cut tree at (34,9).
	if planner, perr := NewRoutePlanner(x.m, x.romData); perr != nil || !planner.CanReach(stand) {
		stand = Destination{Map: route16Map, X: 27, Y: 10}
	}
	if _, err := TravelFlee(x.m, x.romData, stand, x.policy, fuchsiaTravelEngagements); err != nil {
		return false, fmt.Errorf("skill: Route 16 Snorlax approach: %w", err)
	}
	if err := useOverworldKeyItem(x.m, pokeFluteItemFuchsia, func(mm *state.Mem) bool {
		return state.HasEvent(mm, eventFightRoute16Snorlax) || state.DecodeBattle(mm) != nil
	}); err != nil {
		return false, fmt.Errorf("skill: wake Route 16 Snorlax: %w", err)
	}

	var mem state.Mem
	state.Snapshot(x.m, &mem)
	if state.DecodeBattle(&mem) == nil {
		if err := driveStoryUntil(x.m, fuchsiaStoryBudget, func(mm *state.Mem) bool {
			return state.DecodeBattle(mm) != nil
		}); err != nil {
			return false, fmt.Errorf("skill: Route 16 Snorlax battle did not start: %w", err)
		}
	}
	outcome, err := Battle(x.m, x.policy)
	if err != nil {
		return false, fmt.Errorf("skill: Route 16 Snorlax battle: %w", err)
	}
	if outcome != state.ResultWon {
		return false, fmt.Errorf("skill: Route 16 Snorlax battle ended with outcome %d", outcome)
	}
	if err := Cutscene(x.m, fuchsiaStoryBudget, func(mm *state.Mem) bool {
		return state.HasEvent(mm, eventBeatRoute16Snorlax)
	}); err != nil {
		return false, fmt.Errorf("skill: settle Route 16 Snorlax story: %w", err)
	}
	return true, nil
}
