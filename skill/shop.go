package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/profiles"
)

// ItemAntidote is the Gen-I native item ID of ANTIDOTE. Public Buy/Sell keep
// their historical uint8 API while the transaction core uses uint16 native ids.
const ItemAntidote = 0x0b

// viridianMartMap is retained as a Gen-I compatibility constant for recovery
// planners that still reason about Red map ids. Transaction execution itself
// no longer branches on this value; the active shop profile owns that rule.
const viridianMartMap = 0x2a

var ErrCantAfford = errors.New("skill: not enough money for the purchase")
var ErrNotInStock = errors.New("skill: the clerk does not stock the requested item")
var ErrUnsellable = errors.New("skill: the item cannot be sold")
var ErrShopMenuTimeout = errors.New("skill: shop menu transition timed out")
var ErrShopControllerStalled = errors.New("skill: shop controller stalled")
var ErrShopStabilization = errors.New("skill: shop stabilization failed")
var ErrShopNotOpenYet = errors.New("skill: the clerk is not offering a shop yet")

const (
	martAdvanceBudget = 500
	martWaitBudget    = 60
	martQtyBudget     = 120
)

type shopRuntime interface {
	game.ShopDecoder
	game.InventoryDecoder
	game.OverworldDecoder
	game.MenuDecoder
	game.ListMenuDecoder
}

type shopMachine interface {
	menuMachine
}

func shopRuntimeFor(m *emu.Emu) (shopRuntime, error) {
	if m == nil {
		return nil, fmt.Errorf("skill: shop: nil emulator")
	}
	profile, _, err := profiles.Detect(m.ROM())
	if err != nil {
		return nil, fmt.Errorf("skill: shop: detect profile: %w", err)
	}
	runtime, ok := profile.(shopRuntime)
	if !ok {
		return nil, fmt.Errorf("skill: shop: profile %s@%s does not expose shop transaction semantics", profile.ID(), profile.Revision())
	}
	return runtime, nil
}

// Buy purchases qty units of a native Gen-I item and returns at a stable
// overworld boundary. The reusable transaction core is native-id-width agnostic.
func Buy(m *emu.Emu, item uint8, qty int) error {
	runtime, err := shopRuntimeFor(m)
	if err != nil {
		return err
	}
	return buyNative(m, runtime, uint16(item), qty)
}

