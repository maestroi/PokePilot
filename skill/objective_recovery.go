package skill

import (
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// DismissableObjectiveMenu reports a menu that an objective-boundary cleanup
// may safely back out of with B. Most ordinary menus are trivially
// dismissable. The special case is a two-option-shaped screen: the generic
// decoder also matches a short scrolling bag list because both have
// wMaxMenuItem==1 and a live cursor. A live item list (wListMenuID==3) is still
// a bag/menu, not a yes/no question, so it must remain recoverable — that is
// exactly the leaked TM state that motivated this boundary.
func DismissableObjectiveMenu(mem *state.Mem) bool {
	if !state.MenuUp(mem) {
		return false
	}
	if state.DecodeTwoOptionMenu(mem) == nil {
		return true
	}
	return mem.U8(sym.ListMenuID) == itemListMenuID
}

// CloseOpenMenuToOverworld backs out of a menu that a finished objective
// accidentally left open. The caller must first establish that the screen is
// dismissable (see DismissableObjectiveMenu), not a battle or an unanswered
// gameplay choice. B is a safe "back" operation for the former and could
// answer/alter the latter.
//
// Objective execution normally owns its whole menu lifecycle, but this is the
// between-objective safety net: one failed bag/TM/party interaction must not
// poison every later overworld objective with a non-controllable start.
func CloseOpenMenuToOverworld(m *emu.Emu) error {
	return closeToOverworld(m)
}
