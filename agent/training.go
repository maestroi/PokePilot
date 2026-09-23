package agent

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// trainSessionBattleBudget is the deterministic engagement bound shared by
// planner viability estimates and the Train executor. Keeping one value avoids
// telling the strategist an action is viable under a different budget than the
// skill will actually receive.
const trainSessionBattleBudget = 20

// TrainingViability is a compact planner-facing cost class for the default
// local training objective. It is derived from current cumulative XP and ROM
// encounter/base-stat data, never gym-specific strategy.
type TrainingViability string

const (
	TrainingSatisfied     TrainingViability = "satisfied"
	TrainingViable        TrainingViability = "viable"
	TrainingExpensive     TrainingViability = "expensive"
	TrainingOutsideBudget TrainingViability = "outside_budget"
)

// TrainingMethod distinguishes ordinary grinding from deliberate switch
// training. In switch training the target starts every wild battle so Red
// marks it as a participant, then a healthy stronger party member is sent in
// to finish the fight. Gen 1 splits battle XP across participants, so estimates
// must account for that cost instead of pretending the weak target receives
// the full reward.
type TrainingMethod string

const (
	TrainingDirect TrainingMethod = "direct"
	TrainingSwitch TrainingMethod = "switch"
)

// TrainingEstimate explains whether the current grass can plausibly deliver a
// concrete level target within one bounded Train session.
type TrainingEstimate struct {
	CurrentLevel        uint8             `json:"current_level"`
	TargetLevel         uint8             `json:"target_level"`
	XPRemaining         uint32            `json:"xp_remaining"`
	XPPerEncounter      uint32            `json:"xp_per_encounter"`
	EstimatedEncounters int               `json:"estimated_encounters"`
	SessionBudget       int               `json:"session_budget"`
	Viability           TrainingViability `json:"viability"`
	Method              TrainingMethod    `json:"method,omitempty"`
	WildMaxLevel        uint8             `json:"wild_max_level,omitempty"`
	CarryLevel          uint8             `json:"carry_level,omitempty"`
	MinCarryLevel       uint8             `json:"min_carry_level,omitempty"`
}

// Diagnostic is deliberately compact enough for objective notes/history while
// retaining the quantitative reason behind the class.
func (e TrainingEstimate) Diagnostic() string {
	method := ""
	if e.Method == TrainingSwitch {
		method = fmt.Sprintf("; switch training via L%d carry against wilds up to L%d (shared XP)", e.CarryLevel, e.WildMaxLevel)
	}
	switch e.Viability {
	case TrainingSatisfied:
		return fmt.Sprintf("training target L%d already satisfied", e.TargetLevel)
	case TrainingOutsideBudget:
		if e.XPPerEncounter == 0 {
			if e.WildMaxLevel > 0 && e.Method == TrainingDirect && e.MinCarryLevel > 0 {
				return fmt.Sprintf("training unsafe here: wilds reach L%d and no healthy carry meets L%d; budget %d battles", e.WildMaxLevel, e.MinCarryLevel, e.SessionBudget)
			}
			return fmt.Sprintf("training outside current session budget: no usable wild XP estimate; budget %d battles", e.SessionBudget)
		}
		return fmt.Sprintf("training outside current session budget: ~%d encounters for %d XP at ~%d XP/encounter; budget %d battles%s", e.EstimatedEncounters, e.XPRemaining, e.XPPerEncounter, e.SessionBudget, method)
	case TrainingExpensive:
		return fmt.Sprintf("training expensive: ~%d/%d encounters for %d XP at ~%d XP/encounter%s", e.EstimatedEncounters, e.SessionBudget, e.XPRemaining, e.XPPerEncounter, method)
	default:
		return fmt.Sprintf("training viable: ~%d/%d encounters for %d XP at ~%d XP/encounter%s", e.EstimatedEncounters, e.SessionBudget, e.XPRemaining, e.XPPerEncounter, method)
	}
}

var ErrTrainingInefficient = errors.New("agent: insufficient XP opportunity in current training area")

// TrainingInefficientError is a typed strategic blockage, not a controller
// fault. #145 can therefore retain/replan the failed objective while telemetry
// still distinguishes the root cause from battle or navigation failures.
type TrainingInefficientError struct {
	Estimate TrainingEstimate
}

func (e *TrainingInefficientError) Error() string {
	if e == nil {
		return ErrTrainingInefficient.Error()
	}
	return ErrTrainingInefficient.Error() + ": " + e.Estimate.Diagnostic()
}

func (e *TrainingInefficientError) Unwrap() error { return ErrTrainingInefficient }

