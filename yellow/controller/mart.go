package controller

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
	"github.com/maestroi/pokepilot/yellow/sym"
)

const (
	yellowMartMenuBudget     = 3000
	yellowMartTransition     = 900
	yellowMartCursorBudget   = 24
	yellowMartQuantityBudget = 120
	yellowMartExitBudget     = 3000
)

var (
	ErrMartNotOpen    = errors.New("yellow mart: clerk is not offering a shop")
	ErrMartNotInStock = errors.New("yellow mart: requested item is not in stock")
	ErrMartCantAfford = errors.New("yellow mart: not enough money")
)

func yellowShopMenuUp(m *emu.Emu) bool {
	return m.Peek8(sym.MenuWatchedKeys) == 3 && m.Peek8(sym.MaxMenuItem) == 2
}

func yellowItemListUp(m *emu.Emu) bool {
	return m.Peek8(sym.MenuWatchedKeys) == 7 && m.Peek8(sym.ListCount) != 0
}

func yellowListPosition(m *emu.Emu) int {
	return int(m.Peek8(sym.ListScrollOffset)) + int(m.Peek8(sym.CurrentMenuItem))
}

func yellowBCD3(m *emu.Emu, addr uint16) int {
	value := 0
	for i := uint16(0); i < 3; i++ {
		b := m.Peek8(addr + i)
		value = value*100 + int(b>>4)*10 + int(b&0x0f)
	}
	return value
}

func yellowBagItemCount(m *emu.Emu, item uint8) (int, error) {
	count := int(m.Peek8(sym.NumBagItems))
	if count < 0 || count > 20 {
		return 0, fmt.Errorf("yellow mart: invalid bag entry count %d", count)
	}
	for i := 0; i < count; i++ {
		at := sym.BagItems + uint16(i*2)
		if m.Peek8(at) == item {
			return int(m.Peek8(at + 1)), nil
		}
	}
	return 0, nil
}

func yellowMartStockPosition(stock []uint8, item uint8) int {
	for i, id := range stock {
		if id == item {
			return i
		}
	}
	return -1
}

