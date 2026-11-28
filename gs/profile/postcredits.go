package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

// PostCreditsMenu identifies only the two retail Gold/Silver surfaces that
// require a deliberate CONTINUE choice after the Hall of Fame. Menu RAM is
// reused by ordinary gameplay; neither row count nor cursor alone is proof.
type PostCreditsMenu struct {
	ContinueVisible bool
	SaveInfoVisible bool
	Cursor          int // 1-based main-menu cursor when ContinueVisible
}

// DecodePostCreditsMenu uses the *rendered* tilemap as proof of the main menu
// or confirmation panel, never stale wMenuCursorY or an inferred map name.
// The caller must separately prove the Hall of Fame event before using this
// decoder to send any input.
func (*Profile) DecodePostCreditsMenu(reader game.MemoryReader) PostCreditsMenu {
	if reader == nil || reader.Peek8(sym.BattleMode) != 0 ||
		reader.Peek8(sym.SaveFileExists) == 0 {
		return PostCreditsMenu{}
	}
	text := strings.ToUpper(gsScreenText(reader))
	if strings.Contains(text, "CONTINUE") && strings.Contains(text, "NEW GAME") {
		rows := reader.Peek8(sym.TwoDMenuNumRows)
		cursor := reader.Peek8(sym.MenuCursorY)
		if rows >= 3 && rows <= 4 && cursor >= 1 && cursor <= rows {
			return PostCreditsMenu{ContinueVisible: true, Cursor: int(cursor)}
		}
	}
	// The saved-game info confirmation is drawn only *after* selecting
	// CONTINUE; this panel contains BADGES and TIME but no NEW GAME entry.
	if strings.Contains(text, "BADGES") && strings.Contains(text, "TIME") &&
		!strings.Contains(text, "NEW GAME") {
		return PostCreditsMenu{SaveInfoVisible: true}
	}
	return PostCreditsMenu{}
}
