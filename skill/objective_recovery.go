package skill

import "github.com/maestroi/pokepilot/emu"

// CloseOpenMenuToOverworld backs out of a menu that a finished objective
// accidentally left open. The caller must first establish that the screen is
// a menu, not a battle, dialogue box, or unanswered choice: B is a safe
// "back" operation for the former and could answer/alter the latter.
//
// Objective execution normally owns its whole menu lifecycle, but this is the
// between-objective safety net: one failed bag/TM/party interaction must not
// poison every later overworld objective with a non-controllable start.
func CloseOpenMenuToOverworld(m *emu.Emu) error {
	return closeToOverworld(m)
}
