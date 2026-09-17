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
}

// Diagnostic is deliberately compact enough for objective notes/history while
// retaining the quantitative reason behind the class.
func (e TrainingEstimate) Diagnostic() string {
	switch e.Viability {
	case TrainingSatisfied:
		return fmt.Sprintf("training target L%d already satisfied", e.TargetLevel)
	case TrainingOutsideBudget:
		if e.XPPerEncounter == 0 {
			return fmt.Sprintf("training outside current session budget: no usable wild XP estimate; budget %d battles", e.SessionBudget)
		}
		return fmt.Sprintf("training outside current session budget: ~%d encounters for %d XP at ~%d XP/encounter; budget %d battles", e.EstimatedEncounters, e.XPRemaining, e.XPPerEncounter, e.SessionBudget)
	case TrainingExpensive:
		return fmt.Sprintf("training expensive: ~%d/%d encounters for %d XP at ~%d XP/encounter", e.EstimatedEncounters, e.SessionBudget, e.XPRemaining, e.XPPerEncounter)
	default:
		return fmt.Sprintf("training viable: ~%d/%d encounters for %d XP at ~%d XP/encounter", e.EstimatedEncounters, e.SessionBudget, e.XPRemaining, e.XPPerEncounter)
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
	party := skill.AddressesForROM(romData).DecodeParty(mem)
	if len(party.Mons) == 0 {
		return TrainingEstimate{}, fmt.Errorf("agent: training estimate requires a party lead")
	}
	currentXP, ok := state.PartyExperience(mem, 0)
	if !ok {
		return TrainingEstimate{}, fmt.Errorf("agent: training estimate could not read lead experience")
	}
	slots, err := skill.WildGrassSlots(romData, mapID)
	if err != nil {
		return TrainingEstimate{}, err
	}
	lead := party.Mons[0]
	return estimateTraining(romData, lead.Species, currentXP, lead.Level, slots, targetLevel, budget)
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