func buyNative(m shopMachine, runtime shopRuntime, item uint16, qty int) error {
	if qty < 1 || qty > 99 {
		return fmt.Errorf("skill: Buy: quantity %d out of range 1..99", qty)
	}
	if runtime == nil {
		return fmt.Errorf("skill: Buy: nil shop runtime")
	}
	if !runtime.DecodeOverworld(m).Controllable {
		return fmt.Errorf("skill: Buy: player not controllable")
	}
	before := runtime.DecodeInventory(m)
	moneyBefore := int(before.Money)
	bagBefore := inventoryItemQuantity(before, item)
	if runtime.DecodeShop(m).TradeUnavailable {
		return fmt.Errorf("%w: current clerk cannot transact yet", ErrShopNotOpenYet)
	}

	m.Tap(emu.A, 3, 7)
	if err := shopAdvance(m, runtime, game.ShopPhaseActionMenu, "the BUY/SELL/QUIT menu"); err != nil {
		return recoverShopFailureWithRuntime(m, runtime, err)
	}
	if err := selectMenuItemWithDecoder(m, runtime, 0); err != nil {
		return recoverShopFailureWithRuntime(m, runtime, shopControllerFailure("select BUY", err))
	}
	if err := shopAdvance(m, runtime, game.ShopPhaseItemList, "the item list"); err != nil {
		return recoverShopFailureWithRuntime(m, runtime, err)
	}

	shop := runtime.DecodeShop(m)
	pos, ok := shopItemPosition(shop.Items, item)
	if !ok {
		primary := fmt.Errorf("skill: Buy: %w: item %#04x", ErrNotInStock, item)
		if cleanup := exitToOverworldWithRuntime(m, runtime); cleanup != nil {
			return shopStabilizationFailure(primary, cleanup)
		}
		return primary
	}
	if err := selectShopListEntry(m, runtime, pos); err != nil {
		return recoverShopFailureWithRuntime(m, runtime, shopControllerFailure(fmt.Sprintf("select item %#04x", item), err))
	}
	if err := shopWait(m, runtime, game.ShopPhaseQuantity, "the choose-quantity box"); err != nil {
		return recoverShopFailureWithRuntime(m, runtime, err)
	}
	if err := setShopQuantity(m, runtime, qty); err != nil {
		return recoverShopFailureWithRuntime(m, runtime, err)
	}
	total := runtime.DecodeShop(m).Total
	if moneyBefore < total {
		primary := fmt.Errorf("skill: Buy: %w: have %d, need %d", ErrCantAfford, moneyBefore, total)
		if cleanup := exitToOverworldWithRuntime(m, runtime); cleanup != nil {
			return shopStabilizationFailure(primary, cleanup)
		}
		return primary
	}

	m.Tap(emu.A, 3, 7)
	if err := shopAdvance(m, runtime, game.ShopPhaseConfirmation, "the purchase-confirmation prompt"); err != nil {
		return recoverShopFailureWithRuntime(m, runtime, err)
	}
	if err := selectTwoOptionWithDecoder(m, runtime, 0); err != nil {
		return recoverShopFailureWithRuntime(m, runtime, shopControllerFailure("answer YES", err))
	}
	if err := exitShopWithRuntime(m, runtime); err != nil {
		return recoverShopFailureWithRuntime(m, runtime, err)
	}

	after := runtime.DecodeInventory(m)
	bagAfter := inventoryItemQuantity(after, item)
	if bagAfter != bagBefore+qty {
		return fmt.Errorf("skill: Buy: bag count for item %#04x = %d, want %d (before %d + %d)", item, bagAfter, bagBefore+qty, bagBefore, qty)
	}
	if int(after.Money) != moneyBefore-total {
		return fmt.Errorf("skill: Buy: money = %d, want %d (before %d - total %d)", after.Money, moneyBefore-total, moneyBefore, total)
	}
	return nil
}

// Sell sells qty units of one native Gen-I item and returns at a stable
// overworld boundary.
func Sell(m *emu.Emu, item uint8, qty int) error {
	runtime, err := shopRuntimeFor(m)
	if err != nil {
		return err
	}
	return sellNative(m, runtime, uint16(item), qty)
}

