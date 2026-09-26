package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

var ErrRouteTransitionNeedsBattlePolicy = errors.New("skill: semantic route transition requires a battle policy")

type redRouteTransitionExecutor struct {
	m            *emu.Emu
	romData      []byte
	policy       MovePolicy
	fieldActions gameruntime.FieldActionDecoder
}

func newRedRouteTransitionExecutor(m *emu.Emu, romData []byte, policy MovePolicy) world.TransitionExecutor {
	return &redRouteTransitionExecutor{m: m, romData: romData, policy: policy}
}

func (x *redRouteTransitionExecutor) fieldActionDecoder() (gameruntime.FieldActionDecoder, error) {
	if x.fieldActions != nil {
		return x.fieldActions, nil
	}
	decoder, err := fieldActionDecoderFor(x.m)
	if err != nil {
		return nil, err
	}
	x.fieldActions = decoder
	return decoder, nil
}

// evaluateRedRouteGate is the execution-side mirror of semantic routing for
// passive gates. Gate=true means the transition owns no action: its declared
// Requires are the whole contract, so execution only has to re-read live
// capabilities immediately before traversal. Keeping this generic prevents a
// newly declared gate from being routable but failing later because no
// transition-ID-specific executor case was added.
func evaluateRedRouteGate(romData []byte, mem *state.Mem, transition gameruntime.Transition) (*gameruntime.TransitionBlockage, bool) {
	if !transition.Gate {
		return nil, false
	}
	blockage, usable := gameruntime.EvaluateTransition(transition, redRouteCapabilities(romData, mem))
	if usable {
		return nil, true
	}
	return &blockage, true
}

func (x *redRouteTransitionExecutor) ExecuteTransition(edge world.Edge, transition gameruntime.Transition) (world.TransitionExecutionResult, error) {
	if x == nil || x.m == nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: nil Red semantic transition executor")
	}
	if transition.Gate {
		var mem state.Mem
		state.Snapshot(x.m, &mem)
		if blockage, _ := evaluateRedRouteGate(x.romData, &mem, transition); blockage != nil {
			return world.TransitionExecutionResult{}, blockage
		}
		return world.TransitionExecutionResult{}, nil
	}
	switch transition.ID {
	case "red:viridian_north_pokedex":
		// Viridian's old man steps aside once Oak's Pokedex story completes.
		// Nothing is performed here; re-read the semantic story fact
		// immediately before traversal so a route planned on stale
		// capabilities cannot walk into the still-blocked road.
		var mem state.Mem
		state.Snapshot(x.m, &mem)
		if !redRouteCapabilities(x.romData, &mem).Has(capCanLeaveViridianNorth) {
			return world.TransitionExecutionResult{}, &gameruntime.TransitionBlockage{
				Transition: transition,
				Missing:    []gameruntime.CapabilityID{capCanLeaveViridianNorth},
			}
		}
		return world.TransitionExecutionResult{}, nil

	case "red:pewter_east_boulder":
		// The Pewter east-exit NPC steps aside once Brock is beaten. Like
		// Viridian's old man, this gate performs nothing; only the
		// last-moment capability re-check matters.
		var mem state.Mem
		state.Snapshot(x.m, &mem)
		if !redRouteCapabilities(x.romData, &mem).Has(capCanLeavePewterEast) {
			return world.TransitionExecutionResult{}, &gameruntime.TransitionBlockage{
				Transition: transition,
				Missing:    []gameruntime.CapabilityID{capCanLeavePewterEast},
			}
		}
		return world.TransitionExecutionResult{}, nil

	case "red:mt_moon_exit":
		// A gate has nothing to execute: the corridor opens when the Super
		// Nerd is beaten and paid, which is the fossil objective's job. All
		// that is owed here is a re-read of the world immediately before the
		// step, so a route planned against stale capabilities reports the
		// missing prerequisite instead of walking into him.
		var mem state.Mem
		state.Snapshot(x.m, &mem)
		if !state.DecodeStoryFacts(&mem, state.DecodeInventory(&mem)).MtMoonFossilAcquired {
			return world.TransitionExecutionResult{}, &gameruntime.TransitionBlockage{
				Transition: transition,
				Missing:    []gameruntime.CapabilityID{capCanExitMtMoon},
			}
		}
		return world.TransitionExecutionResult{}, nil

	case "red:cerulean_robbed_house":
		// Bill's S.S. Ticket script moves the guard away from the robbed
		// house. The transition itself performs nothing; re-read the semantic
		// story fact immediately before traversal so a stale plan cannot walk
		// into the still-blocked door.
		var mem state.Mem
		state.Snapshot(x.m, &mem)
		if !state.DecodeStoryFacts(&mem, state.DecodeInventory(&mem)).SSTicketAcquired {
			return world.TransitionExecutionResult{}, &gameruntime.TransitionBlockage{
				Transition: transition,
				Missing:    []gameruntime.CapabilityID{capCanPassCeruleanRobbedHouse},
			}
		}
		return world.TransitionExecutionResult{}, nil

	case "red:ss_anne_ticket":
		// The sailor performs the actual ticket presentation in Vermilion's
		// map script. This gate only verifies the durable bag prerequisite at
		// the last possible moment, so a route planned before Bill completed
		// cannot walk into the harbor guard on stale capabilities.
		var mem state.Mem
		state.Snapshot(x.m, &mem)
		if !state.DecodeStoryFacts(&mem, state.DecodeInventory(&mem)).SSTicketAcquired {
			return world.TransitionExecutionResult{}, &gameruntime.TransitionBlockage{
				Transition: transition,
				Missing:    []gameruntime.CapabilityID{capCanBoardSSAnne},
			}
		}
		return world.TransitionExecutionResult{}, nil

	case "red:route9_cut", "red:vermilion_gym_cut":
		// These semantic pivots exist so the component router may select an
		// edge whose static ROM geometry is split by a Cut tree. Execution is
		// deliberately capability-only: Traverse now approaches the selected
		// connection/warp through the shared destination-aware field planner,
		// which cuts only a tree proven to lie on that exact edge route.
		var mem state.Mem
		state.Snapshot(x.m, &mem)
		if !redRouteCapabilities(x.romData, &mem).Has(capCanCut) {
			return world.TransitionExecutionResult{}, &gameruntime.TransitionBlockage{
				Transition: transition,
				Missing:    []gameruntime.CapabilityID{capCanCut},
			}
		}
		return world.TransitionExecutionResult{}, nil

	case "red:route21_surf":
		return x.executeSurf(edge)

	case "red:route12_snorlax":
		return x.executeRoute12Snorlax()

	case "red:victory_road_strength":
		return x.executeVictoryRoadStrength(edge)

	case "red:rocket_b1f_trainer_door":
		return x.executeRocketB1FTrainerDoorIfNeeded(edge)
	default:
		if result, ok, err := x.executeAuditedRouteTransition(edge, transition); ok {
			return result, err
		}
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: no Red executor owns semantic transition %q", transition.ID)
	}
}

