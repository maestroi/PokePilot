package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// ErrBlocked reports that a step could not be taken.
type ErrBlocked struct {
	Step world.Step
	At   struct{ X, Y uint8 }
}

func (e *ErrBlocked) Error() string {
	return fmt.Sprintf("skill: step %s blocked at (%d,%d)", e.Step, e.At.X, e.At.Y)
}

// ErrBattleInterrupted and ErrDialogueInterrupted are returned by WalkPath
// when the game leaves the overworld mid-path. Callers decide what to do;
// WalkPath never presses A to dismiss anything.
var (
	ErrBattleInterrupted   = errors.New("skill: battle interrupted movement")
	ErrDialogueInterrupted = errors.New("skill: text box interrupted movement")
)

// ponytail: the 60/40-frame budgets below are empirical, measured on this
// ROM (one tile of movement, then the step animation settling). Tighten
// them only with a measurement, not a guess.
const (
	stepMoveBudget   = 60
	stepSettleBudget = 40
)

func buttonFor(s world.Step) (emu.Button, bool) {
	switch s {
	case world.StepUp:
		return emu.Up, true
	case world.StepDown:
		return emu.Down, true
	case world.StepLeft:
		return emu.Left, true
	case world.StepRight:
		return emu.Right, true
	}
	return 0, false
}

func playerXY(m *emu.Emu) (uint8, uint8) {
	return m.Peek8(sym.XCoord), m.Peek8(sym.YCoord)
}

// StepOnce attempts a single tile of movement. It returns nil when the
// player's tile coordinate actually changed in the requested direction.
//
// The completion predicate is the coordinate change, not WalkCounter:
// WalkCounter is already 0 before a step starts, so waiting on it alone
// exits after a few frames with the player still on the same tile. A
// direction press may also only turn the player in place, so a timed-out
// attempt is retried once before the step is treated as blocked.
func StepOnce(m *emu.Emu, s world.Step) error {
	btn, ok := buttonFor(s)
	if !ok {
		return fmt.Errorf("skill: invalid step %s", s)
	}

	startX, startY := playerXY(m)
	moved := false
	for attempt := 0; attempt < 2 && !moved; attempt++ {
		_, err := m.HoldUntil(btn, stepMoveBudget, func(m *emu.Emu) bool {
			x, y := playerXY(m)
			return x != startX || y != startY
		})
		if err == nil {
			moved = true
		}
	}
	if !moved {
		return &ErrBlocked{Step: s, At: struct{ X, Y uint8 }{startX, startY}}
	}

	if _, err := m.StepUntil(stepSettleBudget, func(m *emu.Emu) bool {
		return m.Peek8(sym.WalkCounter) == 0
	}); err != nil {
		return fmt.Errorf("skill: step %s: walk animation unsettled after %d frames", s, stepSettleBudget)
	}

	x, y := playerXY(m)
	if int(x) != int(startX)+s.DX || int(y) != int(startY)+s.DY {
		return &ErrBlocked{Step: s, At: struct{ X, Y uint8 }{x, y}}
	}
	return nil
}

// WalkPath executes each step in order, re-reading state after every step.
// It stops and returns an error if a step cannot be completed, if a battle
// starts, or if a text box opens.
//
// Movement never advances dialogue. After dialogue has interrupted
// movement, the recovery layer may press A only while ordinary text is
// active. It never answers a choice.
func WalkPath(m *emu.Emu, path []world.Step) error {
	var mem state.Mem
	for _, step := range path {
		stepErr := StepOnce(m, step)

		// Check for an interruption BEFORE trusting stepErr. A wild
		// encounter fires mid-step: the battle freezes the player, so
		// StepOnce times out and reports the tile as blocked. Reporting
		// that as collision sent a Route 1 investigation chasing a
		// pathfinding bug that did not exist — the tile was walkable and
		// a Pidgey was on the screen.
		state.Snapshot(m, &mem)
		if state.DecodeBattle(&mem) != nil {
			return ErrBattleInterrupted
		}
		if state.DecodeDialogue(&mem) != nil {
			return ErrDialogueInterrupted
		}
		if stepErr != nil {
			return stepErr
		}
	}
	return nil
}

// ponytail: maxWalkRetries and npcWaitFrames are knobs, not laws. Six
// STAGNANT retries covers a sprite that wanders across a corridor twice;
// a walk that gets closer to its target or learns a new call-local runtime
// blocker resets that budget. This distinction matters on long cave/grind
// paths: encountering several independent blockers is progress, not six
// repetitions of the same failure. npcWaitFrames is about one and a half NPC
// steps on this ROM. Tune with a measurement, not a guess.
const (
	maxWalkRetries = 6
	npcWaitFrames  = 48

	// Two misses at the same destination are enough to distinguish a
	// deterministic runtime blocker from the known one-snapshot sprite races
	// below. The learned blocker lives only for this walkAround call.
	unexplainedBlockLearnThreshold = 2
)