func currentTrainingEstimate(mem *state.Mem, romData []byte, mapID uint8, targetLevel, budget int) (TrainingEstimate, error) {
	return currentPartyTrainingEstimate(mem, romData, mapID, 0, targetLevel, budget)
}

func currentTrainingEstimateFromEmu(m *emu.Emu, romData []byte, mapID uint8, targetLevel, budget int) (TrainingEstimate, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return currentTrainingEstimate(&mem, romData, mapID, targetLevel, budget)
}

func estimateTraining(romData []byte, leadSpecies uint8, currentXP uint32, currentLevel uint8, slots []skill.WildEncounterSlot, targetLevel, budget int) (TrainingEstimate, error) {
	if currentLevel < 1 || currentLevel > 100 {
		return TrainingEstimate{}, fmt.Errorf("agent: training estimate lead level %d outside 1..100", currentLevel)
	}
	if targetLevel < 1 || targetLevel > 100 {
		return TrainingEstimate{}, fmt.Errorf("agent: training target level %d outside 1..100", targetLevel)
	}
	if budget <= 0 {
		return TrainingEstimate{}, fmt.Errorf("agent: training battle budget must be positive, got %d", budget)
	}

	leadXP, err := rom.LookupSpeciesExperience(romData, leadSpecies)
	if err != nil {
		return TrainingEstimate{}, fmt.Errorf("agent: training lead experience policy: %w", err)
	}
	floorXP, err := rom.ExperienceAtLevel(leadXP.Growth, int(currentLevel))
	if err != nil {
		return TrainingEstimate{}, err
	}
	if currentXP < floorXP {
		// A settled save should never report cumulative XP below its current
		// level threshold. Clamp synthetic/transitional states so the estimate
		// cannot overstate work because of a stale three-byte read.
		currentXP = floorXP
	}
	targetXP, err := rom.ExperienceAtLevel(leadXP.Growth, targetLevel)
	if err != nil {
		return TrainingEstimate{}, err
	}

	estimate := TrainingEstimate{
		CurrentLevel:  currentLevel,
		TargetLevel:   uint8(targetLevel),
		SessionBudget: budget,
		Method:        TrainingDirect,
	}
	if targetLevel <= int(currentLevel) || currentXP >= targetXP {
		estimate.Viability = TrainingSatisfied
		return estimate, nil
	}
	estimate.XPRemaining = targetXP - currentXP

	var (
		equalXP         uint64
		weightedXP      uint64
		weightTotal     uint64
		count           uint64
		completeWeights = true
	)
	for _, slot := range slots {
		if slot.ID == 0 || slot.Level == 0 {
			continue
		}
		wildXP, err := rom.LookupSpeciesExperience(romData, slot.ID)
		if err != nil {
			return TrainingEstimate{}, fmt.Errorf("agent: training wild species %#02x: %w", slot.ID, err)
		}
		xp := uint64(rom.WildBattleExperience(wildXP.BaseYield, slot.Level))
		equalXP += xp
		count++
		if slot.Chance == 0 {
			completeWeights = false
			continue
		}
		weightedXP += xp * uint64(slot.Chance)
		weightTotal += uint64(slot.Chance)
	}
	if count == 0 || equalXP == 0 {
		estimate.Viability = TrainingOutsideBudget
		return estimate, nil
	}

	// Red does not choose its ten wild slots uniformly. WildGrassSlots carries
	// the exact probability mass from data/wild/probabilities.asm, so use that
	// distribution when it is available. Synthetic callers/tests created before
	// Chance was exposed may omit it; preserve their old equal-slot semantics
	// rather than silently treating zero as zero probability.
	if completeWeights && weightTotal > 0 {
		estimate.XPPerEncounter = uint32((weightedXP + weightTotal/2) / weightTotal)
	} else {
		estimate.XPPerEncounter = uint32((equalXP + count/2) / count)
	}
	if estimate.XPPerEncounter == 0 {
		estimate.Viability = TrainingOutsideBudget
		return estimate, nil
	}
	estimate.EstimatedEncounters = int((uint64(estimate.XPRemaining) + uint64(estimate.XPPerEncounter) - 1) / uint64(estimate.XPPerEncounter))

	switch {
	case estimate.EstimatedEncounters > budget:
		estimate.Viability = TrainingOutsideBudget
	case estimate.EstimatedEncounters*4 > budget*3:
		estimate.Viability = TrainingExpensive
	default:
		estimate.Viability = TrainingViable
	}
	return estimate, nil
}


