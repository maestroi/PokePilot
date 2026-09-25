package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

func partyMenuDecoderFor(m *emu.Emu) (game.PartyMenuDecoder, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: party menu: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: party menu: detect profile: %w", err)
	}
	decoder, ok := profile.(game.PartyMenuDecoder)
	if !ok {
		return nil, fmt.Errorf("skill: party menu: profile %s@%s does not expose party-menu semantics", profile.ID(), profile.Revision())
	}
	return decoder, nil
}

// SelectPartySlot moves a visible party-menu cursor to index and confirms it.
// The active profile owns cursor decoding and menu identity. The selection is
// complete when input ownership leaves the party list; concrete overlays such
// as Red's SWITCH/STATS/CANCEL box are hidden behind that semantic boundary.
func SelectPartySlot(m *emu.Emu, index int) error {
	decoder, err := partyMenuDecoderFor(m)
	if err != nil {
		return err
	}
	return selectPartySlotWithDecoder(m, decoder, index)
}

func selectPartySlotWithDecoder(m menuMachine, decoder game.PartyMenuDecoder, index int) error {
	if decoder == nil {
		return fmt.Errorf("skill: SelectPartySlot: nil party-menu decoder")
	}
	state := decoder.DecodePartyMenu(m)
	if !state.Visible {
		return fmt.Errorf("skill: SelectPartySlot: no party menu is visible")
	}
	if index < 0 || index > state.Cursor.Max {
		return fmt.Errorf("skill: SelectPartySlot: index %d out of range for party menu with max %d", index, state.Cursor.Max)
	}

	const stuckLimit = 5
	stuck := 0
	for state.Cursor.Current != index {
		previous := state.Cursor.Current
		btn := emu.Down
		if previous > index {
			btn = emu.Up
		}
		m.Tap(btn, 3, 7)
		if !waitMenuUntil(m, menuSettleFrames, func() bool {
			next := decoder.DecodePartyMenu(m)
			return next.Visible && next.Cursor.Current != previous
		}) {
			stuck++
			if stuck >= stuckLimit {
				current := decoder.DecodePartyMenu(m)
				return fmt.Errorf(
					"skill: SelectPartySlot: cursor stuck at %d, wanted %d (max %d), %d consecutive taps without movement: %w",
					current.Cursor.Current, index, state.Cursor.Max, stuck, ErrMenuStuck,
				)
			}
		} else {
			stuck = 0
		}
		state = decoder.DecodePartyMenu(m)
		if !state.Visible {
			return fmt.Errorf("skill: SelectPartySlot: party menu disappeared before reaching slot %d", index)
		}
	}

	for i := 0; i < 24; i++ {
		if !decoder.DecodePartyMenu(m).Visible {
			return nil
		}
		m.Tap(emu.A, 3, 7)
		if waitMenuUntil(m, 25, func() bool {
			return !decoder.DecodePartyMenu(m).Visible
		}) {
			return nil
		}
	}
	final := decoder.DecodePartyMenu(m)
	return fmt.Errorf("skill: SelectPartySlot: party menu %q still up at slot %d after selecting slot %d",
		final.Kind, final.Cursor.Current, index)
}
