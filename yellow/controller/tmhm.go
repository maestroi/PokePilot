package controller

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
	"github.com/maestroi/pokepilot/yellow/sym"
)

const yellowTMHMBudget = 9000

func yellowMonKnowsMove(state yellowPartySlotState, move uint8) bool {
	for _, id := range state.moves {
		if id == move {
			return true
		}
	}
	return false
}

func yellowTMHMRecipient(m *emu.Emu, romData []byte, item uint8, requested int) (slot, replace int, err error) {
	count := int(m.Peek8(sym.PartyCount))
	if count <= 0 {
		return -1, -1, fmt.Errorf("yellow TM/HM: empty party")
	}
	candidates := make([]int, 0, count)
	if requested >= 0 && requested < count {
		candidates = append(candidates, requested)
	} else {
		for i := 0; i < count; i++ {
			candidates = append(candidates, i)
		}
	}

	machine, err := yellowrom.LookupTMHM(romData, item)
	if err != nil {
		return -1, -1, err
	}
	for _, candidate := range candidates {
		state, err := yellowPartyState(m, candidate)
		if err != nil {
			continue
		}
		if yellowMonKnowsMove(state, machine.Move) {
			return candidate, -1, nil
		}
		ok, err := yellowrom.CanLearnTMHM(romData, state.species, item)
		if err != nil || !ok {
			continue
		}
		for i, move := range state.moves {
			if move == 0 {
				return candidate, -1, nil
			}
		}

		bestSlot := -1
		bestPower := 1000
		for i, move := range state.moves {
			hm, err := yellowrom.IsHMMove(romData, move)
			if err != nil || hm {
				continue
			}
			mv, err := yellowrom.LookupMove(romData, move)
			if err != nil {
				continue
			}
			power := int(mv.Power)
			if power < bestPower {
				bestPower = power
				bestSlot = i
			}
		}
		if bestSlot >= 0 {
			return candidate, bestSlot, nil
		}
	}
	return -1, -1, fmt.Errorf("yellow TM/HM: no compatible party member with a safe move slot for item %#02x", item)
}

func yellowForgetMenuUp(m *emu.Emu) bool {
	text := strings.ToUpper(screenText(m))
	return strings.Contains(text, "WHICH MOVE") ||
		(m.Peek8(sym.TopMenuItemX) == 5 && m.Peek8(sym.TopMenuItemY) == 8 && m.Peek8(sym.MaxMenuItem) <= 3)
}

