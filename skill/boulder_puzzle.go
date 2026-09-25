package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

var (
	ErrBoulderObservationMismatch = errors.New("skill: boulder observation did not match planned push")
	ErrBoulderPuzzlePushLimit     = errors.New("skill: boulder puzzle push limit reached")
	ErrBoulderPuzzleReplanLimit   = errors.New("skill: boulder puzzle replan limit reached")
	ErrBoulderPuzzleMapChanged    = errors.New("skill: boulder puzzle unexpectedly changed maps")
)

const (
	defaultBoulderPuzzlePushLimit   = 96
	defaultBoulderPuzzleReplanLimit = 256
	boulderPushObserveBudget        = 180
)

// BoulderPuzzleSpec describes a live single-map Strength puzzle. Targets are
// switch/hole coordinates to occupy. Reachable optionally asks the generic
// planner to move enough boulders for a player tile to become reachable.
// TerminalTargets are targets where the game consumes/hides the boulder after
// a successful push (Victory Road 3F's hole is the canonical example).
//
// CompleteEvent is optional and is used as a positive ROM-side completion
// fact when the map script provides one. HasCompleteEvent distinguishes event
// index zero from "no event".
type BoulderPuzzleSpec struct {
	Map       uint8
	Targets   []world.Point
	Reachable *world.Point
	// Fixed adds caller-owned non-movable blockers to the live sprite
	// snapshot. Generic navigation uses this for unrelated warp tiles so a
	// boulder solution cannot "solve" a route by accidentally stepping into
	// another map.
	Fixed map[[2]int]bool
	// MovableIDs optionally restricts the puzzle to specific live sprite slots.
	// This matters for maps such as Seafoam B3F where unrelated Strength
	// boulders share the same map but only a named pair may be sunk into the
	// current-blocking holes.
	MovableIDs       map[int]bool
	TerminalTargets  map[[2]int]bool
	CompleteEvent    state.Event
	HasCompleteEvent bool
	MaxStates        int
	MaxPushes        int
	MaxReplans       int
}

// BoulderPuzzleResult reports the verified work performed by SolveBoulderPuzzle.
type BoulderPuzzleResult struct {
	Pushes   int
	Replans  int
	Explored int
}

func liveBoulderMovables(mem *state.Mem) []world.Movable {
	boulders := state.DecodeBoulders(mem)
	out := make([]world.Movable, 0, len(boulders))
	for _, boulder := range boulders {
		out = append(out, world.Movable{
			ID:  boulder.Slot,
			Pos: world.Point{X: boulder.X, Y: boulder.Y},
		})
	}
	return out
}

func selectedBoulderMovables(movables []world.Movable, allowed map[int]bool) []world.Movable {
	if len(allowed) == 0 {
		return movables
	}
	out := make([]world.Movable, 0, len(movables))
	for _, movable := range movables {
		if allowed[movable.ID] {
			out = append(out, movable)
		}
	}
	return out
}

func unselectedBoulderBlockers(movables []world.Movable, allowed map[int]bool) map[[2]int]bool {
	out := map[[2]int]bool{}
	if len(allowed) == 0 {
		return out
	}
	for _, movable := range movables {
		if !allowed[movable.ID] {
			out[[2]int{movable.Pos.X, movable.Pos.Y}] = true
		}
	}
	return out
}

func liveNonBoulderBlockers(mem *state.Mem) map[[2]int]bool {
	out := map[[2]int]bool{}
	for _, sprite := range state.DecodeSprites(mem) {
		if sprite.PictureID == state.BoulderPictureID {
			continue
		}
		out[[2]int{sprite.X, sprite.Y}] = true
	}
	return out
}

func observedBoulderBySlot(mem *state.Mem, slot int) (state.BoulderState, bool) {
	for _, boulder := range state.DecodeBoulders(mem) {
		if boulder.Slot == slot {
			return boulder, true
		}
	}
	return state.BoulderState{}, false
}

