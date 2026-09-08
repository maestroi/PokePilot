package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// offerWithTMHM extends the portable objective menu with Red-owned progression
// goals and owned machines. Neither concern is reconstructed by generic Offer.
func offerWithTMHM(m *emu.Emu, romData []byte, obs Observation, known *Knowledge) []Objective {
	out := OfferWithProgression(obs, known, newRedObjectiveAdapter(m, romData))
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
			continue
		}
		decision, err := skill.DecideTMHM(romData, party, item.ID, false)
		if err != nil || decision.Existing {
			continue
		}
		out = append(out, Objective{
			Kind: KindUseItem,
			Item: machineItemID(machine),
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

// normalizeObjectiveBoundary is Pokémon Red's implementation of the portable
// boundary contract. It may perform only semantically reversible cleanup: page
// ordinary text and back out of a menu whose meaning is unambiguously "back".
// It never answers a gameplay choice, starts/finishes a battle, or guesses
// through an unknown non-controllable state.
func normalizeObjectiveBoundary(m *emu.Emu) error {
	const maxPasses = 4
	for pass := 0; pass < maxPasses; pass++ {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if state.Controllable(&mem) && state.DecodeDialogue(&mem) == nil && !state.MenuUp(&mem) {
			return nil
		}
		if state.DecodeBattle(&mem) != nil {
			return fmt.Errorf("%w: battle still in progress", ErrObjectiveBoundaryDirty)
		}
		if skill.DismissableObjectiveMenu(&mem) {
			if err := skill.CloseOpenMenuToOverworld(m); err != nil {
				return fmt.Errorf("%w: close leftover menu: %v", ErrObjectiveBoundaryDirty, err)
			}
			continue
		}
		if state.DecodeTwoOptionMenu(&mem) != nil {
			return ErrObjectiveBoundaryChoice
		}
		if state.MenuUp(&mem) {
			return fmt.Errorf("%w: non-dismissable menu remains open", ErrObjectiveBoundaryDirty)
		}
		if state.DecodeDialogue(&mem) != nil {
			res := skill.RecoverDialogue(m, roundRecoveryBudget)
			if res.Stop != skill.DialogueRecovered {
				if res.Stop == skill.DialogueChoiceRequired {
					return ErrObjectiveBoundaryChoice
				}
				return fmt.Errorf("%w: leftover dialogue did not recover: %s", ErrObjectiveBoundaryDirty, recoveryStopName(res.Stop))
			}
			continue
		}
		return fmt.Errorf("%w: player is not controllable and no recoverable menu or dialogue is open", ErrObjectiveBoundaryDirty)
	}
	return fmt.Errorf("%w: cleanup did not converge after %d passes", ErrObjectiveBoundaryDirty, maxPasses)
}

func prepareObjectiveBoundary(m *emu.Emu) error {
	return normalizeObjectiveBoundary(m)
}

func settleObjectiveBoundary(m *emu.Emu) error {
	return normalizeObjectiveBoundary(m)
}
