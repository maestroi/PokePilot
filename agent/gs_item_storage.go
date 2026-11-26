package agent

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/gs/sym"
	"github.com/maestroi/pokepilot/skill"
)

// Gold/Silver mandatory story gifts such as the Dragon Fang go to the 20-slot
// Items pocket; a full pocket makes the retail giveitem fail and the scene
// repeat. Freeing a slot stores one whole stack in the player's PC (reversible:
// it can be withdrawn later), never tosses or sells it.

// gsDepositKeep are Items-pocket entries the League resource preparation
// spends; depositing them would trade a story gift for a battle loss.
var gsDepositKeep = map[uint8]bool{
	gsItemAntidote: true, gsItemBurnHeal: true, gsItemIceHeal: true, gsItemAwakening: true,
	gsItemParlyzHeal: true, gsItemFullRestore: true, gsItemMaxPotion: true, gsItemHyperPotion: true,
	gsItemSuperPotion: true, gsItemPotion: true, gsItemMaxElixer: true, gsItemFullHeal: true,
	gsItemRevive: true, gsItemMaxRevive: true, gsItemFreshWater: true, gsItemSodaPop: true,
	gsItemLemonade: true, gsItemEther: true, gsItemMaxEther: true, gsItemElixer: true,
}

type gsItemStack struct{ id, qty uint8 }

func gsItemsPocket(m *emu.Emu) []gsItemStack {
	n := min(int(m.Peek8(sym.NumItems)), sym.MaxItems)
	out := make([]gsItemStack, n)
	for i := range out {
		out[i] = gsItemStack{m.Peek8(sym.Items + uint16(2*i)), m.Peek8(sym.Items + uint16(2*i+1))}
	}
	return out
}

// gsDepositSlotChoice picks the last stack that is not League medicine, or
// the last stack when everything is medicine.
func gsDepositSlotChoice(items []gsItemStack) int {
	for i := len(items) - 1; i >= 0; i-- {
		if !gsDepositKeep[items[i].id] {
			return i
		}
	}
	return len(items) - 1
}

// gsPlayersPCMenuUp is the WITHDRAW ITEM / DEPOSIT ITEM / ... menu.
func gsPlayersPCMenuUp(m *emu.Emu, profile *gsprofile.Profile) bool {
	text := gsPCText(m, profile)
	return strings.Contains(text, "WITHDRAW ITEM") && strings.Contains(text, "DEPOSIT ITEM")
}

// gsEnsureItemsSlot frees one Items-pocket slot at the current area's
// Pokemon Center when the pocket is full. No-op otherwise.
func gsEnsureItemsSlot(m *emu.Emu, romData []byte, profile *gsprofile.Profile) error {
	if int(m.Peek8(sym.NumItems)) < sym.MaxItems {
		return nil
	}
	if err := gsOpenCenterPC(m, romData, profile, gsCenterPCRowPlayer,
		func() bool { return gsPlayersPCMenuUp(m, profile) }, "player's PC"); err != nil {
		return fmt.Errorf("gen2 free Items slot: %w", err)
	}
	depositErr := gsDepositItemStack(m, profile, gsDepositSlotChoice(gsItemsPocket(m)))
	if err := gsCloseBillsPC(m, profile); err != nil && depositErr == nil {
		depositErr = err
	}
	if depositErr != nil {
		return fmt.Errorf("gen2 free Items slot: %w", depositErr)
	}
	return nil
}

// gsDepositItemStack runs DEPOSIT ITEM from an open player's PC menu and
// stores the whole stack at index. Success is the Items pocket shrinking by
// one entry (pokecenter_pc.asm PlayerDepositItemMenu).
func gsDepositItemStack(m *emu.Emu, profile *gsprofile.Profile, index int) error {
	items := gsItemsPocket(m)
	if index < 0 || index >= len(items) {
		return fmt.Errorf("deposit index %d outside %d-item pocket", index, len(items))
	}
	stack := items[index]
	if err := skill.SelectMenuItem(m, 1); err != nil { // PLAYERSPCITEM_DEPOSIT_ITEM
		return fmt.Errorf("choose DEPOSIT ITEM: %w", err)
	}
	// DepositSellPack always opens the Items pocket's ScrollingMenu.
	position := func() int {
		return int(m.Peek8(sym.MenuScrollPosition)) + int(m.Peek8(sym.MenuCursorY)) - 1
	}
	listUp := func() bool {
		return m.Peek8(sym.CurPocket) == 0 && m.Peek8(sym.MenuCursorY) >= 1 &&
			strings.Contains(gsPCText(m, profile), "CANCEL")
	}
	if !gsPCWait(m, listUp) {
		return fmt.Errorf("%w: deposit item list did not open (screen %q)", errGSSecondBadgeStalled, gsPCText(m, profile))
	}
	for moves := 0; position() != index; moves++ {
		if moves > gsPCMaxListMoves {
			return fmt.Errorf("%w: deposit cursor stuck at %d, want %d", errGSSecondBadgeStalled, position(), index)
		}
		btn := emu.Down
		if position() > index {
			btn = emu.Up
		}
		m.Tap(btn, gsPCMenuHold, 7)
		m.StepFrames(gsPCStepFrames)
	}
	shrunk := func() bool { return int(m.Peek8(sym.NumItems)) == len(items)-1 }
	asked := func() bool { return strings.Contains(gsPCText(m, profile), "How many") }
	// A freshly drawn list can drop the first A; retry only while it is still
	// showing the same cursor, so a press never lands on the next prompt.
	for attempt := 0; ; attempt++ {
		m.Tap(emu.A, gsPCMenuHold, 7)
		if gsPCWait(m, func() bool { return shrunk() || asked() || !listUp() }) && (shrunk() || asked()) {
			break
		}
		if attempt >= 2 || !listUp() || position() != index {
			return fmt.Errorf("%w: deposit of item %#02x neither asked a quantity nor stored it (screen %q)",
				errGSSecondBadgeStalled, stack.id, gsPCText(m, profile))
		}
	}
	if asked() {
		// BuySellToss_InterpretJoypad: DOWN from 1 wraps to the whole stack.
		if stack.qty > 1 {
			m.Tap(emu.Down, gsPCMenuHold, 7)
			m.StepFrames(gsPCStepFrames)
		}
		for attempt := 0; asked() && !shrunk(); attempt++ {
			if got := m.Peek8(sym.ItemQuantityChange); got != stack.qty {
				return fmt.Errorf("%w: deposit quantity %d, want whole stack %d", errGSSecondBadgeStalled, got, stack.qty)
			}
			if attempt > 2 {
				break
			}
			m.Tap(emu.A, gsPCMenuHold, 7)
			gsPCWait(m, func() bool { return shrunk() || !asked() })
		}
	}
	if !gsPCWait(m, shrunk) {
		return fmt.Errorf("%w: Items pocket still has %d entries after depositing item %#02x x%d (screen %q)",
			errGSPCRefused, m.Peek8(sym.NumItems), stack.id, stack.qty, gsPCText(m, profile))
	}
	return nil
}
