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
	// LastText is the last non-empty page this recovery displayed, even when
	// Stop is DialogueRecovered and the box has since closed. A closed box
	// can still be identified this way — a known-impassable notice
	// (Saffron's gate guards) always ends on the same final page, right
	// before the loop pages it away for good.
	LastText string
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
	// lastText tracks the most recent non-empty page this recovery displayed,
	// captured as a side effect of done's look at each snapshot. The
	// typewriter effect means a box's text starts empty and fills in over
	// several frames, so only the last non-empty read reflects what the
	// player actually saw on the page the loop is about to close.
	lastText := ""
	done := func(mm *state.Mem) bool {
		if d := state.DecodeDialogue(mm); d != nil && d.Text != "" {
			lastText = d.Text
		}
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
		Presses:  presses,
		Final:    state.Decode(&final),
		Sprites:  state.DecodeSprites(&final),
		LastText: lastText,
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

// menuDismissBudget bounds one menu dismissal in key presses. Each iteration
// is one press (B on a cursor menu, A on a plain box) plus a settle. The
// deepest dismissal this layer meets is the shop's item list — B back to the
// "Anything else?" box, A to the action menu, B to the overworld: three
// presses. The budget leaves headroom for a menu that needs a moment to
// redraw between presses without spinning long on a menu that will not close.
const menuDismissBudget = 12

// dismissMenu closes a cursor menu that interrupted a walk by backing out of
// it, and reports true only once the overworld is positively controllable.
//
// It is the shared fix for every list menu a walk can stumble into — a shop
// item list, the PC menu, an elevator, the START menu — none of which is a
// question this layer is allowed to answer. The safety comes from pressing the
// key that matches the surface actually on screen:
//
//   - a cursor menu (MenuUp) is pressed with B, the Gen-I cancel key, which
//     never selects an item and never answers a prompt — it only backs out;
//   - a plain text box with no cursor (the shop's "Anything else?" between the
//     item list and the action menu) is paged with A, the same primitive
//     RecoverDialogue uses;
//   - a two-option prompt is never touched: the instant one appears, B has led
//     into a question the caller owns, so the dismissal stops and fails closed.
//
// This is deliberately NOT the shop's exitToOverworld, which decides B vs A
// from DecodeShop's phase: that misclassifies the PC menu and the elevator as
// a greeting and would press A on them, selecting an entry. Here the decision
// is the cursor glyph itself, which is portable across every Gen-I menu.
//
// It fails closed on every path it cannot positively confirm: a screen that is
// neither a menu, a box, nor a controllable overworld returns false, and so
// does a menu that is still up when the budget runs out.
func dismissMenu(m *emu.Emu) (bool, error) {
	if m == nil {
		return false, nil
	}
	for i := 0; i < menuDismissBudget; i++ {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if state.DecodeTwoOptionMenu(&mem) != nil {
			// B led into a question. It is the caller's to answer, not this
			// layer's to dismiss; stop before touching it.
			return false, nil
		}
		switch {
		case state.MenuUp(&mem):
			m.Tap(emu.B, 3, 7)
		case state.DecodeDialogue(&mem) != nil:
			m.Tap(emu.A, 3, 7)
		case state.Controllable(&mem):
			return true, nil
		default:
			// Neither a menu, a box, nor a controllable overworld. Do not
			// guess which key would clear it.
			return false, nil
		}
		m.StepFrames(talkSettle)
	}
	return false, nil
}
