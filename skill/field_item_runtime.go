package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

func fieldItemDecoderFor(m *emu.Emu) (game.FieldItemDecoder, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: field item: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: field item: detect profile: %w", err)
	}
	decoder, ok := profile.(game.FieldItemDecoder)
	if !ok {
		return nil, fmt.Errorf("skill: field item: profile %s@%s does not expose field-item semantics", profile.ID(), profile.Revision())
	}
	return decoder, nil
}

func fieldItemInventoryEntry(state game.InventoryState, item uint16) (int, int) {
	for i, it := range state.Items {
		if it.NativeItemID == item {
			return i, it.Quantity
		}
	}
	return -1, 0
}

func ppRestoreMoveSlotState(mon game.FieldItemPartyMon) (int, bool) {
	best := -1
	var bestPP uint8
	for i, move := range mon.Moves {
		if move == 0 {
			continue
		}
		if mon.PP[i] == 0 {
			return i, true
		}
		if best < 0 || mon.PP[i] < bestPP {
			best, bestPP = i, mon.PP[i]
		}
	}
	return best, best >= 0
}

func fieldItemHadEffectState(before, after game.FieldItemPartyMon) bool {
	if after.Level > before.Level || after.HP > before.HP {
		return true
	}
	if before.Status != "" && after.Status == "" {
		return true
	}
	for i := range before.PP {
		if after.PP[i] > before.PP[i] {
			return true
		}
	}
	return false
}

func selectFieldItemMoveSlot(m menuMachine, decoder game.FieldItemDecoder, slot int) error {
	if slot < 0 {
		return fmt.Errorf("skill: field item move slot: negative slot %d", slot)
	}
	const stuckLimit = 8
	stuck := 0
	for {
		live := decoder.DecodeFieldItem(m)
		if !live.MoveMenuVisible {
			return fmt.Errorf("skill: field item move menu is not visible")
		}
		if slot > live.MoveCursor.Max {
			return fmt.Errorf("skill: field item move slot %d out of range 0..%d", slot, live.MoveCursor.Max)
		}
		if live.MoveCursor.Current == slot {
			m.Tap(emu.A, 3, 7)
			return nil
		}
		before := live.MoveCursor.Current
		btn := emu.Down
		if before > slot {
			btn = emu.Up
		}
		m.Tap(btn, 3, 7)
		if !waitMenuUntil(m, menuSettleFrames, func() bool {
			next := decoder.DecodeFieldItem(m)
			return next.MoveMenuVisible && next.MoveCursor.Current != before
		}) {
			stuck++
			if stuck >= stuckLimit {
				return fmt.Errorf("skill: field item move cursor stuck at %d, want %d: %w", before, slot, ErrMenuStuck)
			}
		} else {
			stuck = 0
		}
	}
}
