package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

// writeLiveSlot places a live sprite in the given slot using the real RAM
// layout (sym base addresses + literal field offsets), independent of the
// decoder's own constants. Coordinates are stored with the ROM's +4 bias, as
// the game stores them.
func writeLiveSlot(m *Mem, slot, x, y int, pictureID uint8) {
	data1 := sym.SpritePlayerStateData1 + uint16(slot)*0x10
	data2 := sym.SpriteStateData2 + uint16(slot)*0x10
	m[data1+0x00] = pictureID     // SPRITESTATEDATA1_PICTUREID
	m[data1+0x02] = 0x00         // SPRITESTATEDATA1_IMAGEINDEX (enabled)
	m[data2+0x04] = uint8(y + 4) // SPRITESTATEDATA2_MAPY (with +4 bias)
	m[data2+0x05] = uint8(x + 4) // SPRITESTATEDATA2_MAPX (with +4 bias)
}

func TestDecodeSprites(t *testing.T) {
	var m Mem

	// Slot 0 is the player. Make it look active; it must still be excluded.
	writeLiveSlot(&m, 0, 1, 1, 0x01)

	// Slot 1: unused (data1[0] == 0). Skipped. Left zeroed.

	// Slot 2: hidden or removed (data1[0] != 0 but data1[0x02] == 0xff).
	// Skipped. A picked-up item ball keeps a non-zero picture ID.
	m[sym.SpritePlayerStateData1+2*0x10+0x00] = 0x02 // picture ID
	m[sym.SpritePlayerStateData1+2*0x10+0x02] = 0xff // image index disabled

	// Slot 3: live, coordinates (10,12), picture ID 0x03.
	writeLiveSlot(&m, 3, 10, 12, 0x03)
	// Deliberately zero the data2 +0x0d scratch byte (SPRITESTATEDATA2_PICTUREID,
	// which map_sprites.asm zeroes for every slot after tile patterns load) so a
	// decoder that regressed to reading it for liveness would skip this slot.
	m[sym.SpriteStateData2+3*0x10+0x0d] = 0

	// Slot 15: live, upper bound. Coordinates (20,24), picture ID 0x0f.
	writeLiveSlot(&m, 15, 20, 24, 0x0f)

	got := DecodeSprites(&m)

	want := []SpriteState{
		{Slot: 3, X: 10, Y: 12, PictureID: 0x03},
		{Slot: 15, X: 20, Y: 24, PictureID: 0x0f},
	}
	if len(got) != len(want) {
		t.Fatalf("DecodeSprites returned %d sprite(s), want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sprite %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}
