package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// Gen 1 item IDs from pokered/constants/item_constants.asm. The automatic
// battle policy intentionally covers ordinary medicine only.
const (
	itemAntidote    uint8 = 0x0b
	itemBurnHeal    uint8 = 0x0c
	itemIceHeal     uint8 = 0x0d
	itemAwakening   uint8 = 0x0e
	itemParlyzHeal  uint8 = 0x0f
	itemFullRestore uint8 = 0x10
	itemMaxPotion   uint8 = 0x11
	itemHyperPotion uint8 = 0x12
	itemSuperPotion uint8 = 0x13
	itemPotion      uint8 = 0x14
	itemFullHeal    uint8 = 0x34
	itemFreshWater  uint8 = 0x3c
	itemSodaPop     uint8 = 0x3d
	itemLemonade    uint8 = 0x3e
)

// Even if an enemy repeatedly knocks the active mon back below the healing
// line, automatic medicine can consume only this many battle turns.
const battleItemUseCap = 4

type battleMedicineChoice struct {
	Item   uint8
	Slot   int
	Reason string
}

type hpMedicine struct {
	item uint8
	heal int
}

// Ordered by heal size so the first sufficient item minimizes waste. Full
// heals come last, which preserves them when an ordinary finite heal suffices.
var hpMedicines = []hpMedicine{
	{item: itemPotion, heal: 20},
	{item: itemSuperPotion, heal: 50},
	{item: itemFreshWater, heal: 50},
	{item: itemSodaPop, heal: 60},
	{item: itemLemonade, heal: 80},
	{item: itemHyperPotion, heal: 200},
	{item: itemMaxPotion, heal: 1 << 30},
	{item: itemFullRestore, heal: 1 << 30},
}

// chooseBattleMedicine returns one conservative medicine action for the
// active mon. HP medicine is considered only at one-third HP or below;
// status medicine is considered whenever a matching cure exists. When both
// apply, FULL RESTORE resolves them in one turn if available.
func chooseBattleMedicine(mem *state.Mem) (battleMedicineChoice, bool) {
	if state.DecodeBattle(mem) == nil {
		return battleMedicineChoice{}, false
	}
	party := state.DecodeParty(mem)
	slot := int(mem.U8(sym.PlayerMonNumber))
	if slot < 0 || slot >= len(party.Mons) {
		return battleMedicineChoice{}, false
	}
	mon := party.Mons[slot]
	if mon.Fainted() || mon.MaxHP == 0 {
		return battleMedicineChoice{}, false
	}

	lowHP := mon.HP*3 <= mon.MaxHP
	status := mon.StatusName()
	if lowHP && status != "" && bagHasItem(mem, itemFullRestore) {
		return battleMedicineChoice{
			Item:   itemFullRestore,
			Slot:   slot,
			Reason: fmt.Sprintf("active HP %d/%d and %s", mon.HP, mon.MaxHP, status),
		}, true
	}
	if lowHP {
		if item, ok := chooseHPMedicine(mem, int(mon.MaxHP-mon.HP)); ok {
			return battleMedicineChoice{
				Item:   item,
				Slot:   slot,
				Reason: fmt.Sprintf("active HP %d/%d", mon.HP, mon.MaxHP),
			}, true
		}
	}
	if status != "" {
		if item, ok := chooseStatusMedicine(mem, status); ok {
			return battleMedicineChoice{Item: item, Slot: slot, Reason: "active is " + status}, true
		}
	}
	return battleMedicineChoice{}, false
}

func chooseHPMedicine(mem *state.Mem, missing int) (uint8, bool) {
	var strongest uint8
	found := false
	for _, med := range hpMedicines {
		if !bagHasItem(mem, med.item) {
			continue
		}
		strongest = med.item
		found = true
		if med.heal >= missing {
			return med.item, true
		}
	}
	return strongest, found
}

func chooseStatusMedicine(mem *state.Mem, status string) (uint8, bool) {
	var specific uint8
	switch status {
	case "poisoned":
		specific = itemAntidote
	case "burned":
		specific = itemBurnHeal
	case "frozen":
		specific = itemIceHeal
	case "asleep":
		specific = itemAwakening
	case "paralyzed":
		specific = itemParlyzHeal
	default:
		return 0, false
	}
	for _, item := range []uint8{specific, itemFullHeal, itemFullRestore} {
		if bagHasItem(mem, item) {
			return item, true
		}
	}
	return 0, false
}

func bagHasItem(mem *state.Mem, item uint8) bool {
	_, qty := bagEntry(mem, item)
	return qty > 0
}

// ppRecoverySlot returns the first live bench mon that has at least one known
// move with current PP. This is a dead-turn escape, not team strategy.
func ppRecoverySlot(mem *state.Mem) (int, bool) {
	party := state.DecodeParty(mem)
	active := int(mem.U8(sym.PlayerMonNumber))
	for slot, mon := range party.Mons {
		if slot != active && !mon.Fainted() && monHasCurrentPP(mon) {
			return slot, true
		}
	}
	return 0, false
}

func monHasCurrentPP(mon state.Mon) bool {
	for i, id := range mon.Moves {
		if id != 0 && mon.PP[i] > 0 {
			return true
		}
	}
	return false
}

