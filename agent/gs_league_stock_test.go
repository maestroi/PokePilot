package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

func TestGSLeagueStockShortfallRespectsExistingStacksAndCapacity(t *testing.T) {
	have := gsLeagueTestInv(gsLeagueItem(gsItemRevive, 2))
	if got := gsLeagueStockShortfall(have, gsItemRevive, 3); got != 1 {
		t.Fatalf("wanted one more revive, got %d", got)
	}
	if got := gsLeagueStockShortfall(have, gsItemRevive, 2); got != 0 {
		t.Fatalf("already stocked, wanted no purchase, got %d", got)
	}
	var full []game.InventoryItem
	for i := 0; i < sym.MaxItems; i++ {
		full = append(full, gsLeagueItem(uint8(0x50+i), 1))
	}
	stock := gsLeagueTestInv(full...)
	if got := gsLeagueStockShortfall(stock, gsItemFullRestore, 3); got != 0 {
		t.Fatalf("must not buy a new item type in full pack, got %d", got)
	}
	stock.Items[0].NativeItemID = uint16(gsItemFullRestore)
	if got := gsLeagueStockShortfall(stock, gsItemFullRestore, 3); got != 2 {
		t.Fatalf("stacking an existing medicine in full pack is legal, got %d", got)
	}
}

func TestGSLeagueIndigoStockUsesActualMartItems(t *testing.T) {
	// Only items stocked by MartIndigoPlateau in the vendored retail
	// tables may be selected by the pre-Will shop controller.
	want := []struct {
		id     uint8
		target int
	}{
		{gsItemRevive, 3},
		{gsItemHyperPotion, 5},
		{gsItemFullRestore, 3},
		{gsItemFullHeal, 2},
	}
	if len(gsLeagueIndigoStock) != len(want) {
		t.Fatalf("Indigo stock list=%v want %v", gsLeagueIndigoStock, want)
	}
	for i, want := range want {
		if gsLeagueIndigoStock[i].item != want.id || gsLeagueIndigoStock[i].target != want.target {
			t.Fatalf("Indigo slot %d=%+v want %+v", i, gsLeagueIndigoStock[i], want)
		}
	}
}
