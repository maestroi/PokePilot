package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

// ErrMenuStuck reports that the cursor would not move to the wanted index.
var ErrMenuStuck = errors.New("skill: menu cursor did not reach the target")

// menuSettleFrames bounds the wait for the cursor to move after a tap. The
// tap itself already steps 10 frames (hold 3, gap 7); this covers the menu's
// joypad poll and cursor redraw.
const menuSettleFrames = 30

// twoOptionConsumedFrames bounds the wait for an answered YES/NO prompt to
// leave the screen. The answer opens the script's next text box, which is
// drawn well inside this window; a prompt still up at the end of it is not
// slowness, it is an answer the game never took.
const twoOptionConsumedFrames = 120

// menuMachine is the small execution surface the generic menu driver needs.
// *emu.Emu satisfies it; deterministic tests can use a fake machine without
// importing any concrete game's RAM layout.
type menuMachine interface {
	game.MemoryReader
	StepFrame()
	StepFrames(int)
	Tap(emu.Button, int, int)
}

func menuDecoderFor(m *emu.Emu) (game.MenuDecoder, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: menu: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: menu: detect profile: %w", err)
	}
	decoder, ok := profile.(game.MenuDecoder)
	if !ok {
		return nil, fmt.Errorf("skill: menu: profile %s@%s does not expose menu semantics", profile.ID(), profile.Revision())
	}
	return decoder, nil
}

func waitMenuUntil(m menuMachine, budget int, pred func() bool) bool {
	for stepped := 0; stepped < budget; stepped++ {
		if pred() {
			return true
		}
		m.StepFrame()
	}
	return false
}

// SelectMenuItem moves the cursor to index and presses A. The public entrypoint
// resolves the active profile's semantic menu decoder; the navigation itself
// contains no game-specific RAM addresses or menu byte layout.
func SelectMenuItem(m *emu.Emu, index int) error {
	decoder, err := menuDecoderFor(m)
	if err != nil {
		return err
	}
	return selectMenuItemWithDecoder(m, decoder, index)
}

// selectMenuItemWithDecoder is the reusable step-and-verify cursor primitive.
// Max is inclusive: valid indices are 0..Max.
func selectMenuItemWithDecoder(m menuMachine, decoder game.MenuDecoder, index int) error {
	if decoder == nil {
		return fmt.Errorf("skill: SelectMenuItem: nil menu decoder")
	}

	// A just-opened menu can be visible before its input loop is polling.
	m.StepFrames(talkSettle)
	menu := decoder.DecodeMenuCursor(m)
	if index < 0 || index > menu.Max {
		return fmt.Errorf("skill: SelectMenuItem: index %d out of range for menu with max %d", index, menu.Max)
	}

	const stuckLimit = 5
	stuck := 0
	for menu.Current != index {
		previous := menu.Current
		btn := emu.Down
		if previous > index {
			btn = emu.Up
		}
		m.Tap(btn, 3, 7)
		if !waitMenuUntil(m, menuSettleFrames, func() bool {
			return decoder.DecodeMenuCursor(m).Current != previous
		}) {
			stuck++
			if stuck >= stuckLimit {
				cur := decoder.DecodeMenuCursor(m).Current
				return fmt.Errorf("skill: SelectMenuItem: cursor stuck at %d, wanted %d (max %d), %d consecutive taps without movement: %w",
					cur, index, menu.Max, stuck, ErrMenuStuck)
			}
		} else {
			stuck = 0
		}
		menu = decoder.DecodeMenuCursor(m)
	}

	m.Tap(emu.A, 3, 7)
	return nil
}

// selectTwoOption selects option 0 or 1 from a live two-option prompt.
func selectTwoOption(m *emu.Emu, index int) error {
	decoder, err := menuDecoderFor(m)
	if err != nil {
		return err
	}
	return selectTwoOptionWithDecoder(m, decoder, index)
}

func selectTwoOptionWithDecoder(m menuMachine, decoder game.MenuDecoder, index int) error {
	if index < 0 || index > 1 {
		return fmt.Errorf("skill: selectTwoOption: index %d out of range 0..1", index)
	}
	if decoder == nil {
		return fmt.Errorf("skill: selectTwoOption: nil menu decoder")
	}

	// Prompt state can become visible a few frames before input is accepted.
	m.StepFrames(talkSettle)

	prompt, ok := decoder.DecodeTwoOption(m)
	if !ok {
		return errors.New("skill: selectTwoOption: no two-option prompt is open")
	}

	const stuckLimit = 5
	stuck := 0
	current := prompt.Current
	for current != index {
		previous := current
		btn := emu.Down
		if previous > index {
			btn = emu.Up
		}
		m.Tap(btn, 3, 7)
		if !waitMenuUntil(m, menuSettleFrames, func() bool {
			return decoder.DecodeMenuCursor(m).Current != previous
		}) {
			stuck++
			if stuck >= stuckLimit {
				return fmt.Errorf("skill: selectTwoOption: cursor stuck at %d, wanted %d: %w", previous, index, ErrMenuStuck)
			}
		} else {
			stuck = 0
		}
		current = decoder.DecodeMenuCursor(m).Current
		if current < 0 || current > 1 {
			return fmt.Errorf("skill: selectTwoOption: cursor moved out of range to %d", current)
		}
	}

	m.Tap(emu.A, 3, 7)
	// Positive postcondition: the profile no longer observes the prompt.
	if !waitMenuUntil(m, twoOptionConsumedFrames, func() bool {
		_, open := decoder.DecodeTwoOption(m)
		return !open
	}) {
		return fmt.Errorf("skill: selectTwoOption: prompt still open %d frames after answering %d", twoOptionConsumedFrames, index)
	}
	return nil
}
