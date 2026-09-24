package game

// MenuCursorState is the semantic state of a simple cursor menu. Max is the
// highest valid index, matching the inclusive cursor contract used by generic
// menu navigation.
type MenuCursorState struct {
	Current int
	Max     int
}

// TwoOptionState describes a live two-option prompt waiting for input.
type TwoOptionState struct {
	Current int
}

// StartMenuState is the semantic state needed by the reusable START-menu
// opener. Concrete profiles decide how a START menu is identified, when it is
// ready for input, and whether the current gameplay state forbids opening it.
type StartMenuState struct {
	Visible  bool
	Ready    bool
	InBattle bool
	Cursor   MenuCursorState
}

// StartMenuEntry is a semantic entry in an overworld START menu. Concrete
// profiles map these identities onto their current game/version-specific
// ordering.
type StartMenuEntry string

const (
	StartMenuPokemon StartMenuEntry = "pokemon"
	StartMenuItems   StartMenuEntry = "items"
)

// MenuDecoder is the optional semantic capability used by generic menu
// navigation. Concrete game profiles own RAM addresses, cursor glyphs,
// prompt-liveness rules, and START-menu identification.
type MenuDecoder interface {
	DecodeMenuCursor(MemoryReader) MenuCursorState
	DecodeTwoOption(MemoryReader) (TwoOptionState, bool)
	DecodeStartMenu(MemoryReader) StartMenuState
	StartMenuEntryIndex(MemoryReader, StartMenuEntry) (int, bool)
}

// MenuProfile is a game profile that exposes semantic menu decoding.
type MenuProfile interface {
	GameProfile
	MenuDecoder
}
