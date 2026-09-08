package skill

import (
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// DismissableObjectiveMenu reports a menu that an objective lifecycle may
// safely back out of with B. Most ordinary menus are trivially dismissable.
// The special case is a two-option-shaped screen: the generic decoder also
// matches a short scrolling bag list because both have wMaxMenuItem==1 and a
// live cursor. A live item list (wListMenuID==3) is still a bag/menu, not a
// yes/no question, so it remains recoverable.
//
// This helper deliberately knows nothing about story or route choices. A
// gameplay question belongs to the skill that encountered it; lifecycle
// cleanup may only undo UI state whose meaning is unambiguously "back".
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
// Objective execution owns this cleanup as part of finishing its transaction:
// one failed bag/TM/party interaction must not poison the next objective with
// a non-controllable start, but this layer never makes a gameplay decision to
// achieve that invariant.
func CloseOpenMenuToOverworld(m *emu.Emu) error {
	return closeToOverworld(m)
}
