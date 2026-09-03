package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// Gen 1 item IDs used by the battle resource policy. They come directly from
// pokered/constants/item_constants.asm. Keep this table small and explicit:
// the policy only needs ordinary medicine, not every battle item in the ROM.
const (
	itemAntidote     uint8 = 0x0b
	itemBurnHeal     uint8 = 0x0c
	itemIceHeal      uint8 = 0x0d
	itemAwakening    uint8 = 0x0e
	itemParlyzHeal   uint8 = 0x0f
	itemFullRestore  uint8 = 0x10
	itemMaxPotion    uint8 = 0x11
	itemHyperPotion  uint8 = 0x12
	itemSuperPotion  uint8 = 0x13
	itemPotion       uint8 = 0x14
	itemFullHeal     uint8 = 0x34
	itemFreshWater   uint8 = 0x3c
	itemSodaPop      uint8 = 0x3d
	itemLemonade     uint8 = 0x3e
)

// battleItemUseCap prevents a deterministic healing loop. Even if an enemy
// repeatedly knocks the active mon back below the heal line, Battle will spend
// at most this many automatic item turns before it commits to fighting or
// another recovery path.
const battleItemUseCap = 4

// ErrPartyOutOfPP means every non-fainted party member has no current PP in
// any known move. This is distinct from ErrNoUsableMove: a temporarily
// disabled move can make the active mon unable to act even though the party
// still owns PP. Callers can treat this error as an explicit Center/PP-recovery
// signal instead of retrying the same impossible move menu forever.
var ErrPartyOutOfPP = errors.New("skill: all live party members are out of PP")

type battleMedicineChoice struct {
	Item   uint8
	Slot   int
	Reason string
}

type hpMedicine struct {
	item uint8
	heal int
}

// Ordered by effective heal size so chooseHPMedicine naturally picks the
// smallest item that can cover the missing HP. Full heals come last to avoid
// spending them when a finite ordinary drink/potion is enough.
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

// chooseBattleMedicine returns one conservative automatic medicine action for
// the current active Pokémon. It makes no long-horizon economy judgement; its
// job is only to keep a battle from deterministically throwing away a nearly
// fainted or incapacitated active mon.
//
// HP medicine is considered only at one-third HP or below. Status medicine is
// considered whenever the active mon has a status for which the bag has a
// matching cure. When both are true, FULL RESTORE is preferred if present
// because it resolves both problems in one battle turn.
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
		missing := int(mon.MaxHP - mon.HP)
		if item, ok := chooseHPMedicine(mem, missing); ok {
			return battleMedicineChoice{
				Item:   item,
				Slot:   slot,
				Reason: fmt.Sprintf("active HP %d/%d", mon.HP, mon.MaxHP),
			}, true
		}
	}
	if status != "" {
		if item, ok := chooseStatusMedicine(mem, status); ok {
			return battleMedicineChoice{
				Item:   item,
				Slot:   slot,
				Reason: "active is " + status,
			}, true
		}
	}
	return battleMedicineChoice{}, false
}

func chooseHPMedicine(mem *state.Mem, missing int) (uint8, bool) {
	bestAvailable := -1
	for _, med := range hpMedicines {
		if !bagHasItem(mem, med.item) {
			continue
		}
		bestAvailable = med.item
		if med.heal >= missing {
			return med.item, true
		}
	}
	if bestAvailable >= 0 {
		return uint8(bestAvailable), true
	}
	return 0, false
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
	if bagHasItem(mem, specific) {
		return specific, true
	}
	if bagHasItem(mem, itemFullHeal) {
		return itemFullHeal, true
	}
	if bagHasItem(mem, itemFullRestore) {
		return itemFullRestore, true
	}
	return 0, false
}

func bagHasItem(mem *state.Mem, item uint8) bool {
	_, qty := bagEntry(mem, item)
	return qty > 0
}

// ppRecoverySlot returns the first live bench slot that has at least one move
// with current PP. Battle calls this only when the active battle state has no
// usable move, so switching is a bounded escape from an otherwise dead move
// menu rather than a general team-composition policy.
func ppRecoverySlot(mem *state.Mem) (int, bool) {
	party := state.DecodeParty(mem)
	active := int(mem.U8(sym.PlayerMonNumber))
	for slot, mon := range party.Mons {
		if slot == active || mon.Fainted() || !monHasCurrentPP(mon) {
			continue
		}
		return slot, true
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

// UseBattleMedicine uses one medicine item on one party slot while a battle
// is in progress. Unlike UseItem (which is sufficient for balls), Gen 1
// medicine opens USE_ITEM_PARTY_MENU and therefore requires an explicit
// target. This helper drives that target menu and verifies two positive facts:
// the target's HP/status changed and the bag quantity dropped by one.
//
// It returns as soon as those facts are observed. The enemy's response may
// still be animating; Battle's outer state machine owns that continuation.
// Returning early is intentional: waiting until the next main menu could hide
// a real heal behind the damage from the enemy's following attack.
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

// selectItemEntry drives the battle main menu's 2x2 cursor to ITEM: left
// column, row 1. It mirrors selectFightEntry/SwitchActive and verifies every
// movement rather than assuming the cursor starts on FIGHT.
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
