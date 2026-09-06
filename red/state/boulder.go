package state

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

// DecodeBoulders returns every currently visible SPRITE_BOULDER in stable
// sprite-slot order. Hidden/removed boulders are excluded by DecodeSprites,
// which matters for Victory Road's 3F hole transition where the pushed
// boulder is hidden on 3F and shown on 2F.
func DecodeBoulders(m *Mem) []BoulderState {
	sprites := DecodeSprites(m)
	out := make([]BoulderState, 0, len(sprites))
	for _, sprite := range sprites {
		if sprite.PictureID != BoulderPictureID {
			continue
		}
		out = append(out, BoulderState{Slot: sprite.Slot, X: sprite.X, Y: sprite.Y})
	}
	return out
}
