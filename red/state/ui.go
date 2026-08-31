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

// MenuUp reports that what is on screen is a MENU rather than ordinary
// dialogue. The distinction decides whether pressing A is safe: on a text
// box A turns the page, but on a menu A SELECTS, and a layer that pages a
// stuck box closed by tapping A will happily walk a shop menu into a
// purchase nobody asked for. MEASURED 2026-08-31: recovery inside the
// Viridian Mart selected POKe BALL and left the run parked on "That will be
// Y200. OK?", which nothing would answer, so every later objective failed.
//
// The test is the CURSOR GLYPH on the tilemap, at the coordinates the menu
// says it drew itself — the same live evidence DecodeTwoOptionMenu uses,
// generalised past its wMaxMenuItem == 1 check so it also catches the
// buy/sell/quit menu and the priced item list. It is deliberately NOT
// wTextBoxID: that is not a liveness bit and goes stale (every catch leaves
// 0x14 behind — see TestRecoverDialogueIgnoresStaleTextBoxID), so reading
// it here would call an ordinary NPC line a menu and refuse to page it.
// The tilemap is the screen: either a cursor is drawn or it is not.
func MenuUp(m *Mem) bool {
	if m.U8(sym.FontLoaded) == 0 {
		return false
	}
	y, x := int(m.U8(sym.TopMenuItemY)), int(m.U8(sym.TopMenuItemX))
	if y >= 18 || x >= 20 {
		return false
	}
	return m.Slice(sym.TileMap, sym.TileMapLen)[y*20+x] == menuCursorTile
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
