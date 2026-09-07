package skill

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// ErrCutsceneTimeout is returned when a scripted sequence has not finished
// within the frame budget.
var ErrCutsceneTimeout = errors.New("skill: cutscene did not finish in budget")

// pokemonNicknamePrompt reports whether the live two-option question is the
// ROM's AskName prompt for a newly acquired Pokemon. This is deliberately
// text-gated rather than treating every TWO_OPTION_MENU alike: many story
// cutscenes intentionally rely on the default YES choice, while nicknaming is
// cosmetic and should never divert autonomous runs into the naming keyboard.
func pokemonNicknamePrompt(mem *state.Mem) bool {
	return state.DecodeTwoOptionMenu(mem) != nil && nicknamePromptOnScreen(mem)
}

// nicknamePromptOnScreen reports that AskName's question text is drawn. It is
// NOT sufficient on its own: the question stays on the tilemap after it has
// been answered, so a loop that refuses to press A on this alone deadlocks
// (MEASURED: GetStarter then times out at 10000 frames with the box still up).
// Pair it with a decodable two-option menu, which is the moment a press is
// actually a choice.
func nicknamePromptOnScreen(mem *state.Mem) bool {
	return strings.Contains(state.ScreenText(mem), "give a nickname")
}

// declineNickname answers AskName with NO so the Pokemon keeps its species
// name. It reports whether the prompt was actually answered; a caller that
// gets false must NOT fall back to pressing A, because A is the YES that
// opens the naming keyboard.
//
// The blind-A path is what produced a party of Pokemon literally named
// "AAAAAAAAAA": one A opens the keyboard with the cursor on 'A', and every
// subsequent A of the same advancing loop types another 'A' until the name is
// full and confirmed. MEASURED in TestStarterKeepsSpeciesName (CHARMANDER ->
// "AAAAAAAAAA") and live on a bought MAGIKARP.
func declineNickname(m frameClock, mem *state.Mem) bool {
	e, ok := m.(*emu.Emu)
	if !ok || state.DecodeTwoOptionMenu(mem) == nil {
		return false
	}
	return selectTwoOption(e, 1) == nil // 1 = NO
}

// Cutscene lets a scripted sequence run to completion. It presses A only
// to advance text boxes and never presses a direction, because the game
// is driving. It returns when done() holds and the player is controllable
// again, or ErrCutsceneTimeout when the frame budget is exhausted.
//
// Pokemon nickname prompts are the one special case: AskName defaults to YES,
// which would send a blind-A cutscene driver into the naming keyboard. They are
// recognized explicitly and answered NO so the Pokemon keeps its species name.
// Other two-option story prompts keep their existing behavior unchanged.
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
		if pokemonNicknamePrompt(&mem) {
			if err := selectTwoOption(m, 1); err != nil {
				return fmt.Errorf("skill: cutscene: decline Pokemon nickname: %w", err)
			}
			spent += 10
			continue
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