// TeachTMHM teaches a Yellow TM/HM through the game's real bag and party
// menus. Compatibility comes from Yellow's ROM tables. When all four move
// slots are occupied it replaces the weakest non-HM move and refuses to
// delete an HM.
func TeachTMHM(m *emu.Emu, romData []byte, item uint8, requestedSlot int) error {
	if m == nil {
		return fmt.Errorf("yellow TM/HM: nil emulator")
	}
	machine, err := yellowrom.LookupTMHM(romData, item)
	if err != nil {
		return err
	}
	idx, beforeQty := yellowBagEntry(m, item)
	if idx < 0 {
		return fmt.Errorf("yellow TM/HM: item %#02x is not in the bag", item)
	}
	slot, replace, err := yellowTMHMRecipient(m, romData, item, requestedSlot)
	if err != nil {
		return err
	}
	before, err := yellowPartyState(m, slot)
	if err != nil {
		return err
	}
	if yellowMonKnowsMove(before, machine.Move) {
		return nil
	}

	if err := openYellowBag(m, romData); err != nil {
		return fmt.Errorf("yellow TM/HM: open bag: %w", err)
	}
	if err := selectYellowBagEntry(m, idx); err != nil {
		return fmt.Errorf("yellow TM/HM: select machine: %w", err)
	}
	if _, err := m.StepUntil(1200, yellowUseTossPrompt); err != nil {
		return fmt.Errorf("yellow TM/HM: USE/TOSS prompt did not appear")
	}
	if err := selectYellowLinearMenuItem(m, 0); err != nil {
		return fmt.Errorf("yellow TM/HM: select USE: %w", err)
	}
	m.Tap(emu.A, 3, 7)

	teachPrompt := func(m *emu.Emu) bool {
		text := strings.ToUpper(screenText(m))
		return m.Peek8(sym.MaxMenuItem) == 1 && strings.Contains(text, "TEACH")
	}
	for frame := 0; frame < 2000 && !teachPrompt(m); frame++ {
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}
	if !teachPrompt(m) {
		return fmt.Errorf("yellow TM/HM: teach prompt did not appear: screen=%q", strings.Join(strings.Fields(screenText(m)), " "))
	}
	if err := selectYellowTwoOption(m, false); err != nil {
		return fmt.Errorf("yellow TM/HM: answer teach prompt: %w", err)
	}

	if _, err := m.StepUntil(1200, func(m *emu.Emu) bool {
		return m.Peek8(sym.PartyMenuTypeOrMessage) == 3
	}); err != nil {
		return fmt.Errorf("yellow TM/HM: party menu did not appear")
	}
	if err := chooseYellowPartySlot(m, slot); err != nil {
		return fmt.Errorf("yellow TM/HM: choose party slot %d: %w", slot, err)
	}

	for frame := 0; frame < yellowTMHMBudget; frame++ {
		state, err := yellowPartyState(m, slot)
		if err == nil && yellowMonKnowsMove(state, machine.Move) {
			break
		}
		text := strings.ToUpper(screenText(m))
		switch {
		case strings.Contains(text, "NOT COMPATIBLE"):
			return fmt.Errorf("yellow TM/HM: ROM compatibility accepted slot %d but game rejected it", slot)
		case yellowForgetMenuUp(m):
			if replace < 0 {
				return fmt.Errorf("yellow TM/HM: game requested move replacement despite an empty slot")
			}
			if err := selectYellowLinearMenuItem(m, replace); err != nil {
				return fmt.Errorf("yellow TM/HM: select forget slot %d: %w", replace, err)
			}
			m.Tap(emu.A, 3, 7)
		case m.Peek8(sym.MaxMenuItem) == 1 && strings.Contains(text, "TRYING TO LEARN"):
			if replace < 0 {
				return fmt.Errorf("yellow TM/HM: unexpected replacement prompt with empty move slot")
			}
			if err := selectYellowTwoOption(m, false); err != nil {
				return fmt.Errorf("yellow TM/HM: accept move replacement: %w", err)
			}
		case m.Peek8(sym.MaxMenuItem) == 1 && strings.Contains(text, "ABANDON LEARNING"):
			// We intend to replace a move, so answer NO and return to the
			// move list rather than abandoning the required machine.
			if err := selectYellowTwoOption(m, true); err != nil {
				return fmt.Errorf("yellow TM/HM: decline abandon learning: %w", err)
			}
		default:
			m.Tap(emu.A, 3, 7)
		}
	}
	after, err := yellowPartyState(m, slot)
	if err != nil || !yellowMonKnowsMove(after, machine.Move) {
		return fmt.Errorf("yellow TM/HM: move %d was not learned by slot %d", machine.Move, slot)
	}

	for frame := 0; frame < 3000; frame++ {
		ready, err := yellowprofileObservation(m, romData)
		if err == nil && ready {
			break
		}
		if m.Peek8(sym.MaxMenuItem) == 1 {
			return fmt.Errorf("yellow TM/HM: unexpected choice while closing menus: screen=%q", strings.Join(strings.Fields(screenText(m)), " "))
		}
		m.Tap(emu.B, 3, 7)
	}
	ready, err := yellowprofileObservation(m, romData)
	if err != nil || !ready {
		return fmt.Errorf("yellow TM/HM: did not return to a stable overworld boundary")
	}

	_, afterQty := yellowBagEntry(m, item)
	if machine.Consumable() {
		if afterQty != beforeQty-1 {
			return fmt.Errorf("yellow TM/HM: TM %#02x quantity %d->%d, want one consumed", item, beforeQty, afterQty)
		}
	} else if afterQty != beforeQty {
		return fmt.Errorf("yellow TM/HM: HM %#02x quantity changed %d->%d", item, beforeQty, afterQty)
	}
	return nil
}