// Buy purchases one Yellow-native item from the current map's ROM-derived
// mart stock. Clerk identity and stock both come from the Yellow ROM parser;
// menu state and postconditions come only from Yellow RAM.
func Buy(m *emu.Emu, romData []byte, item uint8, qty int) error {
	if m == nil {
		return fmt.Errorf("yellow mart: nil emulator")
	}
	if qty < 1 || qty > 99 {
		return fmt.Errorf("yellow mart: quantity %d outside 1..99", qty)
	}
	obs, err := yellowprofile.New().DecodeObservation(m, romData)
	if err != nil {
		return fmt.Errorf("yellow mart: observe: %w", err)
	}
	if !obs.Controllable || obs.InBattle {
		return fmt.Errorf("yellow mart: player is not at a controllable overworld boundary")
	}

	mapID := uint8(obs.NativeMapID)
	stock, err := yellowrom.MartItems(romData, mapID)
	if err != nil {
		return fmt.Errorf("yellow mart: read stock on map %#02x: %w", mapID, err)
	}
	position := yellowMartStockPosition(stock, item)
	if position < 0 {
		return fmt.Errorf("%w: item %#02x on map %#02x", ErrMartNotInStock, item, mapID)
	}

	bagBefore, err := yellowBagItemCount(m, item)
	if err != nil {
		return err
	}
	moneyBefore := yellowBCD3(m, sym.PlayerMoney)

	clerkX, clerkY, err := yellowrom.MartClerkPosition(romData, mapID)
	if err != nil {
		return fmt.Errorf("yellow mart: locate clerk: %w", err)
	}
	if err := interactAt(m, romData, int(clerkX), int(clerkY)); err != nil {
		return fmt.Errorf("yellow mart: approach clerk at (%d,%d): %w", clerkX, clerkY, err)
	}
	if err := waitYellowShopMenu(m); err != nil {
		return recoverYellowMartFailure(m, romData, err)
	}

	if err := selectYellowLinearMenuItem(m, 0); err != nil {
		return recoverYellowMartFailure(m, romData, fmt.Errorf("yellow mart: select BUY: %w", err))
	}
	m.Tap(emu.A, 3, 7)
	if err := waitYellowItemList(m); err != nil {
		return recoverYellowMartFailure(m, romData, err)
	}

	if err := selectYellowMartListEntry(m, position); err != nil {
		return recoverYellowMartFailure(m, romData, err)
	}
	beforeQuantityText := screenText(m)
	m.Tap(emu.A, 3, 7)
	if err := waitYellowQuantityBox(m, beforeQuantityText); err != nil {
		return recoverYellowMartFailure(m, romData, err)
	}

	if err := setYellowMartQuantity(m, qty); err != nil {
		return recoverYellowMartFailure(m, romData, err)
	}
	total := yellowBCD3(m, sym.MoneyTemp)
	if total <= 0 {
		return recoverYellowMartFailure(m, romData, fmt.Errorf("yellow mart: computed purchase total is %d", total))
	}
	if moneyBefore < total {
		primary := fmt.Errorf("%w: have %d, need %d", ErrMartCantAfford, moneyBefore, total)
		return recoverYellowMartFailure(m, romData, primary)
	}

	m.Tap(emu.A, 3, 7)
	if err := waitYellowTwoOption(m, yellowMartTransition); err != nil {
		return recoverYellowMartFailure(m, romData, fmt.Errorf("yellow mart: purchase confirmation: %w", err))
	}
	if err := selectYellowTwoOption(m, false); err != nil {
		return recoverYellowMartFailure(m, romData, fmt.Errorf("yellow mart: confirm purchase: %w", err))
	}

	if err := waitYellowItemList(m); err != nil {
		return recoverYellowMartFailure(m, romData, fmt.Errorf("yellow mart: return to item list after purchase: %w", err))
	}
	m.Tap(emu.B, 3, 7)
	if err := waitYellowShopMenuNoAdvance(m); err != nil {
		return recoverYellowMartFailure(m, romData, fmt.Errorf("yellow mart: return to BUY/SELL/QUIT: %w", err))
	}
	m.Tap(emu.B, 3, 7)
	if err := waitYellowControllable(m, romData, yellowMartExitBudget); err != nil {
		return fmt.Errorf("yellow mart: exit after purchase: %w", err)
	}

	bagAfter, err := yellowBagItemCount(m, item)
	if err != nil {
		return err
	}
	moneyAfter := yellowBCD3(m, sym.PlayerMoney)
	if bagAfter != bagBefore+qty {
		return fmt.Errorf("yellow mart: item %#02x count %d, want %d (before %d + %d)",
			item, bagAfter, bagBefore+qty, bagBefore, qty)
	}
	if moneyAfter != moneyBefore-total {
		return fmt.Errorf("yellow mart: money %d, want %d (before %d - total %d)",
			moneyAfter, moneyBefore-total, moneyBefore, total)
	}
	return nil
}

func waitYellowShopMenu(m *emu.Emu) error {
	seenInteraction := false
	for frame := 0; frame < yellowMartMenuBudget; frame++ {
		if yellowShopMenuUp(m) {
			return nil
		}
		if m.Peek8(sym.FontLoaded) != 0 || m.Peek8(sym.JoyIgnore) != 0 {
			seenInteraction = true
		}
		if seenInteraction && m.Peek8(sym.FontLoaded) == 0 && m.Peek8(sym.JoyIgnore) == 0 {
			return ErrMartNotOpen
		}
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}
	return fmt.Errorf("yellow mart: BUY/SELL/QUIT menu did not appear within %d frames", yellowMartMenuBudget)
}

func waitYellowShopMenuNoAdvance(m *emu.Emu) error {
	for frame := 0; frame < yellowMartTransition; frame++ {
		if yellowShopMenuUp(m) {
			return nil
		}
		m.StepFrame()
	}
	return fmt.Errorf("yellow mart: BUY/SELL/QUIT menu did not return within %d frames", yellowMartTransition)
}

