package skill

import (
	"fmt"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

const (
	capCanRideCyclingRoad gameruntime.CapabilityID = "can_ride_cycling_road"

	bicycleItem uint8 = 0x06

	route16Map       uint8 = 0x1B
	route17Map       uint8 = 0x1C
	route18Map       uint8 = 0x1D
	route19Map       uint8 = 0x1E
	route20Map       uint8 = 0x1F
	route16Gate1FMap uint8 = 0xBA
	route18Gate1FMap uint8 = 0xBE

	eventFightRoute16Snorlax state.Event = 0x4C8
	eventBeatRoute16Snorlax  state.Event = 0x4C9
)

func addAuditedRedRouteCapabilities(mem *state.Mem, caps gameruntime.CapabilitySet) {
	if _, count := bagEntry(mem, bicycleItem); count > 0 {
		caps[capCanRideCyclingRoad] = true
	}
}

func redAuditedRouteTransitionForEdge(edge world.Edge) (gameruntime.Transition, bool) {
	pair := func(a, b uint8) bool {
		return (edge.From == a && edge.To == b) || (edge.From == b && edge.To == a)
	}
	bikeGate := func(id string, requires ...gameruntime.CapabilityID) (gameruntime.Transition, bool) {
		t := semanticTransition(id, edge, requires...)
		t.Gate = true
		return t, true
	}

	switch {
	case edge.Kind == world.EdgeWarp && edge.From == route16Map && edge.To == route16Gate1FMap &&
		edge.WarpX == 24 && (edge.WarpY == 10 || edge.WarpY == 11):
		// The east entrance to Route 16's lower gate is reached from Celadon.
		// Snorlax sits immediately east of the gate, and the guard inside the
		// lower corridor separately requires a Bicycle. This transition owns
		// both preconditions so a journey to Fuchsia cannot walk into either
		// scripted blocker before reporting why the route is closed.
		return semanticTransition("red:route16_snorlax_bicycle", edge, capCanClearSnorlax, capCanRideCyclingRoad), true

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
		// Returning north from Cycling Road exits onto Route 16 west of the
		// sleeping Snorlax. Clearing it is an action, not merely a gate, so the
		// executor uses the Flute and resolves the battle before traversal.
		return semanticTransition("red:route16_snorlax", edge, capCanClearSnorlax), true

	case pair(route19Map, route20Map), pair(route20Map, cinnabarIslandMap):
		// The southern sea route is every bit as Surf-gated as Route 21. Keep
		// Route 19 itself walkable from Fuchsia; Surf starts at the Route 19/20
		// seam and remains required through the Cinnabar connection.
		return semanticTransition("red:southern_sea_surf", edge, capCanSurf), true

	case edge.Kind == world.EdgeWarp && edge.From == celadonCityMap && edge.To == celadonGymMap:
		// Erika's door is behind the Cut tree in Celadon City. Make the named
		// gym destination inherit that field prerequisite instead of relying on
		// an execution-time no-path failure.
		return semanticTransition("red:celadon_gym_cut", edge, capCanCut), true
	}
	return gameruntime.Transition{}, false
}

func (x *redRouteTransitionExecutor) executeAuditedRouteTransition(edge world.Edge, transition gameruntime.Transition) (world.TransitionExecutionResult, bool, error) {
	switch transition.ID {
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
		opened, err := cutThroughReachableTree(x.m, x.romData)
		if err != nil {
			return world.TransitionExecutionResult{}, true, fmt.Errorf("Celadon Gym Cut gate: %w", err)
		}
		return world.TransitionExecutionResult{Changed: opened}, true, nil
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
	if _, err := TravelFlee(x.m, x.romData, Destination{Map: route16Map, X: standX, Y: 10}, x.policy, fuchsiaTravelEngagements); err != nil {
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
