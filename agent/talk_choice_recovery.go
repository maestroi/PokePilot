package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// declineUnexpectedGenericTalkChoice is the last-line recovery for a generic
// KindTalk that reached an unclassified YES/NO prompt or a dismissable menu
// surface (a scrolling list, item, party, or PC menu drawn over dialogue —
// e.g. the Cerulean BadgeHouse "which BADGE should I describe?" list). Known
// service/reward actors are filtered before execution; this covers custom
// text_asm actors and stale/direct objectives without teaching generic Talk
// to accept gameplay choices.
//
// NO is the only answer generic conversation may own for a YES/NO prompt: it
// declines the offered action without spending money, trading, healing,
// taking a gift, or otherwise opting into the prompt. A dismissable menu has
// no answer to give at all, so cancelling it with B (via the objective
// boundary normalizer) IS the decline. Non-YES/NO two-option menus and any
// other menu shape still fail closed.
func declineUnexpectedGenericTalkChoice(m *emu.Emu) (bool, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	interaction := state.DecodeInteraction(&mem)

	switch {
	case genericTalkDeclinableYesNo(interaction):
		if err := skill.AnswerYesNo(m, false); err != nil {
			return true, fmt.Errorf("answer NO: %w", err)
		}
	case skill.DismissableObjectiveMenu(&mem):
		// Nothing to answer; the normalizer below cancels it with B.
	default:
		return false, nil
	}

	// A declined prompt or a cancelled menu can chain into ordinary follow-up
	// dialogue ("Oh, okay", "Come visit me any time", etc.). Reuse the
	// objective boundary normalizer to cancel any remaining menu layers and
	// page that text, verifying we return to a clean overworld state. It
	// deliberately refuses any second gameplay choice, so recovery remains
	// fail-closed.
	if err := normalizeObjectiveBoundary(m); err != nil {
		return true, fmt.Errorf("normalize after decline: %w", err)
	}
	return true, nil
}

func genericTalkDeclinableYesNo(interaction state.InteractionState) bool {
	if interaction.Kind != state.InteractionTwoOption {
		return false
	}
	return (interaction.Options[0] == "YES" && interaction.Options[1] == "NO") ||
		(interaction.Options[0] == "NO" && interaction.Options[1] == "YES")
}
