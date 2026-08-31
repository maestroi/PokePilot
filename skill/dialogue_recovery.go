package skill

import (
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// DialogueRecoveryStop names why RecoverDialogue stopped. Every value is a
// positive outcome — a fact about the game at the stop — not an error path.
type DialogueRecoveryStop uint8

const (
	// DialogueRecovered: the box is closed and the player is controllable;
	// movement may resume.
	DialogueRecovered DialogueRecoveryStop = iota
	// DialogueChoiceRequired: a two-option prompt is up and unanswered.
	// The loop stopped before pressing A on it; answering is not recovery,
	// so the caller decides.
	DialogueChoiceRequired
	// DialogueBudgetExhausted: the box was still up when the frame budget
	// ran out.
	DialogueBudgetExhausted
	// DialogueUnexpectedMode: the screen is not an ordinary text box —
	// measured so far, a battle the box led into.
	DialogueUnexpectedMode
	// DialogueMenuOpen: a MENU is up, not a text box. The loop stopped
	// before pressing A, because A on a menu is a selection: recovery
	// closes boxes, it does not operate menus. Like a choice, this is the
	// caller's to resolve.
	DialogueMenuOpen
)

// DialogueRecoveryResult is what RecoverDialogue found when it stopped.
type DialogueRecoveryResult struct {
	Stop DialogueRecoveryStop
	// Text is the box still on screen at the stop; "" when it cleared.
	Text string
	// Presses is the number of A presses the loop sent. It is zero whenever
	// the loop stopped before its first press — in particular on a choice
	// that was up on entry.
	Presses int
	Final   state.GameState
	Sprites []state.SpriteState
}

// dialogueRecoveryBudget bounds one recovery in frames. A sign is a page or
// two; the rival's forced cutscene is one box into a battle. A budget this
// large can only be exhausted by a box that never closes, so exhausting it
// is a real failure, not slowness.
const dialogueRecoveryBudget = 10000

// RecoverDialogue pages an open text box closed without ever answering a
// choice. It is valid only after ErrDialogueInterrupted: a text box is up
// and movement has stopped. It never sends a direction. Before every A
// press it snapshots and checks the choice decoder, and a detected choice
// returns immediately with zero input sent.
func RecoverDialogue(m *emu.Emu, budget int) DialogueRecoveryResult {
	return recoverDialogue(m, budget)
}

// recoverDialogue is the loop over the frameClock seam so the tests can
// drive it without an emulator; RecoverDialogue is its *emu.Emu front.
//
// Movement never advances dialogue. After dialogue has interrupted
// movement, the recovery layer may press A only while ordinary text is
// active. It never answers a choice.
func recoverDialogue(m frameClock, budget int) DialogueRecoveryResult {
	// done holds when the screen has left the box: a battle took it (the
	// box led into one), or the box is closed and the player is
	// controllable. Checking the battle first is what lets a cutscene box
	// that ends in a battle stop the loop the frame the battle starts.
	done := func(mm *state.Mem) bool {
		return state.DecodeBattle(mm) != nil ||
			(state.DecodeDialogue(mm) == nil && state.Controllable(mm))
	}
	// stopBeforeA is checked after the snapshot and before every A press:
	// a two-option prompt is a question, and this layer does not answer
	// questions — and any other menu is worse, because A there SELECTS.
	// Paging a box closed and operating a menu look identical from here
	// (both want A) and are not remotely the same act.
	stopBeforeA := func(mm *state.Mem) bool {
		return state.DecodeTwoOptionMenu(mm) != nil || state.MenuUp(mm)
	}

	final, presses := advanceCore(m, budget, done, stopBeforeA)

	res := DialogueRecoveryResult{
		Presses: presses,
		Final:   state.Decode(&final),
		Sprites: state.DecodeSprites(&final),
	}
	switch {
	case state.DecodeBattle(&final) != nil:
		res.Stop = DialogueUnexpectedMode
	case state.DecodeTwoOptionMenu(&final) != nil:
		res.Stop = DialogueChoiceRequired
	case state.MenuUp(&final):
		res.Stop = DialogueMenuOpen
	case state.DecodeDialogue(&final) == nil && state.Controllable(&final):
		res.Stop = DialogueRecovered
	default:
		res.Stop = DialogueBudgetExhausted
	}
	if d := state.DecodeDialogue(&final); d != nil {
		res.Text = d.Text
	}
	return res
}
