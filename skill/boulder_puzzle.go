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
	Map              uint8
	Targets          []world.Point
	Reachable        *world.Point
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
	return world.PushPuzzle{
		Grid:      grid,
		Player:    world.Point{X: int(player.X), Y: int(player.Y)},
		Movables:  liveBoulderMovables(&mem),
		Fixed:     liveNonBoulderBlockers(&mem),
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

// resolveBoulderWalkInterruption handles only interruption cleanup and then
// asks the outer solver to re-plan. It never resumes a stale planned path.
func resolveBoulderWalkInterruption(m *emu.Emu, policy MovePolicy, err error) error {
	switch {
	case errors.Is(err, ErrBattleInterrupted):
		resolution, battleErr := fleeThenFight(m, policy, 3)()
		if battleErr != nil {
			return fmt.Errorf("skill: boulder puzzle resolve battle: %w", battleErr)
		}
		if resolution.outcome == state.ResultLost {
			return battleBlackoutError(resolution)
		}
		return nil
	case errors.Is(err, ErrDialogueInterrupted):
		recovery := RecoverDialogue(m, dialogueRecoveryBudget)
		switch recovery.Stop {
		case DialogueRecovered:
			return nil
		case DialogueUnexpectedMode:
			resolution, battleErr := fleeThenFight(m, policy, 3)()
			if battleErr != nil {
				return fmt.Errorf("skill: boulder puzzle resolve dialogue-led battle: %w", battleErr)
			}
			if resolution.outcome == state.ResultLost {
				return battleBlackoutError(resolution)
			}
			return nil
		case DialogueChoiceRequired:
			return fmt.Errorf("skill: boulder puzzle dialogue requires an unanswered choice: %q", recovery.Text)
		case DialogueMenuOpen:
			return fmt.Errorf("skill: boulder puzzle dialogue recovery found an open menu: %q", recovery.Text)
		default:
			return fmt.Errorf("skill: boulder puzzle dialogue did not recover within budget: %q", recovery.Text)
		}
	default:
		return err
	}
}

func executeObservedBoulderPush(m *emu.Emu, spec BoulderPuzzleSpec, policy MovePolicy, push world.Push) (bool, error) {
	if err := WalkPath(m, push.Walk); err != nil {
		if errors.Is(err, ErrBattleInterrupted) || errors.Is(err, ErrDialogueInterrupted) {
			if err := resolveBoulderWalkInterruption(m, policy, err); err != nil {
				return false, err
			}
			return false, nil // world changed while walking: re-plan before pushing
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
		return false, fmt.Errorf("skill: boulder puzzle face slot %d at (%d,%d): %w", push.MovableID, push.From.X, push.From.Y, err)
	}
	state.Snapshot(m, &before)
	if before.U8(sym.StatusFlags1)&fieldStrengthActiveBit == 0 {
		if _, err := UseFieldMove(m, FieldStrength); err != nil {
			return false, fmt.Errorf("skill: boulder puzzle activate Strength for slot %d: %w", push.MovableID, err)
		}
	}

	if err := StepOnce(m, push.Direction); err != nil {
		var blocked *ErrBlocked
		if errors.As(err, &blocked) {
			return false, nil // destination changed after planning; observe again
		}
		return false, fmt.Errorf("skill: boulder puzzle push slot %d %s from (%d,%d): %w", push.MovableID, push.Direction, push.From.X, push.From.Y, err)
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
func SolveBoulderPuzzle(m *emu.Emu, romData []byte, policy MovePolicy, spec BoulderPuzzleSpec) (BoulderPuzzleResult, error) {
	if policy == nil {
		return BoulderPuzzleResult{}, fmt.Errorf("skill: boulder puzzle: nil move policy")
	}
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

	result := BoulderPuzzleResult{}
	for result.Pushes < pushLimit && result.Replans < replanLimit {
		puzzle, mem, err := currentBoulderPuzzle(m, romData, spec)
		if err != nil {
			return result, err
		}
		if boulderPuzzleEventComplete(mem, spec) {
			return result, nil
		}

		plan, err := world.PlanPushPuzzle(puzzle)
		if err != nil {
			return result, fmt.Errorf("skill: boulder puzzle map %#02x from player (%d,%d), boulders=%v: %w", spec.Map, puzzle.Player.X, puzzle.Player.Y, puzzle.Movables, err)
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
						return result, nil
					}
					m.StepFrames(10)
				}
				return result, fmt.Errorf("skill: boulder puzzle geometry goal is satisfied but completion event %d is not set", spec.CompleteEvent)
			}
			return result, nil
		}

		pushed, err := executeObservedBoulderPush(m, spec, policy, plan.Pushes[0])
		if err != nil {
			return result, err
		}
		if pushed {
			result.Pushes++
		}
		// Whether a push happened or an interruption was resolved, the next
		// iteration observes the world again and solves from that truth.
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	if result.Replans >= replanLimit {
		return result, fmt.Errorf("%w: map %#02x after %d replans and %d verified pushes, player=(%d,%d), boulders=%v", ErrBoulderPuzzleReplanLimit, spec.Map, result.Replans, result.Pushes, mem.U8(sym.XCoord), mem.U8(sym.YCoord), state.DecodeBoulders(&mem))
	}
	return result, fmt.Errorf("%w: map %#02x after %d verified pushes, player=(%d,%d), boulders=%v", ErrBoulderPuzzlePushLimit, spec.Map, result.Pushes, mem.U8(sym.XCoord), mem.U8(sym.YCoord), state.DecodeBoulders(&mem))
}
