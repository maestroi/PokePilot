package game

// CaptureState contains the durable evidence used to prove that a capture was
// acquired. Species use the game's native identifier; OwnedDex uses the
// profile's canonical dex-number space for the loaded game.
type CaptureState struct {
	PartySpecies     []uint16
	ActiveBoxSpecies []uint16
	OwnedDex         []uint16
}

// CaptureDecoder hides game-specific party, storage and dex RAM layouts.
type CaptureDecoder interface {
	DecodeCapture(MemoryReader) CaptureState
}

// CaptureProfile combines durable capture evidence with inventory and the
// game-specific mappings needed by ordinary wild capture.
type CaptureProfile interface {
	GameProfile
	CaptureDecoder
	InventoryDecoder

	CaptureDexNumber(romData []byte, nativeSpecies uint16) (uint16, bool)
	OrdinaryCaptureBallOrder() []uint16
}
