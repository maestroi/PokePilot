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

// MenuDecoder is the optional semantic capability used by generic menu
// navigation. Concrete game profiles own RAM addresses, cursor glyphs and
// prompt-liveness rules.
type MenuDecoder interface {
	DecodeMenuCursor(MemoryReader) MenuCursorState
	DecodeTwoOption(MemoryReader) (TwoOptionState, bool)
}

// MenuProfile is a game profile that exposes semantic menu decoding.
type MenuProfile interface {
	GameProfile
	MenuDecoder
}
