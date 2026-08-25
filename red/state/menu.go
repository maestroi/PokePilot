package state

import "github.com/maestroi/pokepilot/red/sym"

// MenuState is the decoded menu cursor.
type MenuState struct {
	Current int // wCurrentMenuItem
	Max     int // wMaxMenuItem, inclusive
}

// DecodeMenu reads the menu cursor. It carries no notion of whether a menu
// is actually open; callers gate on FontLoaded.
func DecodeMenu(m *Mem) MenuState {
	return MenuState{
		Current: int(m.U8(sym.CurrentMenuItem)),
		Max:     int(m.U8(sym.MaxMenuItem)),
	}
}
