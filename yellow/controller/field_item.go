package controller

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/yellow/sym"
)

const (
	yellowFieldItemMenuBudget   = 1200
	yellowFieldItemEffectBudget = 6000
)

var (
	ErrFieldItemNotInBag   = errors.New("yellow field item: item not in bag")
	ErrFieldItemChoice     = errors.New("yellow field item: unsupported choice requires explicit policy")
	ErrFieldItemNoEffect   = errors.New("yellow field item: no verified effect")
)

type yellowPartySlotState struct {
	species uint8
	level   uint8
	hp      uint16
	maxHP   uint16
	status  uint8
	moves   [4]uint8
	pp      [4]uint8
}

func readYellowBE16(m *emu.Emu, addr uint16) uint16 {
	return uint16(m.Peek8(addr))<<8 | uint16(m.Peek8(addr+1))
}

func yellowPartyState(m *emu.Emu, slot int) (yellowPartySlotState, error) {
	count := int(m.Peek8(sym.PartyCount))
	if slot < 0 || slot >= count {
		return yellowPartySlotState{}, fmt.Errorf("party slot %d outside party count %d", slot, count)
	}
	base := sym.PartyMon1 + uint16(slot)*sym.PartyMonSize
	var out yellowPartySlotState
	out.species = m.Peek8(base)
	out.hp = readYellowBE16(m, base+0x01)
	out.status = m.Peek8(base + 0x04)
	for i := 0; i < 4; i++ {
		out.moves[i] = m.Peek8(base + 0x08 + uint16(i))
		out.pp[i] = m.Peek8(base+0x1d+uint16(i)) & 0x3f
	}
	out.level = m.Peek8(base + 0x21)
	out.maxHP = readYellowBE16(m, base+0x22)
	return out, nil
}

func yellowFieldItemHadEffect(before, after yellowPartySlotState) bool {
	if before.species != after.species || after.level > before.level || after.hp > before.hp {
		return true
	}
	if before.status != 0 && after.status == 0 {
		return true
	}
	for i := range before.pp {
		if after.pp[i] > before.pp[i] {
			return true
		}
	}
	return false
}

func yellowBagEntry(m *emu.Emu, item uint8) (index, qty int) {
	count := int(m.Peek8(sym.NumBagItems))
	if count < 0 || count > 20 {
		return -1, 0
	}
	for i := 0; i < count; i++ {
		at := sym.BagItems + uint16(i*2)
		if m.Peek8(at) == item {
			return i, int(m.Peek8(at + 1))
		}
	}
	return -1, 0
}

func selectYellowBagEntry(m *emu.Emu, target int) error {
	count := int(m.Peek8(sym.ListCount))
	if target < 0 || target >= count {
		return fmt.Errorf("yellow field item: bag target %d outside list count %d", target, count)
	}
	for attempt := 0; attempt < 32; attempt++ {
		current := int(m.Peek8(sym.ListScrollOffset)) + int(m.Peek8(sym.CurrentMenuItem))
		if current == target {
			m.Tap(emu.A, 3, 7)
			return nil
		}
		button := emu.Down
		if current > target {
			button = emu.Up
		}
		m.Tap(button, 3, 7)
		if _, err := m.StepUntil(yellowMenuSettleBudget, func(m *emu.Emu) bool {
			return int(m.Peek8(sym.ListScrollOffset))+int(m.Peek8(sym.CurrentMenuItem)) != current
		}); err != nil {
			return fmt.Errorf("yellow field item: bag cursor stuck at %d targeting %d: %w", current, target, err)
		}
	}
	return fmt.Errorf("yellow field item: bag cursor did not reach %d", target)
}

func yellowUseTossPrompt(m *emu.Emu) bool {
	text := strings.ToUpper(screenText(m))
	return m.Peek8(sym.MaxMenuItem) == 1 &&
		strings.Contains(text, "USE") &&
		strings.Contains(text, "TOSS")
}

func yellowMoveLearningChoice(text string) (decline bool, known bool) {
	upper := strings.ToUpper(text)
	switch {
	case strings.Contains(upper, "TRYING TO LEARN"), strings.Contains(upper, "DELETE AN OLDER MOVE"):
		return true, true
	case strings.Contains(upper, "ABANDON LEARNING"):
		return false, true // answer YES to abandoning after declining replacement
	default:
		return false, false
	}
}

func yellowPPRestoreMoveSlot(state yellowPartySlotState) (int, bool) {
	best := -1
	bestPP := uint8(0xff)
	for i, move := range state.moves {
		if move == 0 {
			continue
		}
		if state.pp[i] == 0 {
			return i, true
		}
		if state.pp[i] < bestPP {
			best = i
			bestPP = state.pp[i]
		}
	}
	return best, best >= 0
}

func yellowSingleMovePPRestore(item uint8) bool {
	return item == 0x50 || item == 0x51 // ETHER / MAX ETHER
}

