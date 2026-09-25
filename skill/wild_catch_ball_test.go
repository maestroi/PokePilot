package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestOrdinaryCaptureBallPrefersProfileOrder(t *testing.T) {
	const master, ultra, great, poke uint16 = 0x01, 0x02, 0x03, ItemPokeBall
	order := []uint16{ultra, great, poke}

	if _, ok := ordinaryCaptureBall(game.InventoryState{}, order); ok {
		t.Fatal("empty inventory offered a ball")
	}
	if _, ok := ordinaryCaptureBall(game.InventoryState{
		Items: []game.InventoryItem{{NativeItemID: master, Quantity: 1}},
	}, order); ok {
		t.Fatal("capture policy spent an item absent from its ordinary-ball order")
	}

	inventory := game.InventoryState{Items: []game.InventoryItem{
		{NativeItemID: poke, Quantity: 5},
		{NativeItemID: great, Quantity: 2},
		{NativeItemID: master, Quantity: 1},
	}}
	if ball, _ := ordinaryCaptureBall(inventory, order); ball != great {
		t.Fatalf("ball = %#04x, want Great Ball", ball)
	}
	if n := ordinaryCaptureBallCount(inventory, order); n != 7 {
		t.Fatalf("ordinary ball count = %d, want 7", n)
	}

	inventory.Items = append(inventory.Items, game.InventoryItem{NativeItemID: ultra, Quantity: 1})
	if ball, _ := ordinaryCaptureBall(inventory, order); ball != ultra {
		t.Fatalf("ball = %#04x, want Ultra Ball", ball)
	}
}