func (x *redRouteTransitionExecutor) executeSurf(edge world.Edge) (world.TransitionExecutionResult, error) {
	fieldActions, err := x.fieldActionDecoder()
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition field-action profile: %w", err)
	}
	if fieldActions.DecodeFieldAction(x.m).Surfing {
		return world.TransitionExecutionResult{}, nil
	}
	if edge.Kind != world.EdgeConnection {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition %02x->%02x is not a map connection", edge.From, edge.To)
	}
	if got := x.m.Peek8(sym.CurMap); got != edge.From {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition starts on %02x, current map is %02x", edge.From, got)
	}

	h, err := routingHeaderFor(x.m, edge.From)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition parse map %02x: %w", edge.From, err)
	}
	water, err := liveMapGridForTraversal(x.m, x.romData, h, world.TraversalWater)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition water grid: %w", err)
	}
	overworld, err := overworldDecoderFor(x.m)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition overworld profile: %w", err)
	}

	// Do not plan the whole approach in TraversalWater and then infer where
	// Surf should start. That treats a semantic mode switch as if the player
	// were already surfing for the entire local leg. Route 19 is the production
	// counterexample: the player can be on the long land beach while the chosen
	// Route 19 -> Route 20 connection is water-only. The water-only approach can
	// reject every border candidate even though an ordinary land walk to a
	// legal shore followed by Surf is available (#1983).
	//
	// Ask the same capability-aware field-path planner used by normal local
	// navigation to reach each concrete connection-band target. It models the
	// land prefix and the first land->water step explicitly. Execute only that
	// prefix and Surf action here; once Surf owns input, ordinary Traverse can
	// cross the map connection with fresh live topology.
	rejectedSurfEntry := map[[2]int]bool{}
	var lastErr error
	for attempts := 0; attempts < 256; attempts++ {
		blocked := spriteBlockers(x.m)
		for at := range rejectedSurfEntry {
			blocked[at] = true
		}

		var (
			bestDest    Destination
			bestPlan    []fieldPathStep
			bestActions int
			bestMoves   int
			found       bool
		)
		for _, at := range connectionFieldTargets(water, edge) {
			if !water.Walkable(at[0], at[1]) || blocked[at] {
				continue
			}
			dest := Destination{Map: edge.From, X: uint8(at[0]), Y: uint8(at[1])}
			plan, _, planErr := currentFieldPathPlanWithCost(x.m, x.romData, h, dest, blocked)
			if planErr != nil {
				continue
			}
			actions := fieldPathPlanActionCount(plan)
			if !found || actions < bestActions || (actions == bestActions && len(plan) < bestMoves) {
				bestDest, bestPlan = dest, plan
				bestActions, bestMoves, found = actions, len(plan), true
			}
		}
		if !found {
			if lastErr != nil {
				return world.TransitionExecutionResult{}, fmt.Errorf("%w: skill: Surf transition exhausted shoreline candidates for %02x->%02x: %v", world.ErrTransitionExecutionStalled, edge.From, edge.To, lastErr)
			}
			return world.TransitionExecutionResult{}, fmt.Errorf("%w: skill: Surf transition has no capability-aware path to %02x connection", world.ErrTransitionExecutionStalled, edge.To)
		}

		prefix, action := firstFieldAction(bestPlan)
		if action == nil {
			// The selected band is reachable entirely on land. The semantic
			// Surf annotation is only a capability guard for other components;
			// let generic connection traversal own this ordinary crossing.
			return world.TransitionExecutionResult{}, nil
		}
		if action.Action != fieldPathSurf {
			// A Surf transition must not opportunistically perform an unrelated
			// field action such as Cut. Leave that topology change to the layer
			// that owns it and try another concrete connection target.
			rejectedSurfEntry[[2]int{int(bestDest.X), int(bestDest.Y)}] = true
			lastErr = fmt.Errorf("connection target (%d,%d) first requires field action %d, want Surf", bestDest.X, bestDest.Y, action.Action)
			continue
		}

		sx, sy := playerXY(x.m)
		standX, standY := int(sx), int(sy)
		for _, step := range prefix {
			standX += step.DX
			standY += step.DY
		}
		if standX < 0 || standY < 0 || standX > 255 || standY > 255 {
			rejectedSurfEntry[[2]int{int(bestDest.X), int(bestDest.Y)}] = true
			lastErr = fmt.Errorf("field-path Surf stand (%d,%d) is outside map coordinates", standX, standY)
			continue
		}

		if err := walkWithinMap(x.m, x.romData, Destination{Map: edge.From, X: uint8(standX), Y: uint8(standY)}, x.policy); err != nil {
			return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition reach shoreline (%d,%d): %w", standX, standY, err)
		}

		_, waterX, waterY, targetErr := fieldPathActionTargetWithDecoder(x.m, overworld, *action)
		if targetErr != nil {
			return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition resolve shoreline target: %w", targetErr)
		}
		surfErr := executeFieldPathActionWithDecoders(x.m, overworld, fieldActions, *action)
		if surfErr == nil && fieldActions.DecodeFieldAction(x.m).Surfing {
			return world.TransitionExecutionResult{Changed: true}, nil
		}

		// A failed Surf action is evidence about this exact shore, not the whole
		// map connection. Block its first water tile and let the field planner
		// choose a different land->water entry on the same connection band.
		rejectedSurfEntry[[2]int{waterX, waterY}] = true
		if surfErr != nil {
			lastErr = fmt.Errorf("shore (%d,%d) facing water (%d,%d): %w", standX, standY, waterX, waterY, surfErr)
		} else {
			lastErr = fmt.Errorf("shore (%d,%d) facing water (%d,%d) returned without verified surfing state", standX, standY, waterX, waterY)
		}
	}
	return world.TransitionExecutionResult{}, fmt.Errorf("%w: skill: Surf transition exceeded shoreline retry budget for %02x->%02x: %v", world.ErrTransitionExecutionStalled, edge.From, edge.To, lastErr)
}

