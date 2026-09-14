package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// declineUnexpectedGenericTalkChoice is the last-line recovery for a generic
// KindTalk that reached an unclassified YES/NO prompt. Known service/reward
// actors are filtered before execution; this covers custom text_asm actors and
// stale/direct objectives without teaching generic Talk to accept gameplay
// choices.
//
// NO is the only answer generic conversation may own: it declines the offered
// action without spending money, trading, healing, taking a gift, or otherwise
// opting into the prompt. Non-YES/NO two-option menus still fail closed.
func declineUnexpectedGenericTalkChoice(m *emu.Emu) (bool, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	interaction := state.DecodeInteraction(&mem)
	if !genericTalkDeclinableYesNo(interaction) {
		return false, nil
	}

	if err := skill.AnswerYesNo(m, false); err != nil {
		return true, fmt.Errorf("answer NO: %w", err)
	}
	// A declined prompt can chain into ordinary follow-up dialogue ("Oh,
	// okay", etc.). Reuse the objective boundary normalizer to page that text
	// and verify we return to a clean overworld state. It deliberately refuses
	// any second gameplay choice, so recovery remains fail-closed.
	if err := normalizeObjectiveBoundary(m); err != nil {
		return true, fmt.Errorf("normalize after NO: %w", err)
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
