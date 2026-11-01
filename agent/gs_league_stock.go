package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	gsrom "github.com/maestroi/pokepilot/gs/rom"
	"github.com/maestroi/pokepilot/gs/sym"
	"github.com/maestroi/pokepilot/skill"
)

// Indigo's actual MartIndigoPlateau (pokegold/data/items/marts.asm) stocks
// each of these. Target stock is intentionally bounded; remaining money is
// reserved for campaign repairs rather than spent on every available potion.
var gsLeagueIndigoStock = [...]struct {
	item   uint8
	target int
}{
	{gsItemRevive, 3},
	{gsItemHyperPotion, 5},
	{gsItemFullRestore, 3},
	{gsItemFullHeal, 2},
}

const gsLeagueMoneyReserve = 1000

// gsLeagueStockShortfall never requests a new inventory type when the 20-slot
// Items pocket is full. Stacking an already owned item remains legal.
func gsLeagueStockShortfall(inv game.InventoryState, item uint8, want int) int {
	have := gsLeagueItemQuantity(inv, item)
	if have >= want || want <= 0 {
		return 0
	}
	used := 0
	for _, it := range inv.Items {
		if it.Pocket == game.MachinePocketItems {
			used++
		}
	}
	if used >= sym.MaxItems && have == 0 {
		return 0
	}
	return min(want-have, 99)
}

// gsLeagueStockAtIndigo is called exactly once before entering Will, never
// after the irreversible room lock. Each buy uses the regular Mart UI and is
// proven by actual money and bag changes in skill.Buy. A poor or full-bag
// campaign can proceed with existing supplies rather than inventing cash or
// failing a mandatory badge/League story boundary.
func gsLeagueStockAtIndigo(m *emu.Emu, romData []byte, profile *gsprofile.Profile) error {
	if m == nil || profile == nil {
		return fmt.Errorf("%w: missing Indigo stocking emulator", errGSSecondBadgeUnexpectedState)
	}
	center, err := gsOpeningMapID("INDIGO_PLATEAU_POKECENTER_1F")
	if err != nil {
		return err
	}
	world := profile.DecodeOverworld(m)
	if !world.Controllable || world.NativeMapID != center {
		return fmt.Errorf("%w: Indigo Mart purchases require the controllable Pokecenter lobby", errGSSecondBadgeUnexpectedState)
	}
	const (
		standX uint8 = 11
		standY uint8 = 8
		clerkX uint8 = 11
		clerkY uint8 = 7
	)
	for _, stock := range gsLeagueIndigoStock {
		inv := profile.DecodeInventory(m)
		short := gsLeagueStockShortfall(inv, stock.item, stock.target)
		if short <= 0 {
			continue
		}
		attr, err := gsrom.LookupItem(romData, stock.item)
		if err != nil || attr.Price == 0 {
			return fmt.Errorf("gen2 Indigo Mart: lookup price for item %#02x: %v", stock.item, err)
		}
		affordable := (int(inv.Money) - gsLeagueMoneyReserve) / int(attr.Price)
		if affordable <= 0 {
			continue
		}
		qty := min(short, affordable)
		if err := gsSecondBadgeFaceFrom(m, romData, profile,
			skill.ExactNativeDestination(center, standX, standY),
			clerkX, clerkY, "league:indigo-stock"); err != nil {
			return fmt.Errorf("gen2 Indigo Mart: reach clerk for item %#02x: %w", stock.item, err)
		}
		if err := skill.Buy(m, stock.item, qty); err != nil {
			return fmt.Errorf("gen2 Indigo Mart: buy %d of native item %#02x: %w", qty, stock.item, err)
		}
	}
	return nil
}
