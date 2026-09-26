package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

func battleMenuDecoderFor(m *emu.Emu) (game.BattleMenuDecoder, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: battle menu: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: battle menu: detect profile: %w", err)
	}
	decoder, ok := profile.(game.BattleMenuDecoder)
	if !ok {
		return nil, fmt.Errorf("skill: battle menu: profile %s@%s does not expose battle-menu semantics", profile.ID(), profile.Revision())
	}
	return decoder, nil
}

// selectBattleMainMenuEntry moves the ordinary turn-action cursor to a
// semantic entry. It does not press A; callers retain ownership of the action.
func selectBattleMainMenuEntry(m *emu.Emu, entry game.BattleMenuEntry) error {
	decoder, err := battleMenuDecoderFor(m)
	if err != nil {
		return err
	}
	return selectBattleMainMenuEntryWithDecoder(m, decoder, entry)
}

func selectBattleMainMenuEntryWithDecoder(m menuMachine, decoder game.BattleMenuDecoder, entry game.BattleMenuEntry) error {
	if decoder == nil {
		return fmt.Errorf("skill: battle menu: nil decoder")
	}
	target, ok := decoder.BattleMainMenuEntryPosition(entry)
	if !ok {
		return fmt.Errorf("skill: battle menu entry %q is not available", entry)
	}

	const attempts = 12
	for i := 0; i < attempts; i++ {
		state := decoder.DecodeBattleMainMenu(m)
		if !state.Visible {
			return fmt.Errorf("skill: battle main menu is not visible")
		}
		if state.Cursor == target {
			return nil
		}

		previous := state.Cursor
		var btn emu.Button
		switch {
		case previous.Column < target.Column:
			btn = emu.Right
		case previous.Column > target.Column:
			btn = emu.Left
		case previous.Row < target.Row:
			btn = emu.Down
		case previous.Row > target.Row:
			btn = emu.Up
		default:
			return nil
		}

		m.Tap(btn, 3, 7)
		if !waitMenuUntil(m, menuSettleFrames, func() bool {
			next := decoder.DecodeBattleMainMenu(m)
			return next.Visible && next.Cursor != previous
		}) {
			return fmt.Errorf(
				"skill: battle main menu cursor stuck at col=%d row=%d, want %q at col=%d row=%d: %w",
				previous.Column, previous.Row, entry, target.Column, target.Row, ErrMenuStuck,
			)
		}
	}

	final := decoder.DecodeBattleMainMenu(m)
	return fmt.Errorf(
		"skill: battle main menu cursor ended at col=%d row=%d, want %q at col=%d row=%d: %w",
		final.Cursor.Column, final.Cursor.Row, entry, target.Column, target.Row, ErrMenuStuck,
	)
}

const (
	battleMenuActivatePresses = 3
	battleMenuActivateSettle  = 60
)

// activateBattleMainMenuEntryWithDecoder selects entry and presses A until
// opened reports the submenu. The main menu is drawn before HandleMenuInput
// polls the joypad (PlaceMenuCursor, then Delay3), so an A that lands in that
// window is silently dropped: Lorelei's room left UseBattleMedicine waiting
// 500 frames on an unopened bag (run-z5ghf614serd8, triage:f6e0e5cee7099781).
// A is re-pressed only while the main menu is still up with the cursor on
// entry, which is exactly the dropped-press state, so a retry can never act
// inside the submenu.
func activateBattleMainMenuEntryWithDecoder(m menuMachine, decoder game.BattleMenuDecoder, entry game.BattleMenuEntry, budget int, opened func() bool) error {
	if err := selectBattleMainMenuEntryWithDecoder(m, decoder, entry); err != nil {
		return err
	}
	target, _ := decoder.BattleMainMenuEntryPosition(entry)
	for press := 0; press < battleMenuActivatePresses; press++ {
		m.Tap(emu.A, 3, 7)
		if waitMenuUntil(m, battleMenuActivateSettle, opened) {
			return nil
		}
		if live := decoder.DecodeBattleMainMenu(m); !live.Visible || live.Cursor != target {
			break // the press was taken; let the caller's budget decide
		}
	}
	if waitMenuUntil(m, budget, opened) {
		return nil
	}
	return fmt.Errorf("skill: battle menu %q did not open within %d frames: %w", entry, budget, ErrMenuStuck)
}
