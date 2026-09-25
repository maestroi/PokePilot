package game

import "testing"

func TestBattleResourcesItemIndexAndQuantity(t *testing.T) {
	s := BattleResourcesState{Bag: []InventoryItem{
		{NativeItemID: 300, Quantity: 2},
		{NativeItemID: 500, Quantity: 4},
		{NativeItemID: 300, Quantity: 1},
	}}
	if idx, ok := s.ItemIndex(500); !ok || idx != 1 {
		t.Fatalf("ItemIndex(500)=%d,%v want 1,true", idx, ok)
	}
	if qty := s.ItemQuantity(300); qty != 3 {
		t.Fatalf("ItemQuantity(300)=%d want 3", qty)
	}
	if _, ok := s.ItemIndex(999); ok {
		t.Fatal("ItemIndex(999) unexpectedly found")
	}
}
