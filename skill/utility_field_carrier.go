package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// RepairUtilityFieldCapability is the Cut/Flash-friendly variant of roster
// repair. A solo lead that can legally learn the HM is still progression-safe,
// but permanently occupying one of its four move slots is avoidable when a
// compatible boxed or reachable wild carrier is available. In that one case
// we look for a second Pokemon first; otherwise the generic repair engine owns
// the operation. If no helper can be obtained cheaply, the lead remains the
// deterministic fallback so utility preference can never strand progression.
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
	if party.Count != 1 || !CanPrepareFieldMove(romData, &mem, target) {
		return RepairFieldCapabilities(m, romData, policy, []FieldMove{target})
	}

	required := []FieldMove{target}
	box := state.DecodeBox(&mem)
	boxIndex, _, boxOK, err := chooseCompatibleBoxMon(romData, party, box, target, required)
	if err != nil {
		return fmt.Errorf("skill: RepairUtilityFieldCapability: plan PC carrier for %s: %w", target, err)
	}
	if boxOK {
		if err := WithdrawBoxMon(m, romData, policy, boxIndex); err != nil {
			return fmt.Errorf("skill: RepairUtilityFieldCapability: withdraw %s carrier from box index %d: %w", target, boxIndex, err)
		}
		if _, err := EnsureFieldMove(m, target); err != nil {
			return fmt.Errorf("skill: RepairUtilityFieldCapability: teach %s to withdrawn carrier: %w", target, err)
		}
		return nil
	}

	candidate, wildOK, err := findWildFieldCandidate(m, romData, target, required)
	if err != nil {
		return fmt.Errorf("skill: RepairUtilityFieldCapability: find wild %s carrier: %w", target, err)
	}
	if !wildOK {
		_, err := EnsureFieldMove(m, target)
		return err
	}

	_, balls := bagEntry(&mem, ItemPokeBall)
	if balls <= 0 {
		_, err := EnsureFieldMove(m, target)
		return err
	}
	if _, err := TravelFlee(m, romData, candidate.Destination, policy, pcTravelBattles); err != nil {
		return fmt.Errorf("%w: reach map %#04x for utility carrier species %#02x: %v", ErrFieldRosterCatch, candidate.Map, candidate.Species, err)
	}
	result, err := Catch(m, romData, []uint8{candidate.Species}, policy, minInt(balls, 10))
	if err != nil {
		return fmt.Errorf("%w: catch utility carrier species %#02x for %s: %v", ErrFieldRosterCatch, candidate.Species, target, err)
	}
	if result.Outcome != OutcomeCaught || result.Species != candidate.Species {
		return fmt.Errorf("%w: utility carrier species %#02x for %s ended with outcome %d after %d balls", ErrFieldRosterCatch, candidate.Species, target, result.Outcome, result.BallsThrown)
	}
	if _, err := EnsureFieldMove(m, target); err != nil {
		return fmt.Errorf("skill: RepairUtilityFieldCapability: teach %s after catching species %#02x: %w", target, candidate.Species, err)
	}
	return nil
}
