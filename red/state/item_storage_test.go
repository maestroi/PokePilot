package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodePCItemStorage(t *testing.T) {
	var mem Mem
	mem[sym.PCItemCount] = 3
	mem[sym.PCItems+0] = 0x31
	mem[sym.PCItems+1] = 2
	mem[sym.PCItems+2] = 0xC9
	mem[sym.PCItems+3] = 1
	mem[sym.PCItems+4] = 0x28
	mem[sym.PCItems+5] = 4

	got := DecodePCItemStorage(&mem)
	if len(got.Items) != 3 {
		t.Fatalf("items = %+v, want 3 stacks", got.Items)
	}
	want := []BagItem{{ID: 0x31, Quantity: 2}, {ID: 0xC9, Quantity: 1}, {ID: 0x28, Quantity: 4}}
	for i := range want {
		if got.Items[i] != want[i] {
			t.Fatalf("item %d = %+v, want %+v", i, got.Items[i], want[i])
		}
	}
}

func TestDecodePCItemStorageClampsCorruptCount(t *testing.T) {
	var mem Mem
	mem[sym.PCItemCount] = 0xff
	mem[sym.PCItems] = 0xff
	got := DecodePCItemStorage(&mem)
	if len(got.Items) != 0 {
		t.Fatalf("corrupt storage decoded %d items, want 0", len(got.Items))
	}
}
