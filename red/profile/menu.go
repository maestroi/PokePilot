package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
)

// DecodeMenuCursor keeps Red's menu RAM layout behind the profile boundary.
func (*Profile) DecodeMenuCursor(reader game.MemoryReader) game.MenuCursorState {
	if reader == nil {
		return game.MenuCursorState{}
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])
	menu := state.DecodeMenu(&mem)
	return game.MenuCursorState{Current: menu.Current, Max: menu.Max}
}

// DecodeTwoOption keeps Red's prompt-liveness/cursor-glyph rules behind the
// profile boundary instead of exposing them to the generic skill driver.
func (*Profile) DecodeTwoOption(reader game.MemoryReader) (game.TwoOptionState, bool) {
	if reader == nil {
		return game.TwoOptionState{}, false
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])
	prompt := state.DecodeTwoOptionMenu(&mem)
	if prompt == nil {
		return game.TwoOptionState{}, false
	}
	return game.TwoOptionState{Current: prompt.Index}, true
}
