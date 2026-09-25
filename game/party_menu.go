package game

// PartyMenuKind identifies the interaction that owns a visible party list.
type PartyMenuKind string

const (
	PartyMenuForcedBattle    PartyMenuKind = "forced_battle"
	PartyMenuVoluntaryBattle PartyMenuKind = "voluntary_battle"
	PartyMenuItemUse         PartyMenuKind = "item_use"
)

// PartyMenuState is the semantic state generic party-slot navigation needs.
// Cursor.Max is the highest selectable party slot.
type PartyMenuState struct {
	Visible bool
	Kind    PartyMenuKind
	Cursor  MenuCursorState
}

// PartyMenuDecoder hides game-specific party-menu text markers and cursor RAM.
type PartyMenuDecoder interface {
	DecodePartyMenu(MemoryReader) PartyMenuState
}

// PartyMenuProfile is a game profile that exposes semantic party-menu state.
type PartyMenuProfile interface {
	GameProfile
	PartyMenuDecoder
}
