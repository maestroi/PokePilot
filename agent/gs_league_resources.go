package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

// Gold/Silver native medicine IDs from pokegold/constants/item_constants.asm.
// These are NOT the corresponding Red/Blue IDs.
const (
	gsItemAntidote    uint8 = 0x09
	gsItemBurnHeal    uint8 = 0x0a
	gsItemIceHeal     uint8 = 0x0b
	gsItemAwakening   uint8 = 0x0c
	gsItemParlyzHeal  uint8 = 0x0d
	gsItemFullRestore uint8 = 0x0e
	gsItemMaxPotion   uint8 = 0x0f
	gsItemHyperPotion uint8 = 0x10
	gsItemSuperPotion uint8 = 0x11
	gsItemPotion      uint8 = 0x12
	gsItemMaxElixer   uint8 = 0x15
	gsItemFullHeal    uint8 = 0x26
	gsItemRevive      uint8 = 0x27
	gsItemMaxRevive   uint8 = 0x28
	gsItemFreshWater  uint8 = 0x2e
	gsItemSodaPop     uint8 = 0x2f
	gsItemLemonade    uint8 = 0x30
	gsItemEther       uint8 = 0x3f
	gsItemMaxEther    uint8 = 0x40
	gsItemElixer      uint8 = 0x41

	gsLeagueResourceHPFloor = 55
	gsLeagueMaxFieldItems   = 6
)

type gsLeagueMedicine struct {
	item uint8
	slot int
	kind string
}

func gsLeagueItemQuantity(inv game.InventoryState, item uint8) int {
	total := 0
	for _, entry := range inv.Items {
		if entry.Pocket == game.MachinePocketItems && entry.NativeItemID == uint16(item) {
			total += entry.Quantity
		}
	}
	return total
}

func gsLeagueChooseMedicine(party []game.BattlePartyMon, inv game.InventoryState) (gsLeagueMedicine, bool) {
	qty := func(id uint8) bool { return gsLeagueItemQuantity(inv, id) > 0 }
	live := 0
	fainted := -1
	for i, mon := range party {
		if mon.HP > 0 {
			live++
		} else if fainted < 0 && mon.MaxHP > 0 {
			fainted = i
		}
	}
	// Keep at least two members available if a party of two or more exists.
	// Do not revive everything greedily: future fights must share the stock.
	if fainted >= 0 && live < min(len(party), 2) {
		for _, item := range []uint8{gsItemRevive, gsItemMaxRevive} {
			if qty(item) {
				return gsLeagueMedicine{item: item, slot: fainted, kind: "revive"}, true
			}
		}
	}
	for i, mon := range party {
		if mon.HP == 0 || mon.Status == "" {
			continue
		}
		item := uint8(0)
		switch mon.Status {
		case "poisoned":
			item = gsItemAntidote
		case "burned":
			item = gsItemBurnHeal
		case "frozen":
			item = gsItemIceHeal
		case "asleep":
			item = gsItemAwakening
		case "paralyzed":
			item = gsItemParlyzHeal
		}
		if item != 0 && qty(item) {
			return gsLeagueMedicine{item: item, slot: i, kind: "status"}, true
		}
		for _, general := range []uint8{gsItemFullHeal, gsItemFullRestore} {
			if qty(general) {
				return gsLeagueMedicine{item: general, slot: i, kind: "status"}, true
			}
		}
	}
	// Pick the weakest living member, not simply the first team slot.
	worst := -1
	worstPercent := 101
	for i, mon := range party {
		if mon.HP == 0 || mon.MaxHP == 0 {
			continue
		}
		percent := int(mon.HP) * 100 / int(mon.MaxHP)
		if percent < gsLeagueResourceHPFloor && percent < worstPercent {
			worst, worstPercent = i, percent
		}
	}
	if worst >= 0 {
		mon := party[worst]
		missing := (int(mon.MaxHP)*gsLeagueResourceHPFloor+99)/100 - int(mon.HP)
		// Use the smallest available heal sufficient to reach the floor.
		heals := [...]struct {
			item   uint8
			amount int
		}{
			{gsItemPotion, 20}, {gsItemSuperPotion, 50},
			{gsItemFreshWater, 50}, {gsItemSodaPop, 60},
			{gsItemLemonade, 80}, {gsItemHyperPotion, 200},
			{gsItemMaxPotion, 1000000}, {gsItemFullRestore, 1000000},
		}
		var available uint8
		for _, med := range heals {
			if !qty(med.item) {
				continue
			}
			available = med.item
			if med.amount >= missing {
				return gsLeagueMedicine{item: med.item, slot: worst, kind: "HP"}, true
			}
		}
		if available != 0 {
			return gsLeagueMedicine{item: available, slot: worst, kind: "HP"}, true
		}
	}
	// Do not enter another trainer fight with a live member whose entire
	// moveset has zero PP if a recovery item is available. Ethers target
	// the emptiest move through the reusable field-item selector.
	for i, mon := range party {
		if mon.HP == 0 {
			continue
		}
		total := 0
		known := 0
		for _, move := range mon.Moves {
			if move.NativeMoveID != 0 {
				known++
				total += int(move.PP)
			}
		}
		// Eggs or malformed/empty movesets cannot receive Ether.
		if known == 0 || total >= 4 {
			continue
		}
		for _, id := range []uint8{gsItemEther, gsItemMaxEther, gsItemElixer, gsItemMaxElixer} {
			if qty(id) {
				return gsLeagueMedicine{item: id, slot: i, kind: "PP"}, true
			}
		}
	}
	return gsLeagueMedicine{}, false
}

// gsLeaguePrepare does not invent a free heal inside the locked League.
// Re-observe party and inventory after *every* native menu action; the shared
// UseFieldItem executor must prove both a positive party effect AND a one-unit
// inventory reduction. Cap finite actions per encounter to preserve supplies
// for later members, and never press any UI while a cutscene owns control.
func gsLeaguePrepare(m *emu.Emu, profile *gsprofile.Profile, encounter string) error {
	if m == nil || profile == nil {
		return fmt.Errorf("%w: missing Gen2 League resource state", errGSSecondBadgeUnexpectedState)
	}
	world := profile.DecodeOverworld(m)
	if !world.Controllable {
		return fmt.Errorf("%w: League resource preparation requires controllable overworld (%s)", errGSSecondBadgeUnexpectedState, encounter)
	}
	for n := 0; n < gsLeagueMaxFieldItems; n++ {
		party := profile.DecodeBattleResources(m).Party
		inv := profile.DecodeInventory(m)
		choice, ok := gsLeagueChooseMedicine(party, inv)
		if !ok {
			return nil
		}
		if err := skill.UseFieldItem(m, choice.item, choice.slot); err != nil {
			return fmt.Errorf("gen2 %s: %s item %#02x on party slot %d: %w",
				encounter, choice.kind, choice.item, choice.slot, err)
		}
	}
	return nil
}