func boulderPuzzleEventComplete(mem *state.Mem, spec BoulderPuzzleSpec) bool {
	return spec.HasCompleteEvent && state.HasEvent(mem, spec.CompleteEvent)
}

func validateBoulderPuzzleSpec(spec BoulderPuzzleSpec) error {
	if len(spec.Targets) == 0 && spec.Reachable == nil && !spec.HasCompleteEvent {
		return fmt.Errorf("skill: boulder puzzle map %#02x has no target, reachability goal, or completion event", spec.Map)
	}
	for target := range spec.TerminalTargets {
		found := false
		for _, p := range spec.Targets {
			if target == [2]int{p.X, p.Y} {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("skill: terminal boulder target (%d,%d) is not in puzzle target set", target[0], target[1])
		}
	}
	return nil
}

func currentBoulderPuzzle(m *emu.Emu, romData []byte, spec BoulderPuzzleSpec) (world.PushPuzzle, *state.Mem, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	cur := mem.U8(sym.CurMap)
	if cur != spec.Map {
		return world.PushPuzzle{}, nil, fmt.Errorf("skill: boulder puzzle is for map %#02x, current map is %#02x", spec.Map, cur)
	}
	if !state.Controllable(&mem) {
		return world.PushPuzzle{}, nil, fmt.Errorf("skill: boulder puzzle player is not controllable on map %#02x", cur)
	}

	h, err := rom.ParseMap(romData, cur)
	if err != nil {
		return world.PushPuzzle{}, nil, fmt.Errorf("skill: boulder puzzle parse map %#02x: %w", cur, err)
	}
	grid, err := liveMapGrid(m, romData, h)
	if err != nil {
		return world.PushPuzzle{}, nil, fmt.Errorf("skill: boulder puzzle live grid for map %#02x: %w", cur, err)
	}
	player := state.DecodePlayer(&mem)
	fixed := liveNonBoulderBlockers(&mem)
	allMovables := liveBoulderMovables(&mem)
	movables := selectedBoulderMovables(allMovables, spec.MovableIDs)
	for at := range unselectedBoulderBlockers(allMovables, spec.MovableIDs) {
		fixed[at] = true
	}
	for at, blocked := range spec.Fixed {
		if blocked {
			fixed[at] = true
		}
	}
	return world.PushPuzzle{
		Grid:      grid,
		Player:    world.Point{X: int(player.X), Y: int(player.Y)},
		Movables:  movables,
		Fixed:     fixed,
		Goal:      world.PushGoal{Targets: spec.Targets, Reachable: spec.Reachable},
		MaxStates: spec.MaxStates,
	}, &mem, nil
}

// settleBoulderPush waits for the pushed sprite slot to agree with the plan.
// A terminal target may legally make the sprite disappear; when a completion
// event exists, disappearance is accepted only after that event is observed.
func settleBoulderPush(m *emu.Emu, spec BoulderPuzzleSpec, push world.Push) error {
	terminal := spec.TerminalTargets[[2]int{push.To.X, push.To.Y}]
	var last state.Mem
	for spent := 0; spent <= boulderPushObserveBudget; spent += 10 {
		state.Snapshot(m, &last)
		if got := last.U8(sym.CurMap); got != spec.Map {
			return fmt.Errorf("%w: expected map %#02x after pushing slot %d, observed %#02x", ErrBoulderPuzzleMapChanged, spec.Map, push.MovableID, got)
		}
		boulder, found := observedBoulderBySlot(&last, push.MovableID)
		if found && boulder.X == push.To.X && boulder.Y == push.To.Y {
			return nil
		}
		if terminal && !found {
			if !spec.HasCompleteEvent || boulderPuzzleEventComplete(&last, spec) {
				return nil
			}
		}
		m.StepFrames(10)
	}

	boulder, found := observedBoulderBySlot(&last, push.MovableID)
	return fmt.Errorf("%w: slot %d planned (%d,%d)->(%d,%d), after %d frames found=%v observed=(%d,%d) terminal=%v eventComplete=%v",
		ErrBoulderObservationMismatch, push.MovableID,
		push.From.X, push.From.Y, push.To.X, push.To.Y,
		boulderPushObserveBudget, found, boulder.X, boulder.Y, terminal,
		boulderPuzzleEventComplete(&last, spec))
}

// executeObservedBoulderPush performs one planned push from live state. It
// never resolves a battle or text box itself: an interruption is returned
// as-is so SolveBoulderPuzzle's shared interruption runner can resolve it and
// re-enter the solver, which re-observes and re-plans before any further push.
func executeObservedBoulderPush(m *emu.Emu, spec BoulderPuzzleSpec, push world.Push) (bool, error) {
	if err := WalkPath(m, push.Walk); err != nil {
		if errors.Is(err, ErrBattleInterrupted) || errors.Is(err, ErrDialogueInterrupted) {
			return false, err
		}
		var blocked *ErrBlocked
		if errors.As(err, &blocked) {
			return false, nil // a live blocker moved after planning; observe again
		}
		return false, fmt.Errorf("skill: boulder puzzle walk to slot %d push stand (%d,%d): %w", push.MovableID, push.Stand.X, push.Stand.Y, err)
	}

	var before state.Mem
	state.Snapshot(m, &before)
	if got := before.U8(sym.CurMap); got != spec.Map {
		return false, fmt.Errorf("%w: walk to push stand left map %#02x for %#02x", ErrBoulderPuzzleMapChanged, spec.Map, got)
	}
	player := state.DecodePlayer(&before)
	if int(player.X) != push.Stand.X || int(player.Y) != push.Stand.Y {
		return false, fmt.Errorf("%w: planned player stand (%d,%d), observed (%d,%d)", ErrBoulderObservationMismatch, push.Stand.X, push.Stand.Y, player.X, player.Y)
	}
	boulder, found := observedBoulderBySlot(&before, push.MovableID)
	if !found || boulder.X != push.From.X || boulder.Y != push.From.Y {
		return false, fmt.Errorf("%w: slot %d planned at (%d,%d), found=%v observed=(%d,%d)", ErrBoulderObservationMismatch, push.MovableID, push.From.X, push.From.Y, found, boulder.X, boulder.Y)
	}

	if err := Face(m, uint8(push.From.X), uint8(push.From.Y)); err != nil {
		if errors.Is(err, ErrBattle) {
			return false, fmt.Errorf("%w: facing slot %d at (%d,%d): %v", ErrBattleInterrupted, push.MovableID, push.From.X, push.From.Y, err)
		}
		return false, fmt.Errorf("skill: boulder puzzle face slot %d at (%d,%d): %w", push.MovableID, push.From.X, push.From.Y, err)
	}
	state.Snapshot(m, &before)
	fieldActions, err := fieldActionDecoderFor(m)
	if err != nil {
		return false, fmt.Errorf("skill: boulder puzzle field-action profile: %w", err)
	}
	if !fieldActions.DecodeFieldAction(m).StrengthActive {
		if _, err := useFieldMoveWithDecoder(m, FieldStrength, fieldActions); err != nil {
			return false, fmt.Errorf("skill: boulder puzzle activate Strength for slot %d: %w", push.MovableID, err)
		}
	}

	stepErr := StepOnce(m, push.Direction)
	// StepOnce deliberately reports the coordinate change even when a wild
	// encounter has started taking over the screen. WalkPath therefore checks
	// movementInterruption before trusting stepErr; a direct Strength push must
	// honor the same contract. Without this check, Victory Road could commit a
	// valid boulder push, enter a battle on that step, and then fail the next
	// puzzle observation as uncontrollable. The objective finish boundary would
	// see the still-live battle and escalate the otherwise recoverable encounter
	// into stabilization_failed/objective_boundary_dirty (#1742, #1745).
	if interruptErr := movementInterruption(m); interruptErr != nil {
		return false, interruptErr // push/world may have changed: resolve, observe and re-plan
	}
	if stepErr != nil {
		var blocked *ErrBlocked
		if errors.As(stepErr, &blocked) {
			return false, nil // destination changed after planning; observe again
		}
		return false, fmt.Errorf("skill: boulder puzzle push slot %d %s from (%d,%d): %w", push.MovableID, push.Direction, push.From.X, push.From.Y, stepErr)
	}

	px, py := playerXY(m)
	if int(px) != push.From.X || int(py) != push.From.Y {
		return false, fmt.Errorf("%w: after slot %d push player=(%d,%d), want old boulder tile (%d,%d)", ErrBoulderObservationMismatch, push.MovableID, px, py, push.From.X, push.From.Y)
	}
	if err := settleBoulderPush(m, spec, push); err != nil {
		return false, err
	}
	return true, nil
}

// SolveBoulderPuzzle solves a Strength puzzle from the *current observed
// state*. It builds #105's live map grid, plans only the next push, executes it
// through real movement/Strength, positively verifies player and boulder RAM,
// then throws the rest of the plan away and re-plans. This makes a resumed
// mid-puzzle checkpoint identical to any other starting state and ensures map
// scripts that open doors or hide/show boulders are incorporated immediately.
// localStrengthPuzzleSpec builds the generic GoTo reachability form of the
// boulder solver. Unlike story puzzle specs it has no switch/hole target: the
// only goal is making the requested standing tile reachable. Unrelated warps
// are fixed blockers so the push search stays on this map.
func localStrengthPuzzleSpec(m *emu.Emu, h rom.MapHeader, dest Destination) BoulderPuzzleSpec {
	sx, sy := playerXY(m)
	fixed := warpAvoidance(h, int(sx), int(sy), nil)
	return BoulderPuzzleSpec{
		Map:       h.ID,
		Reachable: &world.Point{X: int(dest.X), Y: int(dest.Y)},
		Fixed:     fixed,
	}
}

// currentLocalStrengthPlan asks the push solver whether moving one or more
// live boulders can make dest reachable. A zero-push result is deliberately
// reported as not-needed: ordinary/Cut/Surf pathing owns that case.
func currentLocalStrengthPlan(m *emu.Emu, romData []byte, h rom.MapHeader, dest Destination) (world.PushPlan, bool, error) {
	if h.ID != dest.Map || m.Peek8(sym.CurMap) != dest.Map {
		return world.PushPlan{}, false, nil
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	fieldActions, err := fieldActionDecoderFor(m)
	if err != nil {
		return world.PushPlan{}, false, err
	}
	if fieldActions.DecodeFieldAction(m).Surfing || len(state.DecodeBoulders(&mem)) == 0 {
		return world.PushPlan{}, false, nil
	}
	puzzle, _, err := currentBoulderPuzzle(m, romData, localStrengthPuzzleSpec(m, h, dest))
	if err != nil {
		return world.PushPlan{}, false, err
	}
	plan, err := world.PlanPushPuzzle(puzzle)
	if err != nil {
		if errors.Is(err, world.ErrPushPuzzleNoSolution) {
			return world.PushPlan{}, false, nil
		}
		return world.PushPlan{}, false, err
	}
	return plan, len(plan.Pushes) > 0, nil
}

// weightedStrengthPlanCost estimates the same coarse travel units used by
// fastest field-path routing. Walking to each push stand and the push itself
// cost movement units; activating Strength is a one-time field-action cost.
// The push solver already supplies FinalWalk for a reachability goal.
func weightedStrengthPlanCost(plan world.PushPlan, strengthActive bool, policy fieldPathCostPolicy) int {
	cost := len(plan.FinalWalk) * policy.moveCost
	if len(plan.Pushes) > 0 && !strengthActive {
		cost += policy.strengthCost
	}
	for _, push := range plan.Pushes {
		cost += len(push.Walk) * policy.moveCost
		cost += policy.moveCost
	}
	return cost
}

func strengthPlanBeatsFieldPath(fieldCost fieldPathCost, plan world.PushPlan, strengthActive bool, policy fieldPathCostPolicy) bool {
	return policy.weighted && len(plan.Pushes) > 0 &&
		weightedStrengthPlanCost(plan, strengthActive, policy) < fieldCost.weighted
}

// preferLocalStrengthRoute asks whether a currently executable Strength route
// is cheaper than the already-planned Cut/Surf/walk route. It deliberately
// declines roster repair: fetching/catching a carrier is not represented in
// this local estimate and therefore must not masquerade as a cheap shortcut.
func preferLocalStrengthRoute(m *emu.Emu, romData []byte, h rom.MapHeader, dest Destination, fieldCost fieldPathCost) (bool, error) {
	policy := fieldPathCostPolicyFor(m)
	if !policy.weighted {
		return false, nil
	}
	plan, needed, err := currentLocalStrengthPlan(m, romData, h, dest)
	if err != nil || !needed {
		return false, err
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	capability := FieldCapabilityFor(&mem, FieldStrength)
	if !capability.Usable && !CanPrepareFieldMove(romData, &mem, FieldStrength) {
		return false, nil
	}
	fieldActions, err := fieldActionDecoderFor(m)
	if err != nil {
		return false, err
	}
	strengthActive := fieldActions.DecodeFieldAction(m).StrengthActive
	return strengthPlanBeatsFieldPath(fieldCost, plan, strengthActive, policy), nil
}

// solveLocalStrengthPath repairs the Strength carrier only after the push
// solver has proved a boulder must move. Current-party compatible Pokémon are
// auto-taught by UseFieldMove. With a Travel policy, a missing carrier is
// repaired through the existing party -> PC -> catch pipeline. If that repair
// moves the player to another map, moved=true tells GoTo to discard all local
// geometry and re-plan the original journey from the new live state.
func solveLocalStrengthPath(m *emu.Emu, romData []byte, policy MovePolicy, h rom.MapHeader, dest Destination) (moved bool, err error) {
	_, needed, err := currentLocalStrengthPlan(m, romData, h, dest)
	if err != nil || !needed {
		return false, err
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	capability := FieldCapabilityFor(&mem, FieldStrength)
	if !capability.Usable && !CanPrepareFieldMove(romData, &mem, FieldStrength) {
		if !capability.BadgeOwned || !capability.HMOwned {
			return false, missingFieldRosterPrerequisite(capability)
		}
		if policy == nil {
			return false, fmt.Errorf("%w: Strength route needs a compatible current-party carrier; Travel can repair from PC/catch", ErrFieldMovePrerequisite)
		}
		beforeMap := m.Peek8(sym.CurMap)
		if err := RepairFieldCapabilities(m, romData, policy, []FieldMove{FieldStrength}); err != nil {
			return false, fmt.Errorf("skill: GoTo: repair Strength carrier for boulder route: %w", err)
		}
		if m.Peek8(sym.CurMap) != beforeMap {
			return true, nil
		}
	}

	spec := localStrengthPuzzleSpec(m, h, dest)
	// Generic navigation must not consume battles/dialogue internally. A nil
	// solver policy bubbles those interruptions back to Travel, preserving its
	// engagement budgets and ownership contract; policy above is used only for
	// deliberate roster repair.
	if _, err := SolveBoulderPuzzle(m, romData, nil, spec); err != nil {
		return false, fmt.Errorf("skill: GoTo: solve local Strength route on map %02x: %w", h.ID, err)
	}
	return false, nil
}

func SolveBoulderPuzzle(m *emu.Emu, romData []byte, policy MovePolicy, spec BoulderPuzzleSpec) (BoulderPuzzleResult, error) {
	if err := validateBoulderPuzzleSpec(spec); err != nil {
		return BoulderPuzzleResult{}, err
	}
	pushLimit := spec.MaxPushes
	if pushLimit <= 0 {
		pushLimit = defaultBoulderPuzzlePushLimit
	}
	replanLimit := spec.MaxReplans
	if replanLimit <= 0 {
		replanLimit = defaultBoulderPuzzleReplanLimit
	}

	// The solver is the interruptible action. A wild encounter can roll on
	// any step (walking to a stand, the push itself, or just before Face) and
	// a sighted trainer or sign can open a box. The shared runner resolves
	// that interruption and re-enters the solver, which re-observes the
	// puzzle and re-plans from live state; it never resumes a stale push.
	// Counters live in result, so push/replan limits span re-entries.
	result := BoulderPuzzleResult{}
	_, err := RunInterruptible(m, policy, InterruptibleAction{
		Name:           "boulder puzzle",
		MaxEngagements: boulderPuzzleMaxEngagements,
		Run: func() error {
			return solveBoulderPuzzleFromLive(m, romData, spec, pushLimit, replanLimit, &result)
		},
	})
	return result, err
}

// boulderPuzzleMaxEngagements bounds battles resolved during one puzzle. It
// matches the replan limit, which bounded interruption handling before the
// solver moved onto the shared runner.
const boulderPuzzleMaxEngagements = defaultBoulderPuzzleReplanLimit

// solveBoulderPuzzleFromLive runs the observe/plan/push loop from the current
// live state until the puzzle's positive goal holds, a limit is reached, or
// an interruption must be resolved by the caller.
func solveBoulderPuzzleFromLive(m *emu.Emu, romData []byte, spec BoulderPuzzleSpec, pushLimit, replanLimit int, result *BoulderPuzzleResult) error {
	for result.Pushes < pushLimit && result.Replans < replanLimit {
		if m.Peek8(sym.IsInBattle) != 0 {
			return ErrBattleInterrupted
		}
		puzzle, mem, err := currentBoulderPuzzle(m, romData, spec)
		if err != nil {
			return err
		}
		if boulderPuzzleEventComplete(mem, spec) {
			return nil
		}

		plan, err := world.PlanPushPuzzle(puzzle)
		if err != nil {
			return fmt.Errorf("skill: boulder puzzle map %#02x from player (%d,%d), boulders=%v: %w", spec.Map, puzzle.Player.X, puzzle.Player.Y, puzzle.Movables, err)
		}
		result.Replans++
		result.Explored += plan.Explored
		if len(plan.Pushes) == 0 {
			// The pure goal is already true (target occupied / exit reachable).
			// Give a map script a short deterministic window to expose its event
			// fact, but do not require one when the caller did not configure it.
			if spec.HasCompleteEvent {
				for spent := 0; spent < boulderPushObserveBudget; spent += 10 {
					state.Snapshot(m, mem)
					if boulderPuzzleEventComplete(mem, spec) {
						return nil
					}
					m.StepFrames(10)
				}
				return fmt.Errorf("skill: boulder puzzle geometry goal is satisfied but completion event %d is not set", spec.CompleteEvent)
			}
			return nil
		}

		pushed, err := executeObservedBoulderPush(m, spec, plan.Pushes[0])
		if err != nil {
			return err
		}
		if pushed {
			result.Pushes++
		}
		// The next iteration observes the world again and solves from that
		// truth, whether or not the planned push landed.
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if result.Replans >= replanLimit {
		return fmt.Errorf("%w: map %#02x after %d replans and %d verified pushes, player=(%d,%d), boulders=%v", ErrBoulderPuzzleReplanLimit, spec.Map, result.Replans, result.Pushes, mem.U8(sym.XCoord), mem.U8(sym.YCoord), state.DecodeBoulders(&mem))
	}
	return fmt.Errorf("%w: map %#02x after %d verified pushes, player=(%d,%d), boulders=%v", ErrBoulderPuzzlePushLimit, spec.Map, result.Pushes, mem.U8(sym.XCoord), mem.U8(sym.YCoord), state.DecodeBoulders(&mem))
}
