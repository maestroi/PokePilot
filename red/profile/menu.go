package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
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

// DecodeStartMenu owns Red's tile markers, story-dependent menu shape and
// battle byte. The generic opener only sees semantic visibility/readiness.
func (*Profile) DecodeStartMenu(reader game.MemoryReader) game.StartMenuState {
	if reader == nil {
		return game.StartMenuState{}
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])

	menu := state.DecodeMenu(&mem)
	text := state.ScreenText(&mem)
	visible := strings.Contains(text, "SAVE") && strings.Contains(text, "EXIT")

	// Red/Blue's START menu gains POKEDEX after Oak gives it. Keep this
	// version-specific menu shape on the profile side of the boundary.
	wantMax := 6
	if state.HasEvent(&mem, state.EventGotPokedex) {
		wantMax = 7
	}

	return game.StartMenuState{
		Visible:  visible,
		Ready:    visible && menu.Max == wantMax,
		InBattle: mem.U8(sym.IsInBattle) != 0,
		Cursor:   game.MenuCursorState{Current: menu.Current, Max: menu.Max},
	}
}
