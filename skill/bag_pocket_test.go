package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
)

// fakePocketPack cycles Gen-II-style pockets left on each Left press.
type fakePocketPack struct {
	pocket int
	taps   int
}

var fakePockets = []game.MachinePocket{
	game.MachinePocketItems, game.MachinePocketBalls, game.MachinePocketKeyItems, game.MachinePocketTMHM,
}

func (*fakePocketPack) Peek8(uint16) byte       { return 0 }
func (*fakePocketPack) PeekInto(uint16, []byte) {}
func (*fakePocketPack) StepFrame()              {}
func (*fakePocketPack) StepFrames(int)          {}
func (p *fakePocketPack) Tap(b emu.Button, _, _ int) {
	if b == emu.Left {
		p.taps++
		p.pocket = (p.pocket + len(fakePockets) - 1) % len(fakePockets)
	}
}

type fakePocketDecoder struct{ pack *fakePocketPack }

func (d fakePocketDecoder) DecodeListMenu(game.MemoryReader) game.ListMenuState {
	return game.ListMenuState{Visible: true, Kind: game.ListMenuItems, Pocket: fakePockets[d.pack.pocket]}
}

func TestUseItemSwitchesToTheItemsPocket(t *testing.T) {
	inv := game.InventoryState{Items: []game.InventoryItem{
		{NativeItemID: 0x12, Quantity: 1, Pocket: game.MachinePocketItems},
		{NativeItemID: 0x05, Quantity: 5, Pocket: game.MachinePocketBalls},
		{NativeItemID: 0x04, Quantity: 20, Pocket: game.MachinePocketBalls},
	}}
	// Positions are pocket-relative: Great Ball is the second ball, not the
	// third carried item.
	if idx, qty := inventoryEntry(inv, 0x04); idx != 1 || qty != 20 {
		t.Fatalf("Great Ball entry = %d x%d, want position 1 x20", idx, qty)
	}

	pack := &fakePocketPack{pocket: 0} // the pack reopens on the Items pocket
	if err := selectItemPocketWithDecoder(pack, fakePocketDecoder{pack}, inventoryPocket(inv, 0x04)); err != nil {
		t.Fatal(err)
	}
	if fakePockets[pack.pocket] != game.MachinePocketBalls {
		t.Fatalf("ended on %q, want balls", fakePockets[pack.pocket])
	}

	// Single-list games have no pocket and must not receive any input.
	single := &fakePocketPack{}
	if err := selectItemPocketWithDecoder(single, fakePocketDecoder{single}, game.MachinePocketUnknown); err != nil || single.taps != 0 {
		t.Fatalf("single-list pocket switch: taps=%d err=%v", single.taps, err)
	}
}
