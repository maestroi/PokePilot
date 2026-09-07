package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

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

// executeObjective is Run's dispatch boundary. Ordinary objectives keep the
// existing Execute path unchanged. Planner-offered TM/HM objectives reuse the
// existing KindUseItem shape but are intercepted here because their semantics
// are teach-a-move, not field medicine.
func executeObjective(m *emu.Emu, romData []byte, o Objective) (retErr error) {
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