func sellNative(m shopMachine, runtime shopRuntime, item uint16, qty int) error {
	if qty < 1 || qty > 99 {
		return fmt.Errorf("skill: Sell: quantity %d out of range 1..99", qty)
	}
	if runtime == nil {
		return fmt.Errorf("skill: Sell: nil shop runtime")
	}
	if !runtime.DecodeOverworld(m).Controllable {
		return fmt.Errorf("skill: Sell: player not controllable")
	}
	before := runtime.DecodeInventory(m)
	moneyBefore := int(before.Money)
	idx, bagBefore := inventoryEntry(before, item)
	if idx < 0 || bagBefore < qty {
		return fmt.Errorf("skill: Sell: item %#04x quantity %d, need %d", item, bagBefore, qty)
	}
	if runtime.DecodeShop(m).TradeUnavailable {
		return fmt.Errorf("%w: current clerk cannot transact yet", ErrShopNotOpenYet)
	}

	m.Tap(emu.A, 3, 7)
	if err := shopAdvance(m, runtime, game.ShopPhaseActionMenu, "the BUY/SELL/QUIT menu"); err != nil {
		return recoverShopFailureWithRuntime(m, runtime, err)
	}
	if err := selectMenuItemWithDecoder(m, runtime, 1); err != nil {
		return recoverShopFailureWithRuntime(m, runtime, shopControllerFailure("select SELL", err))
	}
	if err := shopAdvance(m, runtime, game.ShopPhaseItemList, "the sell item list"); err != nil {
		return recoverShopFailureWithRuntime(m, runtime, err)
	}

	liveInventory := runtime.DecodeInventory(m)
	liveIdx, liveQty := inventoryEntry(liveInventory, item)
	if liveIdx < 0 || liveQty < qty {
		return recoverShopFailureWithRuntime(m, runtime, fmt.Errorf("skill: Sell: item %#04x changed before selection: index=%d quantity=%d", item, liveIdx, liveQty))
	}
	if err := selectShopListEntry(m, runtime, liveIdx); err != nil {
		return recoverShopFailureWithRuntime(m, runtime, shopControllerFailure(fmt.Sprintf("select bag item %#04x", item), err))
	}
	if err := waitSellQuantity(m, runtime); err != nil {
		if runtime.DecodeShop(m).Unsellable {
			primary := fmt.Errorf("skill: Sell: %w: item %#04x", ErrUnsellable, item)
			if cleanup := exitToOverworldWithRuntime(m, runtime); cleanup != nil {
				return shopStabilizationFailure(primary, cleanup)
			}
			return primary
		}
		return recoverShopFailureWithRuntime(m, runtime, err)
	}
	if err := setShopQuantity(m, runtime, qty); err != nil {
		return recoverShopFailureWithRuntime(m, runtime, err)
	}
	total := runtime.DecodeShop(m).Total
	if total <= 0 {
		return recoverShopFailureWithRuntime(m, runtime, fmt.Errorf("skill: Sell: item %#04x x%d produced non-positive sale total %d", item, qty, total))
	}

	m.Tap(emu.A, 3, 7)
	if err := shopAdvance(m, runtime, game.ShopPhaseConfirmation, "the sale-confirmation prompt"); err != nil {
		return recoverShopFailureWithRuntime(m, runtime, err)
	}
	if err := selectTwoOptionWithDecoder(m, runtime, 0); err != nil {
		return recoverShopFailureWithRuntime(m, runtime, shopControllerFailure("answer sale YES", err))
	}

	expectedMoney := moneyBefore + total
	if expectedMoney > 999999 {
		expectedMoney = 999999
	}
	settled := false
	for i := 0; i < martAdvanceBudget; i++ {
		after := runtime.DecodeInventory(m)
		_, afterQty := inventoryEntry(after, item)
		if afterQty == bagBefore-qty && int(after.Money) == expectedMoney {
			settled = true
			break
		}
		m.StepFrame()
	}
	if !settled {
		after := runtime.DecodeInventory(m)
		_, afterQty := inventoryEntry(after, item)
		return recoverShopFailureWithRuntime(m, runtime, fmt.Errorf("skill: Sell: transaction did not settle: item %#04x %d->%d want %d, money %d->%d want %d",
			item, bagBefore, afterQty, bagBefore-qty, moneyBefore, after.Money, expectedMoney))
	}
	if err := exitToOverworldWithRuntime(m, runtime); err != nil {
		return recoverShopFailureWithRuntime(m, runtime, err)
	}

	after := runtime.DecodeInventory(m)
	_, bagAfter := inventoryEntry(after, item)
	if bagAfter != bagBefore-qty {
		return fmt.Errorf("skill: Sell: bag count for item %#04x = %d, want %d", item, bagAfter, bagBefore-qty)
	}
	if int(after.Money) != expectedMoney {
		return fmt.Errorf("skill: Sell: money = %d, want %d (before %d + sale %d)", after.Money, expectedMoney, moneyBefore, total)
	}
	return nil
}

func shopAdvance(m shopMachine, runtime shopRuntime, want game.ShopPhase, what string) error {
	for i := 0; i < martAdvanceBudget; i++ {
		state := runtime.DecodeShop(m)
		if state.Phase == want {
			return nil
		}
		if state.Phase == game.ShopPhaseGreeting {
			m.Tap(emu.A, 3, 7)
			m.StepFrames(talkSettle)
		} else {
			m.StepFrame()
		}
	}
	return shopTimeout(what, runtime.DecodeShop(m))
}

func shopWait(m shopMachine, runtime shopRuntime, want game.ShopPhase, what string) error {
	for i := 0; i < martWaitBudget; i++ {
		if runtime.DecodeShop(m).Phase == want {
			return nil
		}
		m.StepFrames(talkSettle)
	}
	return shopTimeout(what, runtime.DecodeShop(m))
}

func waitSellQuantity(m shopMachine, runtime shopRuntime) error {
	for i := 0; i < martWaitBudget; i++ {
		state := runtime.DecodeShop(m)
		if state.Unsellable {
			return ErrUnsellable
		}
		if state.Phase == game.ShopPhaseQuantity {
			return nil
		}
		m.StepFrames(talkSettle)
	}
	return shopTimeout("the sell choose-quantity box", runtime.DecodeShop(m))
}

