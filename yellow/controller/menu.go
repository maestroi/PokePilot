package controller

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/gen1"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
	"github.com/maestroi/pokepilot/yellow/sym"
)

const (
	yellowStartMenuOpenBudget  = 500
	yellowStartMenuRetryWindow = 25
	yellowItemListMenuID       = 3
)

func yellowStartMenuShape(m *emu.Emu, romData []byte) (maxIndex, pokemonIndex, itemIndex int, err error) {
	obs, err := yellowprofile.New().DecodeObservation(m, romData)
	if err != nil {
		return 0, 0, 0, err
	}
	if obs.Story.Has(gen1.ProgressPokedexAcquired) {
		return 6, 1, 2, nil
	}
	return 5, 0, 1, nil
}

func yellowStartMenuReady(m *emu.Emu, wantMax int) bool {
	text := strings.ToUpper(screenText(m))
	return strings.Contains(text, "SAVE") &&
		strings.Contains(text, "EXIT") &&
		int(m.Peek8(sym.MaxMenuItem)) == wantMax
}

func openYellowStartMenuEntry(m *emu.Emu, romData []byte, entry int) error {
	wantMax, _, _, err := yellowStartMenuShape(m, romData)
	if err != nil {
		return fmt.Errorf("yellow menu: derive START shape: %w", err)
	}
	if entry < 0 || entry > wantMax {
		return fmt.Errorf("yellow menu: START entry %d outside 0..%d", entry, wantMax)
	}

	attempts := yellowStartMenuOpenBudget / yellowStartMenuRetryWindow
	for attempt := 0; attempt < attempts; attempt++ {
		if yellowStartMenuReady(m, wantMax) {
			if err := selectYellowLinearMenuItem(m, entry); err != nil {
				return fmt.Errorf("yellow menu: select START entry %d: %w", entry, err)
			}
			m.Tap(emu.A, 3, 7)
			return nil
		}
		if m.Peek8(sym.IsInBattle) != 0 {
			return fmt.Errorf("yellow menu: START cannot open during battle")
		}
		m.Tap(emu.Start, 3, 7)
		if _, err := m.StepUntil(yellowStartMenuRetryWindow, func(m *emu.Emu) bool {
			return yellowStartMenuReady(m, wantMax)
		}); err == nil {
			if err := selectYellowLinearMenuItem(m, entry); err != nil {
				return fmt.Errorf("yellow menu: select START entry %d: %w", entry, err)
			}
			m.Tap(emu.A, 3, 7)
			return nil
		}
	}
	return fmt.Errorf("yellow menu: START menu did not appear within %d frames: screen=%q max=%d want=%d",
		yellowStartMenuOpenBudget, strings.Join(strings.Fields(screenText(m)), " "),
		m.Peek8(sym.MaxMenuItem), wantMax)
}

func openYellowBag(m *emu.Emu, romData []byte) error {
	_, _, itemIndex, err := yellowStartMenuShape(m, romData)
	if err != nil {
		return err
	}
	if err := openYellowStartMenuEntry(m, romData, itemIndex); err != nil {
		return err
	}
	if _, err := m.StepUntil(600, func(m *emu.Emu) bool {
		return m.Peek8(sym.ListMenuID) == yellowItemListMenuID && m.Peek8(sym.ListCount) != 0
	}); err != nil {
		return fmt.Errorf("yellow menu: bag list did not open: screen=%q list_id=%#02x list_count=%d",
			strings.Join(strings.Fields(screenText(m)), " "),
			m.Peek8(sym.ListMenuID), m.Peek8(sym.ListCount))
	}
	return nil
}

func selectYellowPartySlot(m *emu.Emu, target int) error {
	count := int(m.Peek8(sym.PartyCount))
	if target < 0 || target >= count {
		return fmt.Errorf("yellow menu: party slot %d outside party count %d", target, count)
	}
	for attempt := 0; attempt < 16; attempt++ {
		current := int(m.Peek8(sym.CurrentMenuItem))
		if current == target {
			break
		}
		button := emu.Down
		if current > target {
			button = emu.Up
		}
		m.Tap(button, 3, 7)
		if _, err := m.StepUntil(yellowMenuSettleBudget, func(m *emu.Emu) bool {
			return int(m.Peek8(sym.CurrentMenuItem)) != current
		}); err != nil {
			return fmt.Errorf("yellow menu: party cursor stuck at %d targeting %d: %w", current, target, err)
		}
	}
	if int(m.Peek8(sym.CurrentMenuItem)) != target {
		return fmt.Errorf("yellow menu: party cursor did not reach %d", target)
	}
	m.Tap(emu.A, 3, 7)
	return nil
}
