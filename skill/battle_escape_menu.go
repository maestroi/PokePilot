package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

func battleEscapeMenuDecoderFor(m *emu.Emu) (game.BattleEscapeMenuDecoder, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: battle escape menu: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: battle escape menu: detect profile: %w", err)
	}
	decoder, ok := profile.(game.BattleEscapeMenuDecoder)
	if !ok {
		return nil, fmt.Errorf("skill: battle escape menu: profile %s@%s does not expose escape-menu semantics", profile.ID(), profile.Revision())
	}
	return decoder, nil
}

// selectBattleEscapeRunWithDecoder moves a live RUN-capable battle menu to RUN
// without assuming native cursor columns. It does not confirm the action.
func selectBattleEscapeRunWithDecoder(m menuMachine, decoder game.BattleEscapeMenuDecoder) error {
	if decoder == nil {
		return fmt.Errorf("skill: battle escape menu: nil decoder")
	}
	state := decoder.DecodeBattleEscapeMenu(m)
	if !state.Visible {
		return fmt.Errorf("skill: battle escape menu is not visible")
	}
	target, ok := decoder.BattleEscapeRunPosition(state.Kind)
	if !ok {
		return fmt.Errorf("skill: battle escape menu kind %q has no RUN entry", state.Kind)
	}

	const attempts = 12
	for i := 0; i < attempts; i++ {
		state = decoder.DecodeBattleEscapeMenu(m)
		if !state.Visible {
			return fmt.Errorf("skill: battle escape menu disappeared before RUN was selected")
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
			next := decoder.DecodeBattleEscapeMenu(m)
			return next.Visible && next.Kind == state.Kind && next.Cursor != previous
		}) {
			return fmt.Errorf(
				"skill: battle escape cursor stuck at col=%d row=%d, want RUN at col=%d row=%d: %w",
				previous.Column, previous.Row, target.Column, target.Row, ErrMenuStuck,
			)
		}
	}
	final := decoder.DecodeBattleEscapeMenu(m)
	return fmt.Errorf(
		"skill: battle escape cursor ended at col=%d row=%d, want RUN at col=%d row=%d: %w",
		final.Cursor.Column, final.Cursor.Row, target.Column, target.Row, ErrMenuStuck,
	)
}
