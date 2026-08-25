package state

import "github.com/maestroi/pokepilot/red/sym"

// DialogueState is the decoded text-box context.
type DialogueState struct {
	// TextBoxID is wTextBoxID. Measured on the real ROM as useless for
	// detecting an open box (it read 0x01 before, during and after
	// dialogue); kept only because a few scripts branch on it.
	TextBoxID uint8

	// Text is what the box actually says, decoded from the tilemap.
	Text string
}

// DecodeDialogue returns nil when no text box is up.
func DecodeDialogue(m *Mem) *DialogueState {
	if m.U8(sym.FontLoaded) == 0 {
		return nil
	}
	return &DialogueState{
		TextBoxID: m.U8(sym.TextBoxID),
		Text:      ScreenText(m),
	}
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
