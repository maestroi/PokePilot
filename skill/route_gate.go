package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// AnswerKnownRouteGate answers a choice only when the active travel/interact
// skill owns that transition. It is intentionally not objective-boundary
// recovery: lifecycle cleanup must never make gameplay choices on behalf of a
// previous objective.
//
// Museum 1F's "Would you like to come in?" is one measured route gate. YES
// buys the ¥50 ticket. PewterCityDefaultScript clears that ticket on every
// visit, so the same journey can meet the box more than once; this helper is
// therefore idempotent per open prompt, not per Travel call.
//
// It reports (false, nil) when no such prompt is up, so Travel/TalkAt can try
// it before treating a choice as one they do not own.
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
