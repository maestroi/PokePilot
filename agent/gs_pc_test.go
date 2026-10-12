package agent

import "testing"

func TestGSDepositSlotStoresLowestSurplusMember(t *testing.T) {
	levels := []int{40, 7, 13, 22, 9, 31}
	// Lead (slot 0) and the Surf carrier (slot 2) are never stored.
	if slot, ok := gsDepositSlot(levels, map[int]bool{0: true, 2: true}); !ok || slot != 1 {
		t.Fatalf("deposit slot = %d,%v, want the level-7 member at 1", slot, ok)
	}
	if slot, ok := gsDepositSlot(levels, map[int]bool{1: true, 2: true}); !ok || slot != 4 {
		t.Fatalf("deposit slot = %d,%v, want level-9 member at 4", slot, ok)
	}
	if _, ok := gsDepositSlot([]int{40, 20}, map[int]bool{1: true}); ok {
		t.Fatal("a party of only the lead and a kept carrier has no surplus member")
	}
}
