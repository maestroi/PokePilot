package skill

import (
	"fmt"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/world"
)

func TestDebugRocketB4FLockedDoorGuards(t *testing.T) {
	romData := rocketHideoutROM(t)
	h, err := rom.ParseMap(romData, rocketHideoutB4FMap)
	if err != nil {
		t.Fatalf("ParseMap(B4F): %v", err)
	}
	blocks, err := rom.Blocks(romData, h)
	if err != nil {
		t.Fatalf("Blocks(B4F): %v", err)
	}
	blocks = append([]byte(nil), blocks...)
	idx := 5*int(h.WidthBlocks) + 12
	if idx < 0 || idx >= len(blocks) {
		t.Fatalf("door block index %d outside %d blocks", idx, len(blocks))
	}
	blocks[idx] = 0x2d // RocketHideoutB4FDoorCallbackScript's locked-door block.
	g, err := world.BuildFromBlocks(romData, h, blocks)
	if err != nil {
		t.Fatalf("BuildFromBlocks(B4F locked): %v", err)
	}

	dump := func() string {
		var b strings.Builder
		for y := 8; y <= 16; y++ {
			fmt.Fprintf(&b, "%02d ", y)
			for x := 17; x <= 29; x++ {
				ch := '#'
				if g.Walkable(x, y) {
					ch = '.'
				}
				switch {
				case x == int(rocketB4FEntry.X) && y == int(rocketB4FEntry.Y):
					ch = 'E'
				case x == int(rocketGuard1X) && y == int(rocketGuard1Y):
					ch = '1'
				case x == int(rocketGuard2X) && y == int(rocketGuard2Y):
					ch = '2'
				}
				b.WriteRune(ch)
			}
			b.WriteByte('\n')
		}
		return b.String()
	}

	for _, tc := range []struct {
		name string
		x, y uint8
	}{
		{"guard 1", rocketGuard1X, rocketGuard1Y},
		{"guard 2", rocketGuard2X, rocketGuard2Y},
	} {
		steps, push, err := world.FindPathAdjacent(g, int(rocketB4FEntry.X), int(rocketB4FEntry.Y), int(tc.x), int(tc.y), nil)
		if err != nil {
			t.Fatalf("locked B4F entry -> %s: %v\n%s", tc.name, err, dump())
		}
		t.Logf("locked B4F entry -> %s: %d steps, push=%s\n%s", tc.name, len(steps), push, dump())
	}
}
