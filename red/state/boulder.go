package state

import "github.com/maestroi/pokepilot/red/sym"

// BoulderPictureID is SPRITE_BOULDER in Pokémon Red's sprite constants.
// Keeping the identity next to the live decoder lets puzzle code distinguish
// movable Strength objects from ordinary NPC/item blockers without consulting
// map-specific object indices.
const BoulderPictureID uint8 = 0x3f

// BoulderState is one live movable boulder decoded from sprite RAM.
// Slot is the stable sprite slot for the currently loaded map; X/Y are game
// tile coordinates with the ROM's +4 storage bias already removed.
type BoulderState struct {
	Slot int
	X, Y int
}

// DecodeBoulders returns every present SPRITE_BOULDER in stable sprite-slot
// order, on-screen or not. Off-screen objects carry the same $ff image index
// as hidden ones, so DecodeSprites would drop a boulder the puzzle still needs
// (Victory Road 2F's west-switch boulder from the 1F ladder, run-1biaubd9xooqm).
// Presence comes from the toggleable-object flags instead, which still drop
// Victory Road 3F's boulder once the hole script hides it.
func DecodeBoulders(m *Mem) []BoulderState {
	hidden := HiddenObjectIDs(m)
	var out []BoulderState
	for slot := spriteFirstSlot; slot <= spriteLastSlot; slot++ {
		data1 := sym.SpritePlayerStateData1 + uint16(slot)*spriteSlotSize
		if m.U8(data1+spritePictureID) != BoulderPictureID || hidden[uint8(slot)] {
			continue
		}
		data2 := sym.SpriteStateData2 + uint16(slot)*spriteSlotSize
		out = append(out, BoulderState{Slot: slot, X: int(m.U8(data2+spriteMapX)) - 4, Y: int(m.U8(data2+spriteMapY)) - 4})
	}
	return out
}