// UseFieldItem drives Yellow's overworld bag flow and verifies a real outcome.
// Consumables must disappear by exactly one. Party-targeted items additionally
// accept level/species/HP/status/PP changes as positive semantic evidence.
// Unknown YES/NO choices fail closed; the one predictable move-learning choice
// created by Rare Candy is declined rather than deleting an unknown move.
func UseFieldItem(m *emu.Emu, romData []byte, item uint8, slot int) error {
	if m == nil {
		return fmt.Errorf("yellow field item: nil emulator")
	}
	if m.Peek8(sym.IsInBattle) != 0 {
		return fmt.Errorf("yellow field item: cannot use overworld item during battle")
	}

	idx, qtyBefore := yellowBagEntry(m, item)
	if idx < 0 {
		return fmt.Errorf("%w: id %#02x", ErrFieldItemNotInBag, item)
	}
	if slot < 0 {
		slot = 0
	}
	beforeParty, partyErr := yellowPartyState(m, slot)
	mapBefore := m.Peek8(sym.CurMap)

	if err := openYellowBag(m, romData); err != nil {
		return fmt.Errorf("yellow field item: open bag: %w", err)
	}
	if err := selectYellowBagEntry(m, idx); err != nil {
		return err
	}
	if _, err := m.StepUntil(yellowFieldItemMenuBudget, yellowUseTossPrompt); err != nil {
		return fmt.Errorf("yellow field item: USE/TOSS prompt did not appear: screen=%q", strings.Join(strings.Fields(screenText(m)), " "))
	}
	if err := selectYellowLinearMenuItem(m, 0); err != nil {
		return fmt.Errorf("yellow field item: select USE: %w", err)
	}
	m.Tap(emu.A, 3, 7)

	partyTargeted := false
	moveSlot := -1
	for frame := 0; frame < yellowFieldItemMenuBudget; frame++ {
		if m.Peek8(sym.PartyMenuTypeOrMessage) == 1 {
			partyTargeted = true
			if partyErr != nil {
				return fmt.Errorf("yellow field item: target party: %w", partyErr)
			}
			if yellowSingleMovePPRestore(item) {
				var ok bool
				moveSlot, ok = yellowPPRestoreMoveSlot(beforeParty)
				if !ok {
					return fmt.Errorf("yellow field item: PP restore target has no move")
				}
			}
			if err := chooseYellowPartySlot(m, slot); err != nil {
				return fmt.Errorf("yellow field item: select party slot %d: %w", slot, err)
			}
			break
		}
		if m.Peek8(sym.ListMenuID) != yellowItemListMenuID || m.Peek8(sym.FontLoaded) != 0 || m.Peek8(sym.JoyIgnore) != 0 {
			break
		}
		m.StepFrame()
	}

	if partyTargeted && yellowSingleMovePPRestore(item) {
		if _, err := m.StepUntil(yellowFieldItemMenuBudget, func(m *emu.Emu) bool {
			return m.Peek8(sym.MoveMenuType) == 2 && m.Peek8(sym.CurrentMenuItem) >= 1
		}); err != nil {
			return fmt.Errorf("yellow field item: PP move menu did not appear")
		}
		if err := selectYellowLinearMenuItem(m, moveSlot+1); err != nil {
			return fmt.Errorf("yellow field item: select PP move slot %d: %w", moveSlot, err)
		}
		m.Tap(emu.A, 3, 7)
	}

	for frame := 0; frame < yellowFieldItemEffectBudget; frame++ {
		_, qtyAfter := yellowBagEntry(m, item)
		afterParty, afterPartyErr := yellowPartyState(m, slot)
		partyEffect := partyErr == nil && afterPartyErr == nil && yellowFieldItemHadEffect(beforeParty, afterParty)
		mapEffect := m.Peek8(sym.CurMap) != mapBefore
		consumed := qtyAfter == qtyBefore-1

		obsReady := false
		if m.Peek8(sym.IsInBattle) == 0 {
			if obs, err := yellowprofileObservation(m, romData); err == nil {
				obsReady = obs
			}
		}
		if obsReady && (consumed || partyEffect || mapEffect) {
			return nil
		}

		if m.Peek8(sym.MaxMenuItem) == 1 {
			text := screenText(m)
			decline, known := yellowMoveLearningChoice(text)
			if !known {
				return fmt.Errorf("%w: screen=%q", ErrFieldItemChoice, strings.Join(strings.Fields(text), " "))
			}
			if err := selectYellowTwoOption(m, decline); err != nil {
				return fmt.Errorf("yellow field item: resolve move-learning choice: %w", err)
			}
			continue
		}

		// B safely pages result text and backs out of bag/start menus. On the
		// overworld it is harmless, and completion is gated on semantic state.
		m.Tap(emu.B, 3, 7)
	}

	_, qtyAfter := yellowBagEntry(m, item)
	afterParty, _ := yellowPartyState(m, slot)
	return fmt.Errorf("%w: item=%#02x qty=%d->%d party=%+v->%+v map=%#02x->%#02x",
		ErrFieldItemNoEffect, item, qtyBefore, qtyAfter, beforeParty, afterParty,
		mapBefore, m.Peek8(sym.CurMap))
}

func yellowprofileObservation(m *emu.Emu, romData []byte) (bool, error) {
	obs, err := yellowprofile.New().DecodeObservation(m, romData)
	if err != nil {
		return false, err
	}
	return obs.Controllable && !obs.InBattle, nil
}