func (x *redRouteTransitionExecutor) executeRoute12Snorlax() (world.TransitionExecutionResult, error) {
	var before state.Mem
	state.Snapshot(x.m, &before)
	if state.HasEvent(&before, eventBeatRoute12Snorlax) {
		return world.TransitionExecutionResult{}, nil
	}
	if x.policy == nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("%w: Route 12 Snorlax", ErrRouteTransitionNeedsBattlePolicy)
	}
	if err := clearRoute12Snorlax(x.m, x.romData, x.policy); err != nil {
		return world.TransitionExecutionResult{}, err
	}
	var after state.Mem
	state.Snapshot(x.m, &after)
	if !state.HasEvent(&after, eventBeatRoute12Snorlax) {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Route 12 Snorlax transition completed without EVENT_BEAT_ROUTE12_SNORLAX")
	}
	return world.TransitionExecutionResult{Changed: true}, nil
}

// executeRocketB1FTrainerDoor fights the Rocket5 grunt whose defeat flips
// RocketHideoutB1FDoorCallbackScript's block replacement, opening the only
// passage from the elevator landing to B1F's Game Corner and B2F-stair exits.
func (x *redRouteTransitionExecutor) executeRocketB1FTrainerDoor() (world.TransitionExecutionResult, error) {
	var before state.Mem
	state.Snapshot(x.m, &before)
	if state.HasEvent(&before, eventBeatRocketB1FTrainer4) {
		return world.TransitionExecutionResult{}, nil
	}
	if x.policy == nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("%w: Rocket Hideout B1F door", ErrRouteTransitionNeedsBattlePolicy)
	}
	if got := x.m.Peek8(sym.CurMap); got != rocketHideoutB1FMap {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Rocket B1F door transition on map %#04x, want B1F %#04x", got, rocketHideoutB1FMap)
	}
	if err := ChallengeTrainer(x.m, x.romData, rocketB1FTrainer5X, rocketB1FTrainer5Y, x.policy); err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Rocket B1F door: fight Rocket5: %w", err)
	}
	var after state.Mem
	state.Snapshot(x.m, &after)
	if !state.HasEvent(&after, eventBeatRocketB1FTrainer4) {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Rocket B1F door transition completed without EVENT_BEAT_ROCKET_HIDEOUT_1_TRAINER_4")
	}
	return world.TransitionExecutionResult{Changed: true}, nil
}

