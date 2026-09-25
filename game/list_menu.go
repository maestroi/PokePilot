package game

// ListMenuKind identifies the broad semantic family of a scrolling list.
type ListMenuKind string

const (
	ListMenuGeneric   ListMenuKind = "generic"
	ListMenuItems     ListMenuKind = "items"
	ListMenuElevator  ListMenuKind = "elevator"
	ListMenuPCPokemon ListMenuKind = "pc_pokemon"
)

// ListMenuState is the minimum live state generic scrolling-list navigation
// needs. Position is the absolute zero-based entry under the cursor, including
// any scroll offset owned by the concrete game.
type ListMenuState struct {
	Visible  bool
	Kind     ListMenuKind
	Position int
}

// ListMenuDecoder hides game-specific list ids, cursor bytes and scroll
// offsets from reusable list navigation.
type ListMenuDecoder interface {
	DecodeListMenu(MemoryReader) ListMenuState
}

// ListMenuProfile is a game profile that exposes semantic scrolling-list state.
type ListMenuProfile interface {
	GameProfile
	ListMenuDecoder
}