func livePartyHasCurrentPP(mem *state.Mem) bool {
	for _, mon := range state.DecodeParty(mem).Mons {
		if !mon.Fainted() && monHasCurrentPP(mon) {
			return true
		}
	}
	return false
}

// UseBattleMedicine uses one medicine item on one party slot. Unlike a ball,
// medicine opens USE_ITEM_PARTY_MENU and needs an explicit target. Success is
// proven from RAM: target HP/status changes and the bag count drops by one.
// It returns as soon as both facts are visible, before a following enemy hit
// can hide the heal in a later HP snapshot; Battle resumes the turn itself.
func UseBattleMedicine(m *emu.Emu, item uint8, slot int) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.DecodeBattle(&mem) == nil {
		return fmt.Errorf("skill: UseBattleMedicine: no battle in progress on map %#04x", m.Peek8(sym.CurMap))
	}
	party := state.DecodeParty(&mem)
	if slot < 0 || slot >= len(party.Mons) {
		return fmt.Errorf("skill: UseBattleMedicine: slot %d out of range for a party of %d", slot, len(party.Mons))
	}
	beforeMon := party.Mons[slot]
	idx, beforeQty := bagEntry(&mem, item)
	if idx < 0 || beforeQty < 1 {
		return fmt.Errorf("skill: UseBattleMedicine: %w (id %#02x)", ErrNotInBag, item)
	}

	if err := waitBattleMainMenu(m); err != nil {
		return fmt.Errorf("skill: UseBattleMedicine: %w", err)
	}
	if err := selectItemEntry(m); err != nil {
		return err
	}
	m.Tap(emu.A, 3, 7)
	if _, err := m.StepUntil(bagMenuBudget, func(m *emu.Emu) bool {
		return battleScreenHas(m, bagMenuMarker)
	}); err != nil {
		return fmt.Errorf("skill: UseBattleMedicine: bag list did not open within %d frames", bagMenuBudget)
	}
	if err := selectBagEntry(m, idx); err != nil {
		return fmt.Errorf("skill: UseBattleMedicine: %w", err)
	}
	if _, err := m.StepUntil(itemUsePartyBudget, useItemPartyMenuUp); err != nil {
		return fmt.Errorf("skill: UseBattleMedicine: medicine target menu did not open within %d frames", itemUsePartyBudget)
	}
	if err := SelectPartySlot(m, slot); err != nil {
		return fmt.Errorf("skill: UseBattleMedicine: %w", err)
	}

	effectObserved := false
	start := m.FrameCount()
	for int(m.FrameCount()-start) <= bagUseBudget {
		state.Snapshot(m, &mem)
		party = state.DecodeParty(&mem)
		if slot < len(party.Mons) {
			afterMon := party.Mons[slot]
			if afterMon.HP > beforeMon.HP || (beforeMon.Status != 0 && afterMon.Status == 0) {
				effectObserved = true
			}
		}
		_, afterQty := bagEntry(&mem, item)
		if afterQty == beforeQty-1 {
			if !effectObserved {
				return fmt.Errorf("skill: UseBattleMedicine: item %#02x was consumed but slot %d showed no HP/status effect", item, slot)
			}
			return nil
		}
		if state.DecodeBattle(&mem) == nil {
			return fmt.Errorf("skill: UseBattleMedicine: battle ended before item %#02x consumption/effect was verified", item)
		}
		m.Tap(emu.A, 3, 7)
	}
	state.Snapshot(m, &mem)
	_, afterQty := bagEntry(&mem, item)
	return fmt.Errorf("skill: UseBattleMedicine: item %#02x did not complete within %d frames (bag %d -> %d, effect=%t)",
		item, bagUseBudget, beforeQty, afterQty, effectObserved)
}

// selectItemEntry moves the 2x2 battle-main-menu cursor to ITEM (left column,
// row 1), verifying each transition rather than assuming it starts on FIGHT.
func selectItemEntry(m *emu.Emu) error {
	atItem := func(m *emu.Emu) bool {
		return m.Peek8(sym.TopMenuItemX) == battleMenuLeftX && int(m.Peek8(sym.CurrentMenuItem)) == 1
	}
	for i := 0; i < 8; i++ {
		if atItem(m) {
			return nil
		}
		prevX, prevRow := m.Peek8(sym.TopMenuItemX), int(m.Peek8(sym.CurrentMenuItem))
		var btn emu.Button
		switch {
		case prevX == battleMenuRightX:
			btn = emu.Left
		case prevRow == 0:
			btn = emu.Down
		default:
			btn = emu.Up
		}
		m.Tap(btn, 3, 7)
		if _, err := m.StepUntil(menuSettleFrames, func(m *emu.Emu) bool {
			return m.Peek8(sym.TopMenuItemX) != prevX || int(m.Peek8(sym.CurrentMenuItem)) != prevRow
		}); err != nil {
			return fmt.Errorf("skill: UseBattleMedicine: cursor stuck at x=%#02x row %d, want ITEM (x=%#02x row 1): %w",
				prevX, prevRow, battleMenuLeftX, ErrMenuStuck)
		}
	}
	return fmt.Errorf("skill: UseBattleMedicine: cursor did not reach ITEM")
}