const (
	// A target more than three levels below the strongest local wild is treated
	// as unsafe for direct grinding. The actual battle layer still evaluates
	// type/move matchups turn by turn; this is only the coarse pre-battle gate
	// that decides whether the weak mon should be protected by switch training.
	directTrainingWildGap = 3
	// A carry may trail the strongest local wild slightly because the battle
	// switch policy also considers moves, HP and typing. Requiring it to be at
	// least within two levels keeps "L12 target + L15 backup in Victory Road"
	// from being mislabeled as safe switch training.
	carryWildLevelSlack = 2
)

func strongestWildLevel(slots []skill.WildEncounterSlot) uint8 {
	var max uint8
	for _, slot := range slots {
		if slot.Level > max {
			max = slot.Level
		}
	}
	return max
}

func minimumTrainingCarryLevel(targetLevel, wildMax uint8) uint8 {
	min := uint8(1)
	if wildMax > carryWildLevelSlack {
		min = wildMax - carryWildLevelSlack
	}
	// The carry should also be materially ahead of the target; otherwise a
	// second mediocre mon merely shares XP while adding another failure mode.
	if targetLevel <= 95 && targetLevel+5 > min {
		min = targetLevel + 5
	}
	return min
}

func bestTrainingCarry(party state.PartyState, targetSlot int, minLevel uint8) (int, uint8, bool) {
	bestSlot := -1
	var bestLevel uint8
	for slot, mon := range party.Mons {
		if slot == targetSlot || mon.Fainted() || mon.Level < minLevel || mon.StatusName() == "frozen" {
			continue
		}
		if mon.MaxHP > 0 && mon.HP*2 < mon.MaxHP {
			continue
		}
		usablePP := false
		for _, pp := range mon.PP {
			if pp > 0 {
				usablePP = true
				break
			}
		}
		if !usablePP {
			continue
		}
		if bestSlot < 0 || mon.Level > bestLevel {
			bestSlot, bestLevel = slot, mon.Level
		}
	}
	return bestSlot, bestLevel, bestSlot >= 0
}

func classifyTrainingEstimate(e *TrainingEstimate) {
	if e == nil || e.Viability == TrainingSatisfied {
		return
	}
	if e.XPPerEncounter == 0 {
		e.Viability = TrainingOutsideBudget
		e.EstimatedEncounters = 0
		return
	}
	e.EstimatedEncounters = int((uint64(e.XPRemaining) + uint64(e.XPPerEncounter) - 1) / uint64(e.XPPerEncounter))
	switch {
	case e.EstimatedEncounters > e.SessionBudget:
		e.Viability = TrainingOutsideBudget
	case e.EstimatedEncounters*4 > e.SessionBudget*3:
		e.Viability = TrainingExpensive
	default:
		e.Viability = TrainingViable
	}
}

// applyPartyTrainingMethod converts a raw direct-XP estimate into the method
// that is actually safe for this party in this encounter band. When direct
// training is unsafe but a healthy carry exists, Gen 1 will split XP between
// the target and carry, so the effective XP rate is halved and viability is
// recomputed. If no carry can safely take over, the area is rejected rather
// than letting a low-level target repeatedly black out.
func applyPartyTrainingMethod(party state.PartyState, targetSlot int, slots []skill.WildEncounterSlot, estimate TrainingEstimate) TrainingEstimate {
	wildMax := strongestWildLevel(slots)
	estimate.WildMaxLevel = wildMax
	if estimate.Viability == TrainingSatisfied || targetSlot < 0 || targetSlot >= len(party.Mons) || wildMax == 0 {
		return estimate
	}

	target := party.Mons[targetSlot]
	if int(wildMax) <= int(target.Level)+directTrainingWildGap {
		return estimate
	}

	minCarry := minimumTrainingCarryLevel(target.Level, wildMax)
	estimate.MinCarryLevel = minCarry
	_, carryLevel, ok := bestTrainingCarry(party, targetSlot, minCarry)
	if !ok {
		// Preserve the fact that XP itself may be plentiful, but make the method
		// non-executable in this area. Diagnostic() explains that safety, not XP,
		// is the blocker.
		estimate.Method = TrainingDirect
		estimate.XPPerEncounter = 0
		estimate.Viability = TrainingOutsideBudget
		estimate.EstimatedEncounters = 0
		return estimate
	}

	estimate.Method = TrainingSwitch
	estimate.CarryLevel = carryLevel
	// Two participating Pokémon split wild XP in Gen 1. Use the conservative
	// half-rate here; occasional later tactical switches can only make this
	// estimate safer, never overstate target XP.
	estimate.XPPerEncounter /= 2
	classifyTrainingEstimate(&estimate)
	return estimate
}
