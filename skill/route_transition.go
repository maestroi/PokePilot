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
	g, gerr := world.BuildGraph(x.romData)
	if gerr != nil {
		return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition build graph: %w", gerr)
	}

	// A semantic Surf edge can expose several geometrically plausible shore
	// tiles, but the ROM is authoritative about whether pressing Surf while
	// facing across that exact seam is legal. Pallet -> Route 21 is the
	// production case: (3,17) is reachable in the static water view but the
	// game rejects Surf there, while another tile on the same south edge works.
	// Treat a rejected shore as per-candidate evidence and keep searching the
	// same connection band instead of terminating the whole transition.
	excluded := map[[2]int]bool{}
	blocked := spriteBlockers(x.m)
	var lastErr error
	for attempts := 0; attempts < 256; attempts++ {
		sx, sy := playerXY(x.m)
		tx, ty, targetErr := edgeTargetForConnectionExcluding(water, edge, int(sx), int(sy), blocked, excluded)
		if targetErr != nil {
			if lastErr != nil {
				return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition exhausted shoreline candidates for %02x->%02x: %w", edge.From, edge.To, lastErr)
			}
			return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition cannot reach %02x connection in water mode: %w", edge.To, targetErr)
		}
		steps, pathErr := world.FindPath(water, int(sx), int(sy), tx, ty, blocked)
		if pathErr != nil {
			excluded[[2]int{tx, ty}] = true
			lastErr = pathErr
			continue
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
			if g.ConnectionExitWalkable(edge) {
				// Destination has an ordinary land landing. The semantic annotation
				// is satisfied but no Surf entry is required from this component.
				return world.TransitionExecutionResult{}, nil
			}
			standX, standY = tx, ty
			// The tile one step beyond the player in the connection direction is
			// outside this map's grid; the ROM's IsNextTileShoreOrWater reads
			// wTileInFrontOfPlayer from the current map, so an out-of-bounds target
			// gives a wrong tile id and Surf is rejected. Scan the in-map
			// neighbours for a collision tile the ROM recognises as water ($14),
			// shore ($32), or Safari shore ($48) and face that instead.
			waterX, waterY = -1, -1
			for _, n := range [][2]int{{tx + 1, ty}, {tx - 1, ty}, {tx, ty + 1}, {tx, ty - 1}} {
				if !water.InBounds(n[0], n[1]) {
					continue
				}
				id, ok := water.Tile(n[0], n[1])
				if !ok {
					continue
				}
				if id == surfWaterTile || id == 0x32 || id == 0x48 {
					waterX, waterY = n[0], n[1]
					break
				}
			}
			if waterX < 0 {
				excluded[[2]int{tx, ty}] = true
				lastErr = fmt.Errorf("shore (%d,%d): no in-map water/shore tile adjacent for Surf target", tx, ty)
				continue
			}
		}

		if err := walkWithinMap(x.m, x.romData, Destination{Map: edge.From, X: uint8(standX), Y: uint8(standY)}); err != nil {
			return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition reach shoreline (%d,%d): %w", standX, standY, err)
		}
		if err := Face(x.m, uint8(waterX), uint8(waterY)); err != nil {
			return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition face water (%d,%d): %w", waterX, waterY, err)
		}
		x.m.StepFrames(2)
		result, surfErr := UseFieldMove(x.m, FieldSurf)
		if surfErr == nil && result.Surfing && x.m.Peek8(sym.WalkBikeSurfState) == fieldSurfingState {
			return world.TransitionExecutionResult{Changed: true}, nil
		}

		// The ROM rejected this exact shore. Exclude the edge target that led
		// here and retry another candidate on the same band. A failed Surf use
		// does not consume the capability or alter map topology.
		excluded[[2]int{tx, ty}] = true
		if surfErr != nil {
			lastErr = fmt.Errorf("shore (%d,%d) facing (%d,%d): %w", standX, standY, waterX, waterY, surfErr)
		} else {
			lastErr = fmt.Errorf("shore (%d,%d) facing (%d,%d) returned without verified surfing state", standX, standY, waterX, waterY)
		}
	}
	return world.TransitionExecutionResult{}, fmt.Errorf("skill: Surf transition exceeded shoreline retry budget for %02x->%02x: %w", edge.From, edge.To, lastErr)
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
