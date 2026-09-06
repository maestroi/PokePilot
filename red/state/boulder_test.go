package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodeBouldersFiltersLiveSpriteType(t *testing.T) {
	var m Mem
	writeLiveSlot(&m, 2, 5, 15, BoulderPictureID)
	writeLiveSlot(&m, 4, 7, 5, 0x01)
	writeLiveSlot(&m, 7, 17, 13, BoulderPictureID)

	// A hidden boulder keeps a picture ID but uses image index ff; the shared
	// sprite decoder must keep it out of the live puzzle state.
	writeLiveSlot(&m, 9, 23, 15, BoulderPictureID)
	m[sym.SpritePlayerStateData1+9*0x10+0x02] = 0xff

	got := DecodeBoulders(&m)
	want := []BoulderState{
		{Slot: 2, X: 5, Y: 15},
		{Slot: 7, X: 17, Y: 13},
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