func shopTimeout(what string, state game.ShopState) error {
	return fmt.Errorf("%w: skill: shop: %s did not appear (phase=%d quantity=%d max=%d total=%d)",
		ErrShopMenuTimeout, what, state.Phase, state.Quantity, state.MaxQuantity, state.Total)
}

func shopControllerFailure(context string, err error) error {
	return fmt.Errorf("skill: shop: %s: %w", context, errors.Join(ErrShopControllerStalled, err))
}

func shopStabilizationFailure(primary, cleanup error) error {
	return errors.Join(primary, fmt.Errorf("%w: %v", ErrShopStabilization, cleanup))
}

func recoverShopFailureWithRuntime(m shopMachine, runtime shopRuntime, err error) error {
	if err == nil {
		return nil
	}
	if cleanup := exitToOverworldWithRuntime(m, runtime); cleanup != nil {
		return shopStabilizationFailure(err, cleanup)
	}
	return err
}

func recoverShopFailure(m *emu.Emu, err error) error {
	runtime, runtimeErr := shopRuntimeFor(m)
	if runtimeErr != nil {
		return errors.Join(err, runtimeErr)
	}
	return recoverShopFailureWithRuntime(m, runtime, err)
}

func selectShopListEntry(m shopMachine, runtime shopRuntime, index int) error {
	if err := selectScrollingListEntryWithDecoder(m, runtime, index); err != nil {
		return err
	}
	m.StepFrames(talkSettle)
	return nil
}

func setShopQuantity(m shopMachine, runtime shopRuntime, qty int) error {
	for i := 0; i < martQtyBudget; i++ {
		state := runtime.DecodeShop(m)
		cur := state.Quantity
		if cur == qty {
			return nil
		}
		if cur > qty {
			return fmt.Errorf("%w: quantity overshot %d (quantity=%d)", ErrShopControllerStalled, qty, cur)
		}
		m.Tap(emu.Up, 3, 7)
		m.StepFrames(talkSettle)
	}
	return fmt.Errorf("%w: quantity did not reach %d (quantity=%d)", ErrShopControllerStalled, qty, runtime.DecodeShop(m).Quantity)
}

func exitShopWithRuntime(m shopMachine, runtime shopRuntime) error {
	if err := shopAdvance(m, runtime, game.ShopPhaseItemList, "the item list after the purchase"); err != nil {
		return err
	}
	return leaveFromItemListWithRuntime(m, runtime)
}

func leaveFromItemListWithRuntime(m shopMachine, runtime shopRuntime) error {
	m.Tap(emu.B, 3, 7)
	if err := shopWait(m, runtime, game.ShopPhaseActionMenu, "the BUY/SELL/QUIT menu after leaving the item list"); err != nil {
		return err
	}
	m.Tap(emu.B, 3, 7)
	return exitToOverworldWithRuntime(m, runtime)
}

func exitToOverworldWithRuntime(m shopMachine, runtime shopRuntime) error {
	for i := 0; i < martWaitBudget; i++ {
		if runtime.DecodeOverworld(m).Controllable {
			m.StepFrames(talkSettle)
			if runtime.DecodeOverworld(m).Controllable {
				return nil
			}
			continue
		}
		switch runtime.DecodeShop(m).Phase {
		case game.ShopPhaseActionMenu, game.ShopPhaseItemList, game.ShopPhaseQuantity, game.ShopPhaseConfirmation:
			m.Tap(emu.B, 3, 7)
			m.StepFrames(talkSettle)
		case game.ShopPhaseGreeting:
			m.Tap(emu.A, 3, 7)
			m.StepFrames(talkSettle)
		default:
			m.StepFrames(talkSettle)
		}
	}
	state := runtime.DecodeShop(m)
	world := runtime.DecodeOverworld(m)
	return fmt.Errorf("skill: shop: still not controllable after leaving shop (phase=%d map=%#04x at (%d,%d))",
		state.Phase, world.NativeMapID, world.X, world.Y)
}

func exitToOverworld(m *emu.Emu) error {
	runtime, err := shopRuntimeFor(m)
	if err != nil {
		return err
	}
	return exitToOverworldWithRuntime(m, runtime)
}
