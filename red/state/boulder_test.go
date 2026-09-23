package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodeBouldersKeepsOffScreenDropsHidden(t *testing.T) {
	var m Mem
	writeLiveSlot(&m, 2, 5, 15, BoulderPictureID)
	writeLiveSlot(&m, 4, 7, 5, 0x01)
	writeLiveSlot(&m, 7, 17, 13, BoulderPictureID)

	// Off-screen objects use image index ff but are still on the map: the
	// puzzle needs them.
	writeLiveSlot(&m, 8, 4, 14, BoulderPictureID)
	m[sym.SpritePlayerStateData1+8*0x10+0x02] = 0xff

	// A toggle-hidden boulder (Victory Road 3F after the hole) is gone.
	writeLiveSlot(&m, 9, 23, 15, BoulderPictureID)
	m[sym.SpritePlayerStateData1+9*0x10+0x02] = 0xff
	m[sym.ToggleableObjectList] = 9
	m[sym.ToggleableObjectList+1] = 3
	m[sym.ToggleableObjectList+2] = 0xff
	m[sym.ToggleableObjectFlags] = 1 << 3

	got := DecodeBoulders(&m)
	want := []BoulderState{
		{Slot: 2, X: 5, Y: 15},
		{Slot: 7, X: 17, Y: 13},
		{Slot: 8, X: 4, Y: 14},
	}
	if len(got) != len(want) {
		t.Fatalf("DecodeBoulders returned %d boulder(s), want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("boulder %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
