package skill

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	hm01Item       uint8 = 0xC4
	cutMove        uint8 = 0x0F
	cutFieldMove   uint8 = 1
	cutTreeTile    uint8 = 0x3D
	gymCutTreeTile uint8 = 0x50
	vermilionCity  uint8 = 0x05
	cutMenuBudget        = 4000
)

func monKnowsMove(mon state.Mon, move uint8) bool {
	for _, id := range mon.Moves {
		if id == move {
			return true
		}
	}
	return false
}

func partyMoveSlot(mem *state.Mem, move uint8) int {
	for i, mon := range state.DecodeParty(mem).Mons {
		if monKnowsMove(mon, move) {
			return i
		}
	}
	return -1
}

func cutScreenHas(m *emu.Emu, marker string) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return strings.Contains(state.ScreenText(&mem), marker)
}

func tmhmPartyMenuUp(m *emu.Emu) bool   { return cutScreenHas(m, "Use TM") }
func normalPartyMenuUp(m *emu.Emu) bool { return cutScreenHas(m, "Choose") }
func fieldMoveMenuUp(m *emu.Emu) bool {
	return cutScreenHas(m, "STATS") && m.Peek8(sym.FieldMoves) != 0
}

func movePartyCursor(m *emu.Emu, index int) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	count := int(state.DecodeParty(&mem).Count)
	if index < 0 || index >= count {
		return fmt.Errorf("skill: party slot %d out of range for party of %d", index, count)
	}
	for i := 0; i < 60; i++ {
		cur := int(m.Peek8(sym.CurrentMenuItem))
		if cur == index {
			return nil
		}
		btn := emu.Down
		if cur > index {
			btn = emu.Up
		}
		m.Tap(btn, 3, 7)
		_, _ = m.StepUntil(menuSettleFrames, func(m *emu.Emu) bool {
			return int(m.Peek8(sym.CurrentMenuItem)) != cur
		})
	}
	return fmt.Errorf("skill: party cursor at %d, want %d", m.Peek8(sym.CurrentMenuItem), index)
}

func selectTMHMPartySlot(m *emu.Emu, index int) error {
	if err := movePartyCursor(m, index); err != nil {
		return err
	}
	for i := 0; i < 24 && tmhmPartyMenuUp(m); i++ {
		m.Tap(emu.A, 3, 7)
		_, _ = m.StepUntil(25, func(m *emu.Emu) bool { return !tmhmPartyMenuUp(m) })
	}
	if tmhmPartyMenuUp(m) {
		return fmt.Errorf("skill: TM/HM party menu still up after selecting slot %d", index)
	}
	return nil
}

func selectFieldMoveUser(m *emu.Emu, index int) error {
	if err := movePartyCursor(m, index); err != nil {
		return err
	}
	for i := 0; i < 24 && !fieldMoveMenuUp(m); i++ {
		m.Tap(emu.A, 3, 7)
		_, _ = m.StepUntil(25, fieldMoveMenuUp)
	}
	if !fieldMoveMenuUp(m) {
		return fmt.Errorf("skill: field-move menu did not appear after selecting slot %d", index)
	}
	return nil
}

func openStartMenuEntry(m *emu.Emu, entry int) error {
	if err := waitForStartMenu(m); err != nil {
		return err
	}
	return SelectMenuItem(m, entry)
}

func closeToOverworld(m *emu.Emu) error {
	var mem state.Mem
	for i := 0; i < 80; i++ {
		state.Snapshot(m, &mem)
		if state.Controllable(&mem) && mem.U8(sym.FontLoaded) == 0 {
			return nil
		}
		m.Tap(emu.B, 3, 7)
		m.StepFrames(20)
	}
	state.Snapshot(m, &mem)
	return fmt.Errorf("skill: menus did not close to overworld: screen=%q", state.ScreenText(&mem))
}

// TeachCut is the compatibility entry point for existing story code. The
// shared field-action framework owns prerequisite checks and generic HM
// teaching now, so Cut cannot drift from Surf/Strength/Flash semantics.
func TeachCut(m *emu.Emu) (int, error) {
	return EnsureFieldMove(m, FieldCut)
}

// finishTeachingCut is retained for older measurement/tests that exercise the
// historical menu path directly. Production teaching goes through TeachTMHM
// via EnsureFieldMove.
func finishTeachingCut(m *emu.Emu, slot int, before [4]uint8) (bool, error) {
	tried := map[uint8]bool{}
	lastForget := -1
	var mem state.Mem
	for frames := 0; frames < cutMenuBudget; frames += 20 {
		state.Snapshot(m, &mem)
		if partyMoveSlot(&mem, cutMove) == slot {
			return true, nil
		}
		text := state.ScreenText(&mem)
		switch {
		case strings.Contains(text, "not compatible"):
			for i := 0; i < 30 && !tmhmPartyMenuUp(m); i++ {
				m.Tap(emu.A, 3, 7)
				m.StepFrames(20)
			}
			return false, nil
		case forgetMenuUp(m):
			if lastForget >= 0 {
				tried[before[lastForget]] = true
				lastForget = -1
			}
			pick := forgetSlot(m.ROM(), before, tried)
			if pick < 0 {
				return false, fmt.Errorf("skill: TeachCut: slot %d has no move that can be replaced", slot)
			}
			if err := selectForgetSlot(m, pick); err != nil {
				return false, fmt.Errorf("skill: TeachCut: choose move to forget: %w", err)
			}
			lastForget = pick
		case strings.Contains(text, "trying to learn"):
			if state.DecodeTwoOptionMenu(&mem) != nil {
				if err := SelectMenuItem(m, 0); err != nil {
					return false, fmt.Errorf("skill: TeachCut: answer replace-move prompt: %w", err)
				}
			} else {
				m.Tap(emu.A, 3, 7)
			}
		case tmhmPartyMenuUp(m):
			return false, nil
		default:
			m.Tap(emu.A, 3, 7)
		}
		m.StepFrames(20)
	}
	return false, fmt.Errorf("skill: TeachCut: slot %d did not settle within %d frames", slot, cutMenuBudget)
}

func cuttableFrontTile(tile uint8) bool { return tile == cutTreeTile || tile == gymCutTreeTile }

// CutAhead is retained as the public compatibility verb, but execution now
// goes through UseFieldMove so Travel, Vermilion Gym, and direct callers all
// share the same badge/learned-move/context/completion rules.
func CutAhead(m *emu.Emu) error {
	_, err := UseFieldMove(m, FieldCut)
	return err
}

// EnterVermilionGym is the public compatibility entry point. The route-gate
// implementation is destination-aware: it selects the actual Vermilion Gym
// warp and lets Traverse's shared field-path planner choose Cut only when that
// exact door route requires it. No independent tree scan remains here.
func EnterVermilionGym(m *emu.Emu, romData []byte, policy MovePolicy) error {
	return enterVermilionGymViaRouteGate(m, romData, policy)
}
