package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// ErrCutsceneTimeout is returned when a scripted sequence has not finished
// within the frame budget.
var ErrCutsceneTimeout = errors.New("skill: cutscene did not finish in budget")

// Cutscene lets a scripted sequence run to completion. It presses A only
// to advance text boxes and never presses a direction, because the game
// is driving. It returns when done() holds and the player is controllable
// again, or ErrCutsceneTimeout when the frame budget is exhausted.
//
// The success predicate is positive, per DESIGN.md 3.2b: done() holds AND
// the player is controllable. Merely the absence of a text box is not
// enough, because a cutscene can be mid-animation with no box up.
func Cutscene(m *emu.Emu, budgetFrames int, done func(*state.Mem) bool) error {
	var mem state.Mem
	spent := 0
	for {
		state.Snapshot(m, &mem)
		if done(&mem) && state.Controllable(&mem) {
			return nil
		}
		if spent >= budgetFrames {
			break
		}
		if mem.U8(sym.FontLoaded) != 0 {
			// A text box is up: tap A to advance it. The hold/gap are the
			// defaults known to work on Pokemon Red.
			m.Tap(emu.A, 3, 7)
			spent += 10
		} else {
			// No text box: the game is driving (animation, a scripted walk,
			// a map transition). Just step and wait.
			m.StepFrame()
			spent++
		}
	}

	// Budget exhausted: report a failure that is diagnosable without a
	// screenshot. The map, coordinates, wJoyIgnore and wFontLoaded name the
	// exact state the cutscene was stuck in.
	state.Snapshot(m, &mem)
	return fmt.Errorf("skill: cutscene timed out after %d frames: map=%#04x at (%d,%d) wJoyIgnore=%#04x wFontLoaded=%#04x: %w",
		spent, mem.U8(sym.CurMap), mem.U8(sym.XCoord), mem.U8(sym.YCoord),
		mem.U8(sym.JoyIgnore), mem.U8(sym.FontLoaded), ErrCutsceneTimeout)
}
