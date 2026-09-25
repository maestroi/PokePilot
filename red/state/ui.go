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
// The test is the CURSOR GLYPH on the tilemap, at the location the ROM itself
// recorded for it: wMenuCursorLocation is written by PlaceMenuCursor on every
// iteration of HandleMenuInput, and nothing draws a filled '▶' anywhere else.
// It is deliberately NOT wTextBoxID: that is not a liveness bit and goes stale
// (every catch leaves 0x14 behind — see
// TestRecoverDialogueIgnoresStaleTextBoxID), so reading it here would call an
// ordinary NPC line a menu and refuse to page it.
//
// Read wMenuCursorLocation rather than TopMenuItemX/TopMenuItemY. Those are
// only the FIRST item's coordinates; PlaceMenuCursor walks down one row per
// item, and two rows per item when BIT_DOUBLE_SPACED_MENU is set. The START
// menu is double-spaced with TopMenuItemY=2, so with ITEM selected
// (CurrentMenuItem=2) its filled cursor is drawn at (11,6) while (11,2) still
// holds the box border. Reading the top-item coordinates therefore reported
// "no menu" for a perfectly live START menu, which left
// UseEvolutionItem unable to see the leftover surface it had to dismiss and
// deadlocked every stone use (run-jxh8lk19wv6on, triage:067dd95f2f04909a).
func MenuUp(m *Mem) bool {
	if m.U8(sym.FontLoaded) == 0 {
		return false
	}
	return menuCursorDrawn(m)
}

// pendingFlyOrDungeonWarp is wStatusFlags6 BIT_FLY_WARP (3) | BIT_DUNGEON_WARP (4).
const pendingFlyOrDungeonWarp = 1<<3 | 1<<4

// Controllable reports whether the game is accepting free overworld input.
// The map-dimension check is essential: wCurMap, wXCoord and wYCoord are
// written during new-game initialisation while the intro is still running,
// so they are NOT evidence that the overworld has been reached. A loaded
// map always has non-zero dimensions.
//
// A Fly or dungeon (hole) warp writes wCurMap before the new map's sprites
// load, and EnterMap clears BIT_FLY_WARP/BIT_DUNGEON_WARP only after they do
// (home/overworld.asm). Until then the old map's sprite table is still in RAM
// (Victory Road 3F->2F hole, run-1biaubd9xooqm), so the warp is not landed.
//
// The Cable Club warp is the same shape: the link menu writes wCurMap to the
// Trade Center/Colosseum, then SpecialEnterMap zeroes the joypad and returns
// with the old map still loaded and wJoyIgnore clear. EnterMap runs only after
// the overworld clears wEnteringCableClub, so input before then is dropped.
func Controllable(m *Mem) bool {
	return m.U8(sym.StatusFlags6)&pendingFlyOrDungeonWarp == 0 &&
		m.U8(sym.EnteringCableClub) == 0 &&
		m.U8(sym.CurMapWidth) != 0 &&
		m.U8(sym.CurMapHeight) != 0 &&
		m.U8(sym.FontLoaded) == 0 &&
		m.U8(sym.JoyIgnore) == 0 &&
		m.U8(sym.WalkCounter) == 0 &&
		m.U8(sym.IsInBattle) == 0 &&
		m.U8(sym.StatusFlags4)&battleOverOrBlackout == 0
}

// battleOverOrBlackout is wStatusFlags4's BIT_BATTLE_OVER_OR_BLACKOUT. The
// overworld sets it when a battle ends (home/overworld.asm .battleOccurred)
// and clears it on the next EnterMap or inside HandleBlackOut, so while it is
// set a battle-end or whiteout transition is still in flight; a lost battle
// also leaves wIsInBattle=$ff through that window. MEASURED on
// run-22ahrk9pflcilu3jxq9xt37x6: after losing to Lance the joypad, font and
// walk counter read idle for 81 frames while HandleBlackOut was still fading
// to the Indigo Plateau respawn. Reading that as control let the battle
// settle hand back a fainted party "in" Lance's room, so every later
// objective planned from a room the player was about to leave.
const battleOverOrBlackout = 1 << 5
