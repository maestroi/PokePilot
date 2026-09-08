package agent

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// objectiveFrameBudget is the emergency guard for one synchronous objective.
// Most low-level waits are hundreds or thousands of frames and a journey is
// already bounded to 20 engagements; half a million frames leaves generous
// room for legitimate long travel/training while ensuring a broken inner loop
// returns control to Run instead of leaving a farm worker on one round forever.
const objectiveFrameBudget uint64 = 500_000

// offerWithTMHM extends the ordinary factual objective menu with owned
// machines that the ROM says a party member can learn and the move-set policy
// says are worth spending. Offer itself intentionally has no ROM or full party
// move data, so machine recommendations live at Run's richer boundary rather
// than smuggling compatibility policy into Observation.
func offerWithTMHM(m *emu.Emu, romData []byte, obs Observation, known *Knowledge) []Objective {
	out := Offer(obs, known)
	var mem state.Mem
	state.Snapshot(m, &mem)
	return appendTMHMObjectives(romData, state.DecodeParty(&mem), state.DecodeInventory(&mem), out)
}

func appendTMHMObjectives(romData []byte, party state.PartyState, inventory state.InventoryState, out []Objective) []Objective {
	for _, item := range inventory.Items {
		if item.Quantity == 0 {
			continue
		}
		machine, err := rom.LookupTMHM(romData, item.ID)
		if err != nil {
			continue // ordinary bag item, unknown machine, or malformed ROM entry
		}
		decision, err := skill.DecideTMHM(romData, party, item.ID, false)
		if err != nil || decision.Existing {
			continue // incompatible, not an upgrade, or already learned
		}
		out = append(out, Objective{
			Kind: KindUseItem,
			Item: item.ID,
			Slot: decision.PartySlot,
			Note: tmhmDecisionNote(machine, decision),
		})
	}
	return out
}

func tmhmDecisionNote(machine rom.Machine, decision skill.TMHMDecision) string {
	name := fmt.Sprintf("TM%02d", machine.Number)
	semantics := "consumable/finite"
	if machine.HM {
		name = fmt.Sprintf("HM%02d", machine.Number-rom.NumTMs)
		semantics = "reusable item; learned HM cannot be forgotten"
	}
	placement := "empty move slot"
	if decision.ReplaceSlot >= 0 {
		placement = fmt.Sprintf("replace move slot %d", decision.ReplaceSlot)
	}
	return fmt.Sprintf("(%s teaches move %d; %s; party slot %d score %d->%d; %s)",
		name, machine.Move, semantics, decision.PartySlot, decision.BeforeScore, decision.AfterScore, placement)
}

// normalizeObjectiveBoundary establishes the control invariant shared by the
// start and finish of every objective transaction. It may perform only
// semantically reversible cleanup: page ordinary text and back out of a menu
// whose meaning is unambiguously "back". It never answers a gameplay choice,
// starts/finishes a battle, or guesses through an unknown non-controllable
// state. Those belong to the skill that encountered them.
//
// Keeping this rule symmetric is important. Cleanup after Execute attributes a
// leaked menu/dialogue to the objective that produced it, before the planner is
// allowed to choose something else. The start check then becomes an invariant
// guard (and a compatibility path for old/resumed checkpoints), not the normal
// owner of previous-objective recovery.
func normalizeObjectiveBoundary(m *emu.Emu) error {
	const maxPasses = 4
	for pass := 0; pass < maxPasses; pass++ {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if state.Controllable(&mem) && state.DecodeDialogue(&mem) == nil && !state.MenuUp(&mem) {
			return nil
		}
		if state.DecodeBattle(&mem) != nil {
			return fmt.Errorf("battle still in progress")
		}
		// Check dismissable menus before the generic two-option decoder: a
		// two-entry bag list has the same cursor/max shape as YES/NO but is
		// still just a menu that B can safely unwind.
		if skill.DismissableObjectiveMenu(&mem) {
			if err := skill.CloseOpenMenuToOverworld(m); err != nil {
				return fmt.Errorf("close leftover menu: %w", err)
			}
			continue
		}
		if state.DecodeTwoOptionMenu(&mem) != nil {
			return fmt.Errorf("unanswered choice remains open")
		}
		if state.MenuUp(&mem) {
			return fmt.Errorf("non-dismissable menu remains open")
		}
		if state.DecodeDialogue(&mem) != nil {
			res := skill.RecoverDialogue(m, roundRecoveryBudget)
			if res.Stop != skill.DialogueRecovered {
				return fmt.Errorf("leftover dialogue did not recover: %s", recoveryStopName(res.Stop))
			}
			continue
		}
		return fmt.Errorf("player is not controllable and no recoverable menu or dialogue is open")
	}
	return fmt.Errorf("objective boundary cleanup did not converge after %d passes", maxPasses)
}

