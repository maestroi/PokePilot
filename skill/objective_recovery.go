package skill

import (
	"fmt"

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

// AnswerKnownRouteGate answers the one two-option prompt that is a measured
// route gate rather than a gameplay question: Museum 1F's "Would you like to
// come in?". YES buys the ¥50 ticket. PewterCityDefaultScript clears that
// ticket on every visit, so the same journey can meet the box more than once;
// this helper is therefore idempotent per open prompt, not per Travel call.
//
// It reports (false, nil) when no such prompt is up, so callers can try it
// before treating a leftover choice as unrecoverable.
func AnswerKnownRouteGate(m *emu.Emu) (bool, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.DecodeTwoOptionMenu(&mem) == nil {
		return false, nil
	}
	text := ""
	if d := state.DecodeDialogue(&mem); d != nil {
		text = d.Text
	}
	index, ok := talkApproachChoiceIndex(m.Peek8(sym.CurMap), text)
	if !ok {
		return false, nil
	}
	if err := selectTwoOption(m, index); err != nil {
		return false, fmt.Errorf("answer route-gate choice: %w", err)
	}
	rec := RecoverDialogue(m, dialogueRecoveryBudget)
	if rec.Stop != DialogueRecovered {
		return false, fmt.Errorf("route-gate thank-you did not clear (%d): %q", rec.Stop, rec.Text)
	}
	return true, nil
}
