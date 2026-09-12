package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

var ErrRouteTransitionNeedsBattlePolicy = errors.New("skill: semantic route transition requires a battle policy")

type redRouteTransitionExecutor struct {
	m       *emu.Emu
	romData []byte
	policy  MovePolicy
}

func newRedRouteTransitionExecutor(m *emu.Emu, romData []byte, policy MovePolicy) world.TransitionExecutor {
	return &redRouteTransitionExecutor{m: m, romData: romData, policy: policy}
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

	case "red:route9_cut":
		// The actual tree is inside Route 9 and is still handled by Travel's
		// live Cut recovery. This gate only proves that recovery is possible
		// before the planner commits to the east route.
		var mem state.Mem
		state.Snapshot(x.m, &mem)
		if !redRouteCapabilities(x.romData, &mem).Has(capCanCut) {
			return world.TransitionExecutionResult{}, &gameruntime.TransitionBlockage{
				Transition: transition,
				Missing:    []gameruntime.CapabilityID{capCanCut},
			}
		}
		return world.TransitionExecutionResult{}, nil

	case "red:vermilion_gym_cut":
		if edge.From == vermilionGymMap {
			// Leaving: the tree stands outside, on Vermilion City's side of
			// this door, so cutThroughReachableTree (which only ever looks at
			// the CURRENT map, and only recognizes the exact overworld/gym
			// tree tile ids) finds nothing while still inside the gym, and
			// still misses this one live tile-ID quirk once outside. Cross
			// the door ourselves first, then reuse EnterVermilionGym's own
			// tree finder — already proven against this exact tree — to clear
			// whatever still blocks the yard the door lands in, so the exit
			// this transition promised is the one the walker actually gets.
			// Measured on run-3djisxgsy3dgzpnsde2inzyuh round 7: the
			// one-directional gate only ever cut the tree on entry, so every
			// later GoTo leaving the Gym found the same tree still standing
			// and reported "world: no route" trying to reach Celadon.
			if err := Traverse(x.m, x.romData, edge); err != nil {
				return world.TransitionExecutionResult{}, fmt.Errorf("Cut gate: leave gym: %w", err)
			}
			h, err := rom.ParseMap(x.romData, vermilionCity)
			if err != nil {
				return world.TransitionExecutionResult{}, fmt.Errorf("Cut gate: leave gym: parse city: %w", err)
			}
			grid, err := world.Build(x.romData, h)
			if err != nil {
				return world.TransitionExecutionResult{}, fmt.Errorf("Cut gate: leave gym: build city: %w", err)
			}
			tree, err := findVermilionGymTree(x.m, x.romData, grid, x.policy)
			if err != nil {
				// No verifiable tree left standing is the idempotent
				// already-cut case (a later run through the same door, or a
				// route that lands beside a tree some earlier leg already
				// removed): Traverse already delivered the crossing this
				// transition promised, so report it and let ordinary
				// geometry take it from here instead of failing the leg.
				return world.TransitionExecutionResult{Changed: true}, nil
			}
			if err := CutAhead(x.m); err != nil {
				return world.TransitionExecutionResult{}, fmt.Errorf("Cut gate: leave gym: cut tree at (%d,%d): %w", tree.x, tree.y, err)
			}
			// Traverse already performed the crossing; report Changed so the
			// caller re-plans from the new position instead of traversing e again.
			return world.TransitionExecutionResult{Changed: true}, nil
		}
		opened, err := cutThroughReachableTree(x.m, x.romData)
		if err != nil {
			return world.TransitionExecutionResult{}, fmt.Errorf("Cut gate: %w", err)
		}
		// No candidate is the idempotent already-open case. Traverse is the
		// positive proof that ordinary geometry is now sufficient.
		return world.TransitionExecutionResult{Changed: opened}, nil

	case "red:route21_surf":
		return x.executeSurf(edge)

	case "red:route12_snorlax":
		return x.executeRoute12Snorlax()

	case "red:victory_road_strength":
		return x.executeVictoryRoadStrength(edge)
	default:
		if result, ok, err := x.executeAuditedRouteTransition(edge, transition); ok {
			return result, err
		}
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: no Red executor owns semantic transition %q", transition.ID)
	}
}

func (x *redRouteTransitionExecutor) executeSurf(edge world.Edge) (world.TransitionExecutionResult, error) {
	if x.m.Peek8(sym.WalkBikeSurfState) == fieldSurfingState {
		return world.TransitionExecutionResult{}, nil
	}
	if edge.Kind != world.EdgeConnection {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition %02x->%02x is not a map connection", edge.From, edge.To)
	}
	if got := x.m.Peek8(sym.CurMap); got != edge.From {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition starts on %02x, current map is %02x", edge.From, got)
	}

	h, err := rom.ParseMap(x.romData, edge.From)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition parse map %02x: %w", edge.From, err)
	}
	land, err := liveMapGridForTraversal(x.m, x.romData, h, world.TraversalLand)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition land grid: %w", err)
	}
	water, err := liveMapGridForTraversal(x.m, x.romData, h, world.TraversalWater)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition water grid: %w", err)
	}
	sx, sy := playerXY(x.m)
	blocked := spriteBlockers(x.m)
	tx, ty, err := edgeTarget(water, edge.Dir, int(sx), int(sy), blocked)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition cannot reach %02x connection in water mode: %w", edge.To, err)
	}
	steps, err := world.FindPath(water, int(sx), int(sy), tx, ty, blocked)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition water path: %w", err)
	}

	px, py := int(sx), int(sy)
	standX, standY, waterX, waterY := 0, 0, 0, 0
	found := false
	for _, step := range steps {
		nx, ny := px+step.DX, py+step.DY
		if !land.Passable(px, py, nx, ny) && water.Passable(px, py, nx, ny) {
			standX, standY, waterX, waterY = px, py, nx, ny
			found = true
			break
		}
		px, py = nx, ny
	}
	if !found {
		// The connection is already ordinary-walkable from this position;
		// do not enter Surf merely because the semantic edge is annotated.
		return world.TransitionExecutionResult{}, nil
	}
	if err := walkWithinMap(x.m, x.romData, Destination{Map: edge.From, X: uint8(standX), Y: uint8(standY)}); err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition reach shoreline (%d,%d): %w", standX, standY, err)
	}
	if err := Face(x.m, uint8(waterX), uint8(waterY)); err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition face water (%d,%d): %w", waterX, waterY, err)
	}
	x.m.StepFrames(2)
	result, err := UseFieldMove(x.m, FieldSurf)
	if err != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition enter mode: %w", err)
	}
	if !result.Surfing || x.m.Peek8(sym.WalkBikeSurfState) != fieldSurfingState {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition returned without verified surfing state")
	}
	return world.TransitionExecutionResult{Changed: true}, nil
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
	if boulderPuzzleEventComplete(&before, spec) {
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