func prepareObjectiveBoundary(m *emu.Emu) error {
	return normalizeObjectiveBoundary(m)
}

func settleObjectiveBoundary(m *emu.Emu) error {
	return normalizeObjectiveBoundary(m)
}

// objectiveBoundaryError combines execution and finish-boundary failures
// without losing the original typed error identity. If execution itself was
// successful, a dirty finish is a postcondition failure owned by the objective
// that just ran — never by whatever the planner might pick next.
func objectiveBoundaryError(o Objective, primary, boundary error) error {
	if boundary == nil {
		return primary
	}
	if primary != nil {
		return fmt.Errorf("%w; objective left invalid boundary: %v", primary, boundary)
	}
	return fmt.Errorf("agent: %s: objective postcondition: %w", o, boundary)
}

// executeObjective is Run's synchronous objective transaction boundary. It
// normalizes the start, executes one planner-selected action under an absolute
// frame deadline, then normalizes/verifies the finish before returning control
// to Run. A menu/dialogue leak is therefore charged to the objective that
// produced it instead of poisoning an unrelated next objective.
func executeObjective(m *emu.Emu, romData []byte, o Objective) error {
	deadline := m.FrameCount() + objectiveFrameBudget
	primary := m.WithFrameDeadline(deadline, func() error {
		return executeObjectiveUnbounded(m, romData, o)
	})
	if errors.Is(primary, emu.ErrFrameDeadline) {
		primary = fmt.Errorf("agent: %s: objective frame watchdog: %w", o, primary)
		if ferr := captureObjectiveFailure(m, o, primary); ferr != nil {
			fmt.Printf("  ram forensics: %v\n", ferr)
		}
	}

	boundary := settleObjectiveBoundary(m)
	retErr := objectiveBoundaryError(o, primary, boundary)
	if primary == nil && boundary != nil {
		// Execute captured nothing because it returned success; preserve the
		// actual dirty finish that violated the objective transaction.
		if ferr := captureObjectiveFailure(m, o, retErr); ferr != nil {
			fmt.Printf("  ram forensics: %v\n", ferr)
		}
	}
	return retErr
}

// executeObjectiveUnbounded contains the ordinary dispatch. It is called only
// through executeObjective so every planner-selected action shares the same
// start invariant, finish invariant, and frame watchdog.
func executeObjectiveUnbounded(m *emu.Emu, romData []byte, o Objective) (retErr error) {
	if err := prepareObjectiveBoundary(m); err != nil {
		retErr = fmt.Errorf("agent: %s: objective start invariant: %w", o, err)
		if ferr := captureObjectiveFailure(m, o, retErr); ferr != nil {
			fmt.Printf("  ram forensics: %v\n", ferr)
		}
		return retErr
	}

	if o.Kind != KindUseItem {
		return Execute(m, romData, o)
	}
	if _, err := rom.LookupTMHM(romData, o.Item); err != nil {
		return Execute(m, romData, o)
	}
	if o.Slot < 0 || o.Slot > 5 {
		return fmt.Errorf("agent: %s: party slot %d out of range 0..5", o, o.Slot)
	}

	// Preserve the same objective-error forensics guarantee as Execute. TM/HM
	// dispatch lives outside Execute only to avoid changing the stable generic
	// UseItem contract for medicine.
	defer func() {
		if retErr == nil {
			return
		}
		if err := captureObjectiveFailure(m, o, retErr); err != nil {
			fmt.Printf("  ram forensics: %v\n", err)
		}
	}()

	result, err := skill.TeachTMHMToSlot(m, o.Item, false, o.Slot)
	if err != nil {
		return fmt.Errorf("agent: %s: %w", o, err)
	}
	fmt.Printf("  taught machine move %d to party slot %d (score %d -> %d, consumed=%v)\n",
		result.Decision.Machine.Move,
		result.Decision.PartySlot,
		result.Decision.BeforeScore,
		result.Decision.AfterScore,
		result.Consumed,
	)
	return nil
}
