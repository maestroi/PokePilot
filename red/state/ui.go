package state

import "github.com/maestroi/pokepilot/red/sym"

// MenuState is the decoded menu context.
type MenuState struct {
	Current uint8 // cursor index
	Max     uint8 // highest selectable index
}

// DialogueState is the decoded text-box context.
type DialogueState struct {
	TextBoxID uint8
}

// DecodeMenu returns nil when no menu is open.
func DecodeMenu(m *Mem) *MenuState {
	if m.U8(sym.FontLoaded) == 0 {
		return nil
	}
	return &MenuState{
		Current: m.U8(sym.CurrentMenuItem),
		Max:     m.U8(sym.MaxMenuItem),
	}
}

// DecodeDialogue returns nil when no text box is up.
func DecodeDialogue(m *Mem) *DialogueState {
	if m.U8(sym.FontLoaded) == 0 {
		return nil
	}
	return &DialogueState{TextBoxID: m.U8(sym.TextBoxID)}
}

// Controllable reports whether the game is accepting free overworld input.
// The map-dimension check is essential: wCurMap, wXCoord and wYCoord are
// written during new-game initialisation while the intro is still running,
// so they are NOT evidence that the overworld has been reached. A loaded
// map always has non-zero dimensions.
func Controllable(m *Mem) bool {
	return m.U8(sym.CurMapWidth) != 0 &&
		m.U8(sym.CurMapHeight) != 0 &&
		m.U8(sym.FontLoaded) == 0 &&
		m.U8(sym.JoyIgnore) == 0 &&
		m.U8(sym.WalkCounter) == 0
}
