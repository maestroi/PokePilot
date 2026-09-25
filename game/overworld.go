package game

// OverworldState is the portable runtime state needed by reusable local
// movement. NativeMapID remains opaque to generic movement; adapters own its
// encoding and higher-level routing owns its interpretation.
type OverworldState struct {
	NativeMapID uint16
	X, Y        uint8

	Controllable bool
	MovementIdle bool
	InBattle     bool
	InDialogue   bool
}

// OverworldDecoder hides the RAM layout used to observe local overworld
// movement and interruption state.
type OverworldDecoder interface {
	DecodeOverworld(MemoryReader) OverworldState
}

// OverworldProfile is a game profile that exposes portable overworld runtime
// state.
type OverworldProfile interface {
	GameProfile
	OverworldDecoder
}
