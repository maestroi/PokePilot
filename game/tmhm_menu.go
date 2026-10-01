package game

// TMHMMenuDecoder exposes the on-screen wording of the party-select menu that
// appears after choosing to teach a TM/HM. The exact phrase is a game fact, not
// a shared invariant: Red and Blue render "Use TM on which #MON?", while Yellow
// renders "Teach to which #MON?". Generic execution matches the profile's
// marker and never a hardcoded phrase, so a Gen-I revision with different menu
// wording cannot stall the teach flow on a marker that is only true of one
// cartridge.
type TMHMMenuDecoder interface {
	// TMHMPartyMenuMarker returns a substring of the rendered screen that
	// identifies the TM/HM party-select menu for this game.
	TMHMPartyMenuMarker() string
}

// TMHMMenuProfile is a game profile that exposes the TM/HM teach-menu wording.
type TMHMMenuProfile interface {
	GameProfile
	TMHMMenuDecoder
}
