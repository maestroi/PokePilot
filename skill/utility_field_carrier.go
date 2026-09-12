package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// utilityFieldRecoveryBlocked keeps an exhausted Cut/Flash roster recovery on
// the normal replan path. ErrFieldMovePrerequisite is already a typed gameplay
// blockage at the agent boundary, while cause preserves the more specific
// roster identity for diagnostics and future recovery policy.
func utilityFieldRecoveryBlocked(target FieldMove, cause error, format string, args ...any) error {
	detail := fmt.Sprintf(format, args...)
	if cause == nil {
		return fmt.Errorf("%w: %s recovery blocked: %s", ErrFieldMovePrerequisite, target, detail)
	}
	return fmt.Errorf("%w: %w: %s recovery blocked: %s", ErrFieldMovePrerequisite, cause, target, detail)
}

// RepairUtilityFieldCapability is the Cut/Flash-friendly variant of roster
// repair. It prefers a bench/boxed/wild utility carrier over permanently
// occupying a solo lead's move slot, but progression always wins: a compatible
// lead remains the deterministic fallback. When the current party cannot learn
// the move at all, this function owns the complete box/catch recovery instead
// of silently delegating to a route-agnostic candidate search.
func RepairUtilityFieldCapability(m *emu.Emu, romData []byte, policy MovePolicy, target FieldMove) error {
	if target != FieldCut && target != FieldFlash {
		return RepairFieldCapabilities(m, romData, policy, []FieldMove{target})
	}
	if policy == nil {
		return fmt.Errorf("skill: RepairUtilityFieldCapability: nil move policy")
	}

	var mem state.Mem
	state.Snapshot(m, &mem)
	cap := FieldCapabilityFor(&mem, target)
	if !cap.BadgeOwned || !cap.HMOwned {
		return fmt.Errorf("%w: %s badge=%v HM=%v", ErrFieldRosterPrerequisite, cap.Name, cap.BadgeOwned, cap.HMOwned)
	}
	if cap.Usable {
		return nil
	}

	party := state.DecodeParty(&mem)
	canPrepareHere := CanPrepareFieldMove(romData, &mem, target)
	// With more than one current party member the generic TM/HM decision already
	// prefers a compatible bench carrier for Cut/Flash. There is no reason to
	// leave the current roster and hunt another Pokemon first.
	if canPrepareHere && party.Count != 1 {
		if _, err := EnsureFieldMove(m, target); err != nil {
			return fmt.Errorf("skill: RepairUtilityFieldCapability: prepare %s in current party: %w", target, err)
		}
		return nil
	}

	required := []FieldMove{target}
	box := state.DecodeBox(&mem)
	boxIndex, depositSlot, boxOK, err := chooseCompatibleBoxMon(romData, party, box, target, required)
	if err != nil {
		return fmt.Errorf("skill: RepairUtilityFieldCapability: plan PC carrier for %s: %w", target, err)
	}
	if boxOK {
		if party.Count >= gen1PartyCapacity {
			if err := DepositPartyMon(m, romData, policy, depositSlot); err != nil {
				return fmt.Errorf("skill: RepairUtilityFieldCapability: make party room for boxed %s carrier: %w", target, err)
			}
		}
		if err := WithdrawBoxMon(m, romData, policy, boxIndex); err != nil {
			return fmt.Errorf("skill: RepairUtilityFieldCapability: withdraw %s carrier from box index %d: %w", target, boxIndex, err)
		}
		if _, err := EnsureFieldMove(m, target); err != nil {
			return fmt.Errorf("skill: RepairUtilityFieldCapability: teach %s to withdrawn carrier: %w", target, err)
		}
		return nil
	}

	candidate, wildOK, err := findReachableWildFieldCandidate(m, romData, target, required)
	if err != nil {
		return fmt.Errorf("skill: RepairUtilityFieldCapability: find reachable wild %s carrier: %w", target, err)
	}
	if !wildOK {
		// A solo compatible lead is the last-resort utility carrier. This keeps
		// the preference for a helper Pokemon from ever blocking progression.
		if canPrepareHere {
			if _, err := EnsureFieldMove(m, target); err != nil {
				return fmt.Errorf("skill: RepairUtilityFieldCapability: fallback teach %s to lead: %w", target, err)
			}
			return nil
		}
		return utilityFieldRecoveryBlocked(target, ErrFieldRosterNoRecovery,
			"no compatible current-party member, active-box member, or semantically reachable wild species")
	}

	state.Snapshot(m, &mem)
	party = state.DecodeParty(&mem)
	incoming := state.Mon{Species: candidate.Species}
	depositSlot, legal, err := chooseDepositSlotForIncoming(romData, party, incoming, required)
	if err != nil {
		return fmt.Errorf("skill: RepairUtilityFieldCapability: plan party room for wild species %#02x: %w", candidate.Species, err)
	}
	if !legal {
		if canPrepareHere {
			if _, err := EnsureFieldMove(m, target); err != nil {
				return fmt.Errorf("skill: RepairUtilityFieldCapability: fallback teach %s to lead: %w", target, err)
			}
			return nil
		}
		return utilityFieldRecoveryBlocked(target, ErrFieldRosterNoRecovery,
			"wild species %#02x cannot be added without stranding the required field capability", candidate.Species)
	}

	state.Snapshot(m, &mem)
	_, balls := bagEntry(&mem, ItemPokeBall)
	if balls <= 0 {
		if canPrepareHere {
			if _, err := EnsureFieldMove(m, target); err != nil {
				return fmt.Errorf("skill: RepairUtilityFieldCapability: fallback teach %s to lead: %w", target, err)
			}
			return nil
		}
		return fmt.Errorf("%w: compatible wild species %#02x exists on map %#04x but no POKE BALL is available", ErrFieldRosterNoBalls, candidate.Species, candidate.Map)
	}

	if party.Count >= gen1PartyCapacity {
		if err := DepositPartyMon(m, romData, policy, depositSlot); err != nil {
			return fmt.Errorf("skill: RepairUtilityFieldCapability: make room for wild species %#02x: %w", candidate.Species, err)
		}
	}
	if _, err := TravelFlee(m, romData, candidate.Destination, policy, pcTravelBattles); err != nil {
		// Preserve the underlying route/controller identity. The previous %v
		// erased it, turning a normal blocked recovery into unknown_failure.
		return fmt.Errorf("%w: reach map %#04x for utility carrier species %#02x: %w", ErrFieldRosterCatch, candidate.Map, candidate.Species, err)
	}
	result, err := Catch(m, romData, []uint8{candidate.Species}, policy, minInt(balls, 10))
	if err != nil {
		return fmt.Errorf("%w: catch utility carrier species %#02x for %s: %w", ErrFieldRosterCatch, candidate.Species, target, err)
	}
	if result.Outcome != OutcomeCaught || result.Species != candidate.Species {
		state.Snapshot(m, &mem)
		_, remaining := bagEntry(&mem, ItemPokeBall)
		if remaining <= 0 {
			return fmt.Errorf("%w: utility carrier species %#02x for %s was not caught after %d balls", ErrFieldRosterNoBalls, candidate.Species, target, result.BallsThrown)
		}
		return utilityFieldRecoveryBlocked(target, ErrFieldRosterCatch,
			"utility carrier species %#02x was not caught (outcome %d after %d balls)", candidate.Species, result.Outcome, result.BallsThrown)
	}
	if _, err := EnsureFieldMove(m, target); err != nil {
		return fmt.Errorf("skill: RepairUtilityFieldCapability: teach %s after catching species %#02x: %w", target, candidate.Species, err)
	}
	return nil
}