func waitYellowItemList(m *emu.Emu) error {
	for frame := 0; frame < yellowMartTransition; frame++ {
		if yellowItemListUp(m) {
			return nil
		}
		if m.Peek8(sym.MaxMenuItem) == 1 {
			return fmt.Errorf("yellow mart: unexpected two-option choice before item list")
		}
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}
	return fmt.Errorf("yellow mart: item list did not appear within %d frames", yellowMartTransition)
}

func waitYellowQuantityBox(m *emu.Emu, previousText string) error {
	for frame := 0; frame < yellowMartTransition; frame++ {
		if m.Peek8(sym.ItemQuantity) != 0 && yellowBCD3(m, sym.MoneyTemp) > 0 && screenText(m) != previousText {
			return nil
		}
		m.StepFrame()
	}
	return fmt.Errorf("yellow mart: quantity box did not appear within %d frames", yellowMartTransition)
}

func waitYellowTwoOption(m *emu.Emu, budget int) error {
	for frame := 0; frame < budget; frame++ {
		if m.Peek8(sym.MaxMenuItem) == 1 {
			return nil
		}
		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
		} else {
			m.StepFrame()
		}
	}
	return fmt.Errorf("two-option menu did not appear within %d frames", budget)
}

func selectYellowMartListEntry(m *emu.Emu, target int) error {
	if target < 0 || target >= int(m.Peek8(sym.ListCount)) {
		return fmt.Errorf("yellow mart: list target %d outside list count %d", target, m.Peek8(sym.ListCount))
	}
	for attempt := 0; attempt < yellowMartCursorBudget; attempt++ {
		current := yellowListPosition(m)
		if current == target {
			return nil
		}
		button := emu.Down
		if current > target {
			button = emu.Up
		}
		m.Tap(button, 3, 7)
		if _, err := m.StepUntil(yellowMenuSettleBudget, func(m *emu.Emu) bool {
			return yellowListPosition(m) != current
		}); err != nil {
			return fmt.Errorf("yellow mart: list cursor stuck at %d targeting %d: %w", current, target, err)
		}
	}
	return fmt.Errorf("yellow mart: list cursor did not reach %d from %d", target, yellowListPosition(m))
}

func setYellowMartQuantity(m *emu.Emu, target int) error {
	for attempt := 0; attempt < yellowMartQuantityBudget; attempt++ {
		current := int(m.Peek8(sym.ItemQuantity))
		if current == target {
			return nil
		}
		button := emu.Up
		if current > target {
			button = emu.Down
		}
		m.Tap(button, 3, 7)
		if _, err := m.StepUntil(yellowMenuSettleBudget, func(m *emu.Emu) bool {
			return int(m.Peek8(sym.ItemQuantity)) != current
		}); err != nil {
			return fmt.Errorf("yellow mart: quantity stuck at %d targeting %d: %w", current, target, err)
		}
	}
	return fmt.Errorf("yellow mart: quantity did not reach %d from %d", target, m.Peek8(sym.ItemQuantity))
}

func waitYellowControllable(m *emu.Emu, romData []byte, budget int) error {
	stable := 0
	for frame := 0; frame < budget; frame++ {
		obs, err := yellowprofile.New().DecodeObservation(m, romData)
		if err != nil {
			return err
		}
		if obs.Controllable && !obs.InBattle {
			stable++
			if stable >= 8 {
				return nil
			}
		} else {
			stable = 0
		}
		m.StepFrame()
	}
	return fmt.Errorf("controllable boundary did not settle within %d frames", budget)
}

func recoverYellowMartFailure(m *emu.Emu, romData []byte, primary error) error {
	if primary == nil {
		return nil
	}
	for attempt := 0; attempt < 12; attempt++ {
		if err := waitYellowControllable(m, romData, 24); err == nil {
			return primary
		}
		m.Tap(emu.B, 3, 7)
	}
	if err := waitYellowControllable(m, romData, yellowMartExitBudget); err != nil {
		return errors.Join(primary, fmt.Errorf("yellow mart: cleanup failed: %w", err))
	}
	return primary
}
