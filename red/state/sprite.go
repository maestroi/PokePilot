package state

import "github.com/maestroi/pokepilot/red/sym"

// Layout of the sprite state arrays. Both wSpritePlayerStateData1 (0xC100)
// and wSpriteStateData2 (0xC200) are 16 slots of 16 bytes each: slot 0 is the
// player and slots 1..15 hold the current map's objects. The offsets below are
// the fields this decoder reads; they are kept local to this file.
const (
	spriteSlotSize   uint16 = 0x10
	spriteFirstSlot        = 1
	spriteLastSlot         = 15

	// Offsets within one wSpritePlayerStateData1 (data1) slot.
	spritePictureID  uint16 = 0x00 // non-zero: the slot holds a sprite
	spriteImageIndex uint16 = 0x02 // $ff: hidden or removed object

	// Offsets within one wSpriteStateData2 (data2) slot.
	spriteMapY uint16 = 0x04 // stored with the ROM's +4 bias
	spriteMapX uint16 = 0x05 // stored with the ROM's +4 bias
)

// SpriteState is one live map object decoded from sprite RAM: its slot, its
// tile coordinates on the current map, and its picture ID.
type SpriteState struct {
	Slot      int
	X, Y      int
	PictureID uint8
}

// DecodeSprites reads the live map objects (sprite slots 1..15) from a RAM
// snapshot and returns them in slot order.
//
// Liveness comes from wSpritePlayerStateData1, not wSpriteStateData2: a slot
// is live when its picture ID (data1[0]) is non-zero AND its image index
// (data1[2]) is not $ff. The $ff image index marks a hidden or removed object
// (a picked-up item ball keeps a non-zero picture ID), so the second condition
// is what excludes it. The data2 picture ID byte (data2[0x0d]) is scratch that
// map_sprites.asm zeroes for every slot after tile patterns load, so it must
// not be consulted for liveness. Slot 0 is the player and is never returned.
// Coordinates come from wSpriteStateData2's map Y/X, which carry the ROM's +4
// bias, so both are decremented by 4.
func DecodeSprites(m *Mem) []SpriteState {
	var out []SpriteState
	for slot := spriteFirstSlot; slot <= spriteLastSlot; slot++ {
		data1 := sym.SpritePlayerStateData1 + uint16(slot)*spriteSlotSize
		data2 := sym.SpriteStateData2 + uint16(slot)*spriteSlotSize

		pictureID := m.U8(data1 + spritePictureID)
		if pictureID == 0 {
			continue // unused slot: zeroed at map load
		}
		if m.U8(data1+spriteImageIndex) == 0xff {
			continue // hidden or removed object
		}

		out = append(out, SpriteState{
			Slot:      slot,
			X:         int(m.U8(data2+spriteMapX)) - 4,
			Y:         int(m.U8(data2+spriteMapY)) - 4,
			PictureID: pictureID,
		})
	}
	return out
}
