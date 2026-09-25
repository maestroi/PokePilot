package game

// OverworldBlackoutState is the portable runtime view needed to distinguish an
// out-of-battle party wipe from ordinary movement/dialogue and to recognize the
// eventual respawn boundary. Profiles own the native status bits, party layout,
// and saved respawn-map encoding.
type OverworldBlackoutState struct {
	BlackoutInProgress bool
	PartyAllFainted    bool
	RespawnNativeMapID uint16
}

// OverworldBlackoutDecoder hides game-specific blackout/respawn RAM details
// from reusable Travel and movement orchestration.
type OverworldBlackoutDecoder interface {
	DecodeOverworldBlackout(MemoryReader) OverworldBlackoutState
}

// OverworldBlackoutProfile is a game profile exposing portable blackout state.
type OverworldBlackoutProfile interface {
	GameProfile
	OverworldBlackoutDecoder
}
