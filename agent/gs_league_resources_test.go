package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func gsLeagueTestInv(items ...game.InventoryItem) game.InventoryState {
	for i := range items {
		items[i].Pocket = game.MachinePocketItems
	}
	return game.InventoryState{Items: items}
}

func gsLeagueItem(id uint8, qty int) game.InventoryItem {
	return game.InventoryItem{NativeItemID: uint16(id), Quantity: qty}
}

func TestGSLeagueMedicineUsesNativeGen2ItemsAndChoosesRecovery(t *testing.T) {
	cases := []struct {
		name  string
		party []game.BattlePartyMon
		items []game.InventoryItem
		item  uint8
		slot  int
		ok    bool
	}{
		{
			name:  "revive when only one party member remains",
			party: []game.BattlePartyMon{{HP: 70, MaxHP: 100}, {HP: 0, MaxHP: 120}},
			items: []game.InventoryItem{gsLeagueItem(gsItemRevive, 1)},
			item:  gsItemRevive, slot: 1, ok: true,
		},
		{
			name:  "prefer specific Gen2 poison cure",
			party: []game.BattlePartyMon{{HP: 90, MaxHP: 100, Status: "poisoned"}},
			items: []game.InventoryItem{gsLeagueItem(gsItemAntidote, 1), gsLeagueItem(gsItemFullRestore, 1)},
			item:  gsItemAntidote, slot: 0, ok: true,
		},
		{
			name:  "choose smallest sufficient heal for weakest member",
			party: []game.BattlePartyMon{{HP: 40, MaxHP: 100}, {HP: 3, MaxHP: 80}},
			items: []game.InventoryItem{gsLeagueItem(gsItemPotion, 3), gsLeagueItem(gsItemSuperPotion, 2)},
			item:  gsItemSuperPotion, slot: 1, ok: true,
		},
		{
			name:  "recover exhausted PP without Red item numbers",
			party: []game.BattlePartyMon{{HP: 90, MaxHP: 100, Moves: [4]game.BattlePartyMove{{NativeMoveID: 1, PP: 0}}}},
			items: []game.InventoryItem{gsLeagueItem(gsItemEther, 1)},
			item:  gsItemEther, slot: 0, ok: true,
		},
		{
			name:  "no recovery needed",
			party: []game.BattlePartyMon{{HP: 80, MaxHP: 100, Moves: [4]game.BattlePartyMove{{NativeMoveID: 1, PP: 17}}}},
			items: []game.InventoryItem{gsLeagueItem(gsItemPotion, 3)},
		},
		{
			name:  "do not use balls as medicine",
			party: []game.BattlePartyMon{{HP: 1, MaxHP: 100}},
			items: []game.InventoryItem{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := gsLeagueChooseMedicine(c.party, gsLeagueTestInv(c.items...))
			if ok != c.ok || (ok && (got.item != c.item || got.slot != c.slot)) {
				t.Fatalf("medicine=%+v, ok=%v want item %#02x slot %d ok=%v", got, ok, c.item, c.slot, c.ok)
			}
		})
	}
}

func TestGSLeagueMedicineRejectsWrongPocketAndDoesNotSpendWithoutNeed(t *testing.T) {
	party := []game.BattlePartyMon{{HP: 5, MaxHP: 100}}
	inv := game.InventoryState{Items: []game.InventoryItem{
		{NativeItemID: uint16(gsItemPotion), Quantity: 2, Pocket: game.MachinePocketBalls},
	}}
	if choice, ok := gsLeagueChooseMedicine(party, inv); ok {
		t.Fatalf("non-Items pocket used: %+v", choice)
	}
}
