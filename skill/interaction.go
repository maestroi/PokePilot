package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// ErrUnexpectedInteraction reports that a semantic UI operation was asked to
// act on a different live surface. The operation never presses a button in
// that case, so unknown choices fail closed instead of being guessed through.
var ErrUnexpectedInteraction = errors.New("skill: unexpected interaction state")

const interactionTransitionFrames = 120

// WaitForInteraction waits without input until want is positively decoded.
// It is suitable for autonomous transitions that draw a menu after a prior
// semantic action. It never pages dialogue or selects a menu while waiting.
func WaitForInteraction(m *emu.Emu, want state.InteractionKind, frameBudget int) (state.InteractionState, error) {
	if frameBudget < 0 {
		return state.InteractionState{}, fmt.Errorf("skill: WaitForInteraction: negative frame budget %d", frameBudget)
	}
	var mem state.Mem
	for i := 0; i <= frameBudget; i++ {
		state.Snapshot(m, &mem)
		got := state.DecodeInteraction(&mem)
		if got.Kind == want {
			return got, nil
		}
		if i != frameBudget {
			m.StepFrame()
		}
	}
	state.Snapshot(m, &mem)
	got := state.DecodeInteraction(&mem)
	return got, fmt.Errorf("skill: WaitForInteraction: %w: wanted %q, got %q text=%q after %d frames",
		ErrUnexpectedInteraction, want, got.Kind, got.Text, frameBudget)
}

// AnswerTwoOption selects one of the two entries on a live two-option prompt.
// The underlying controller verifies the cursor position before pressing A and
// verifies that the prompt was consumed afterward.
func AnswerTwoOption(m *emu.Emu, index int) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if got := state.DecodeInteraction(&mem); got.Kind != state.InteractionTwoOption {
		return fmt.Errorf("skill: AnswerTwoOption: %w: got %q text=%q", ErrUnexpectedInteraction, got.Kind, got.Text)
	}
	return selectTwoOption(m, index)
}

// AnswerYesNo explicitly answers a semantic YES/NO prompt. It selects by the
// decoded option labels, not by a fixed index: Gen I has both YES/NO and
// NO/YES layouts. Non-yes/no two-option menus (HEAL/CANCEL, directions,
// TRADE/CANCEL) are rejected without input.
func AnswerYesNo(m *emu.Emu, yes bool) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	got := state.DecodeInteraction(&mem)
	if got.Kind != state.InteractionTwoOption {
		return fmt.Errorf("skill: AnswerYesNo: %w: got %q text=%q", ErrUnexpectedInteraction, got.Kind, got.Text)
	}
	want := "NO"
	if yes {
		want = "YES"
	}
	if !((got.Options[0] == "YES" && got.Options[1] == "NO") ||
		(got.Options[0] == "NO" && got.Options[1] == "YES")) {
		return fmt.Errorf("skill: AnswerYesNo: %w: two-option menu is %q/%q, not YES/NO",
			ErrUnexpectedInteraction, got.Options[0], got.Options[1])
	}
	index := 0
	if got.Options[1] == want {
		index = 1
	}
	return selectTwoOption(m, index)
}

// SelectInteractionIndex selects a semantic menu entry by zero-based index.
// Scrolling list menus use absolute list positions (scroll offset + cursor).
// Party menus deliberately use SelectPartySlot rather than SelectMenuItem:
// PartyMenuInit stores wMaxMenuItem as the last valid index (count-1), while
// ordinary cursor menus store a count. Sending a party menu through the
// generic helper made the final party member unreachable.
func SelectInteractionIndex(m *emu.Emu, index int) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	got := state.DecodeInteraction(&mem)
	switch got.Kind {
	case state.InteractionTwoOption:
		return AnswerTwoOption(m, index)
	case state.InteractionListMenu, state.InteractionElevatorMenu, state.InteractionItemMenu, state.InteractionPCPokemonList:
		return selectListEntry(m, index)
	case state.InteractionPartyMenu:
		return SelectPartySlot(m, index)
	case state.InteractionMenu, state.InteractionPCMenu:
		return SelectMenuItem(m, index)
	default:
		return fmt.Errorf("skill: SelectInteractionIndex: %w: got %q text=%q", ErrUnexpectedInteraction, got.Kind, got.Text)
	}
}

// CancelInteraction presses B only on a menu-shaped interaction and verifies
// that the live decoded surface changed. It deliberately refuses ordinary
// dialogue and two-option prompts because B can carry gameplay meaning there.
func CancelInteraction(m *emu.Emu) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	before := state.DecodeInteraction(&mem)
	switch before.Kind {
	case state.InteractionMenu, state.InteractionListMenu, state.InteractionElevatorMenu,
		state.InteractionItemMenu, state.InteractionPartyMenu, state.InteractionPCMenu,
		state.InteractionPCPokemonList:
		// Safe back/cancel surfaces.
	default:
		return fmt.Errorf("skill: CancelInteraction: %w: cannot cancel %q text=%q", ErrUnexpectedInteraction, before.Kind, before.Text)
	}

	m.Tap(emu.B, 3, 7)
	if _, err := m.StepUntil(interactionTransitionFrames, func(e *emu.Emu) bool {
		var nextMem state.Mem
		state.Snapshot(e, &nextMem)
		return state.DecodeInteraction(&nextMem) != before
	}); err != nil {
		state.Snapshot(m, &mem)
		after := state.DecodeInteraction(&mem)
		return fmt.Errorf("skill: CancelInteraction: interaction did not change after B: before=%+v after=%+v: %w", before, after, err)
	}
	return nil
}