func victoryRoadSectionForTransition(edge world.Edge) (VictoryRoadBoulderSection, bool) {
	switch {
	case edge.From == victoryRoad1FMap && edge.To == victoryRoad2FMap:
		return VictoryRoad1FSwitch, true
	case edge.From == victoryRoad2FMap && edge.To == victoryRoad3FMap:
		return VictoryRoad2FSwitch1, true
	case edge.From == victoryRoad3FMap && edge.To == victoryRoad2FMap:
		return VictoryRoad3FSwitch, true
	default:
		// Descending 2F -> 1F is geometrically traversable and does not own a
		// new boulder objective; the coarse bidirectional semantic annotation
		// remains harmless until the route-fact model is made directional.
		return 0, false
	}
}

func (x *redRouteTransitionExecutor) executeVictoryRoadStrength(edge world.Edge) (world.TransitionExecutionResult, error) {
	section, ok := victoryRoadSectionForTransition(edge)
	if !ok {
		return world.TransitionExecutionResult{}, nil
	}
	spec, _ := VictoryRoadBoulderSpec(section)
	var before state.Mem
	state.Snapshot(x.m, &before)
	// The switch only gates the warp; 2F entry resets the 1F switch event
	// while leaving the player past the barrier on the way back down.
	if boulderPuzzleEventComplete(&before, spec) || warpEdgeReachable(x.m, x.romData, edge) {
		return world.TransitionExecutionResult{}, nil
	}
	if x.policy == nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("%w: %s", ErrRouteTransitionNeedsBattlePolicy, section)
	}
	if _, err := SolveVictoryRoadBoulderSection(x.m, x.romData, x.policy, section); err != nil {
		return world.TransitionExecutionResult{}, err
	}
	var after state.Mem
	state.Snapshot(x.m, &after)
	if !boulderPuzzleEventComplete(&after, spec) {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: %s solver returned without its completion event", section)
	}
	return world.TransitionExecutionResult{Changed: true}, nil
}