func blockedDestination(e *ErrBlocked) [2]int {
	return [2]int{int(e.At.X) + e.Step.DX, int(e.At.Y) + e.Step.DY}
}

// walkAround walks a planned path, re-planning around obstacles the static
// collision grid cannot know about: sprites stand in doorways and wander
// into gaps, and only walking into one discovers it.
//
// readBlocked returns the tiles live sprites occupy right now, decoded from
// a fresh RAM snapshot; it is called exactly once at the top of every
// attempt. plan must re-read the player's position, because a partially-walked
// path leaves them somewhere new. walk performs the steps. wait lets game
// time pass.
//
// Live sprite positions are ephemeral observations and are never cached.
// Separately, if real movement is blocked twice at the same destination while
// that tile is absent from the live-sprite snapshot, the destination is added
// to a call-local blocker set. That gives the pathfinder a bounded chance to
// route around deterministic runtime geometry without teaching permanent
// world state or special-casing a map coordinate. This is the failure shape
// seen in Mt. Moon 1F at (10,22)->(9,22): repeatedly retrying the identical
// static-grid path can never make progress, but another path may.
//
// The retry budget measures STAGNATION, not total re-plans. A long walk can
// legitimately encounter several distinct runtime blockers (or make partial
// progress before the next NPC crosses it). Learning a new blocker resets the
// budget, and a newly planned path shorter than every path since the last
// learned blocker does too. Six attempts that reveal nothing new still stop.
// This keeps unattended runs bounded without turning "four different cave
// tiles disagreed with the static grid" into a hard failure merely because
// each tile needed two confirmations.
//
// Two known sprite races are absorbed before a blocker is learned:
//
//   - The liveness filter is the sprite's IMAGEINDEX, which is the screen
//     overlay's state, so the snapshot is screen-local: a sprite that just
//     walked off the edge of the screen still decodes live on its last
//     tile until the overlay catches up, and one that just entered may not
//     be in it yet.
//   - TryWalking writes a walking NPC's DESTINATION tile at the start of
//     its 16-frame animation, so a sprite mid-step can straddle two tiles:
//     the snapshot may report the tile it is leaving, not the one it is
//     entering.
func walkAround(readBlocked func() map[[2]int]bool, plan func(blocked map[[2]int]bool) ([]world.Step, error), walk func([]world.Step) error, wait func()) error {
	unexplainedMisses := map[[2]int]int{}
	learnedBlocked := map[[2]int]bool{}
	stagnantRetries := 0
	bestPlanLen := -1

	for {
		liveBlocked := readBlocked()
		blocked := mergeBlockers(liveBlocked, learnedBlocked)
		steps, err := plan(blocked)
		if err != nil {
			// A plan error is retryable only when the fresh sprite snapshot
			// contains blockers that may move. Call-local learned blockers are
			// evidence from repeated failed movement, so do not spin waiting
			// for them to disappear.
			if len(liveBlocked) == 0 || stagnantRetries >= maxWalkRetries {
				return err
			}
			stagnantRetries++
			wait()
			continue
		}

		// walk may have completed a prefix before the previous collision.
		// A shorter remaining plan is direct evidence that the call moved
		// closer to its destination, so it gets a fresh stall budget.
		if bestPlanLen < 0 || len(steps) < bestPlanLen {
			bestPlanLen = len(steps)
			stagnantRetries = 0
		}

		if err := walk(steps); err != nil {
			var eb *ErrBlocked
			if !errors.As(err, &eb) {
				return err
			}

			target := blockedDestination(eb)
			learnedNow := false
			if liveBlocked[target] {
				// This was explained by the snapshot we planned from. Never
				// turn an observed sprite position into learned geometry.
				delete(unexplainedMisses, target)
			} else {
				unexplainedMisses[target]++
				if unexplainedMisses[target] >= unexplainedBlockLearnThreshold && !learnedBlocked[target] {
					learnedBlocked[target] = true
					learnedNow = true
				}
			}

			if learnedNow {
				// New runtime evidence means the next plan is meaningfully
				// different. Its path may be longer than the old one, so reset
				// both the stall count and the path-length baseline.
				stagnantRetries = 0
				bestPlanLen = -1
			} else {
				stagnantRetries++
				if stagnantRetries > maxWalkRetries {
					return err
				}
			}

			wait() // give a wandering sprite time to move on; the next
			// attempt reads a fresh snapshot and may also avoid a repeatedly
			// unexplained destination for the remainder of this call
			continue
		}
		return nil
	}
}
