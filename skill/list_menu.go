package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

func listMenuDecoderFor(m *emu.Emu) (game.ListMenuDecoder, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: list menu: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: list menu: detect profile: %w", err)
	}
	decoder, ok := profile.(game.ListMenuDecoder)
	if !ok {
		return nil, fmt.Errorf("skill: list menu: profile %s@%s does not expose list-menu semantics", profile.ID(), profile.Revision())
	}
	return decoder, nil
}

// selectScrollingListEntry drives one open scrolling list to an absolute entry
// index and presses A. The profile owns how cursor and scroll offset combine.
func selectScrollingListEntry(m *emu.Emu, index int) error {
	decoder, err := listMenuDecoderFor(m)
	if err != nil {
		return err
	}
	return selectScrollingListEntryWithDecoder(m, decoder, index)
}

func selectScrollingListEntryWithDecoder(m menuMachine, decoder game.ListMenuDecoder, index int) error {
	if decoder == nil {
		return fmt.Errorf("skill: list menu: nil decoder")
	}
	if index < 0 {
		return fmt.Errorf("skill: list menu: negative index %d", index)
	}

	// A caller can observe a list during its draw transition. Preserve the
	// existing bag/mart behavior and give the controller one settle window
	// before reading or confirming the cursor.
	m.StepFrames(talkSettle)

	state := decoder.DecodeListMenu(m)
	if !state.Visible {
		return fmt.Errorf("skill: list menu is not visible")
	}

	const stuckLimit = 8
	stuck := 0
	for state.Position != index {
		previous := state.Position
		btn := emu.Down
		if previous > index {
			btn = emu.Up
		}
		m.Tap(btn, 3, 7)
		if !waitMenuUntil(m, menuSettleFrames, func() bool {
			next := decoder.DecodeListMenu(m)
			return next.Visible && next.Position != previous
		}) {
			stuck++
			if stuck >= stuckLimit {
				current := decoder.DecodeListMenu(m)
				return fmt.Errorf(
					"skill: list menu cursor stuck at entry %d, wanted %d, %d consecutive taps without movement: %w",
					current.Position, index, stuck, ErrMenuStuck,
				)
			}
		} else {
			stuck = 0
		}
		state = decoder.DecodeListMenu(m)
		if !state.Visible {
			return fmt.Errorf("skill: list menu disappeared before reaching entry %d", index)
		}
	}

	m.Tap(emu.A, 3, 7)
	return nil
}
