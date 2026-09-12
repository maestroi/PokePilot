package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
)

type partyTrainingEstimator func(slot, targetLevel int) (TrainingEstimate, error)

// insertPartyTrainingObjectives adds one concrete training choice for each
// healthy non-lead party member that the current grass can plausibly advance
// within a bounded session. The strategist chooses which species is worth the
// time; deterministic code only exposes viable mechanics and costs.
func insertPartyTrainingObjectives(obs Observation, known *Knowledge, out []Objective, estimate partyTrainingEstimator) []Objective {
	if !obs.HasGrass || len(obs.Party) < 2 || estimate == nil {
		return out
	}

	extra := make([]Objective, 0, len(obs.Party)-1)
	for slot := 1; slot < len(obs.Party) && slot < 6; slot++ {
		mon := obs.Party[slot]
		if mon.Species == "" || mon.Level >= 100 || skill.BelowRetreatLine(mon.HP, mon.MaxHP) {
			continue
		}
		target := int(mon.Level) + trainStep
		est, err := estimate(slot, target)
		if err != nil || est.Viability == TrainingOutsideBudget || est.Viability == TrainingSatisfied {
			continue
		}
		extra = append(extra, Objective{
			Kind:    KindTrain,
			Level:   uint8(target),
			Species: mon.Species,
			Slot:    slot,
			Note:    partyTrainingChoiceNote(slot, mon, obs.Party[0], obs.WildGrass, &est),
		})
	}
	if len(extra) == 0 {
		return out
	}
	extra = annotate(extra, known)

	// Training is a local action. Keep it before journeys so the strategist's
	// menu still reads as local work first, travel second.
	at := len(out)
	for i, o := range out {
		if o.Kind == KindGoTo {
			at = i
			break
		}
	}
	merged := make([]Objective, 0, len(out)+len(extra))
	merged = append(merged, out[:at]...)
	merged = append(merged, extra...)
	merged = append(merged, out[at:]...)
	return merged
}

func partyTrainingChoiceNote(slot int, mon, lead PartyMon, wild []WildSpecies, estimate *TrainingEstimate) string {
	detail := fmt.Sprintf("party slot %d L%d; current lead L%d", slot, mon.Level, lead.Level)
	if len(wild) > 0 {
		minLevel, maxLevel := int(wild[0].MinLevel), int(wild[0].MaxLevel)
		for _, w := range wild[1:] {
			if int(w.MinLevel) < minLevel {
				minLevel = int(w.MinLevel)
			}
			if int(w.MaxLevel) > maxLevel {
				maxLevel = int(w.MaxLevel)
			}
		}
		detail += fmt.Sprintf("; local wilds L%d-L%d", minLevel, maxLevel)
	}
	if estimate != nil {
		detail += "; " + estimate.Diagnostic()
	}
	return "(" + detail + ")"
}

// currentPartyTrainingEstimate is currentTrainingEstimate for an arbitrary
// party slot. estimateTraining is already species/XP based; only the old
// wrapper was lead-specific.
func currentPartyTrainingEstimate(mem *state.Mem, romData []byte, mapID uint8, slot, targetLevel, budget int) (TrainingEstimate, error) {
	party := state.DecodeParty(mem)
	if slot < 0 || slot >= len(party.Mons) {
		return TrainingEstimate{}, fmt.Errorf("agent: training estimate party slot %d out of range for party of %d", slot, len(party.Mons))
	}
	currentXP, ok := state.PartyExperience(mem, slot)
	if !ok {
		return TrainingEstimate{}, fmt.Errorf("agent: training estimate could not read party slot %d experience", slot)
	}
	slots, err := skill.WildGrassSlots(romData, mapID)
	if err != nil {
		return TrainingEstimate{}, err
	}
	mon := party.Mons[slot]
	return estimateTraining(romData, mon.Species, currentXP, mon.Level, slots, targetLevel, budget)
}

// resolveTrainingPartySlot makes a species-targeted objective resilient to a
// prior party reorder. Slot is retained as a fast-path/hint and as a fallback
// for legacy lead objectives that do not carry Species.
func resolveTrainingPartySlot(mem *state.Mem, o Objective) (int, error) {
	party := state.DecodeParty(mem)
	if len(party.Mons) == 0 {
		return 0, fmt.Errorf("agent: %s: training requires a party", o)
	}
	if o.Species != "" {
		want, ok := redSpeciesID(o.Species)
		if !ok {
			return 0, fmt.Errorf("agent: %s: unknown Red species %q", o, o.Species)
		}
		if o.Slot >= 0 && o.Slot < len(party.Mons) && party.Mons[o.Slot].Species == want {
			return o.Slot, nil
		}
		for i, mon := range party.Mons {
			if mon.Species == want {
				return i, nil
			}
		}
		return 0, fmt.Errorf("agent: %s: target species %s is no longer in the party", o, o.Species)
	}
	if o.Slot < 0 || o.Slot >= len(party.Mons) {
		return 0, fmt.Errorf("agent: %s: party slot %d out of range for party of %d", o, o.Slot, len(party.Mons))
	}
	return o.Slot, nil
}

// promoteToLeadStable gives the verified party-swap primitive one recovery
// attempt when a menu transition wins the race against PromoteToLead's source-
// screen guard. The failure is not ignored: we only retry after proving the
// leftover surface is a dismissable menu and unwinding it to the overworld.
// This covers checkpoints/transition frames where the old menu disappears
// before the next party screen has finished drawing, without making unknown
// interaction failures broadly recoverable.
func promoteToLeadStable(m *emu.Emu, slot int) error {
	if err := skill.PromoteToLead(m, slot); err != nil {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if !skill.DismissableObjectiveMenu(&mem) {
			return err
		}
		if cleanupErr := skill.CloseOpenMenuToOverworld(m); cleanupErr != nil {
			return fmt.Errorf("%v; recover interrupted party menu: %w", err, cleanupErr)
		}
		if retryErr := skill.PromoteToLead(m, slot); retryErr != nil {
			return fmt.Errorf("%v; retry after menu recovery: %w", err, retryErr)
		}
	}
	return nil
}

// executeTrainingObjective temporarily promotes the requested party member to
// slot 0 because Red only awards battle XP to participating slots. A clean or
// progress-making session swaps the original lead back afterward. Retreats and
// blackouts deliberately keep the trained member active: Run's established
// recovery bookkeeping reads the active mon's ending level before replanning.
func executeTrainingObjective(m *emu.Emu, romData []byte, o Objective, result ObjectiveResult) (ObjectiveResult, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	slot, err := resolveTrainingPartySlot(&mem, o)
	if err != nil {
		return result, err
	}
	if estimate, estimateErr := currentPartyTrainingEstimate(&mem, romData, m.Peek8(sym.CurMap), slot, int(o.Level), trainSessionBattleBudget); estimateErr == nil && estimate.Viability == TrainingOutsideBudget {
		result.Outcome = OutcomeBlocked
		return result, fmt.Errorf("agent: %s: %w", o, &TrainingInefficientError{Estimate: estimate})
	}

	promoted := slot > 0
	if promoted {
		if err := promoteToLeadStable(m, slot); err != nil {
			return result, fmt.Errorf("agent: %s: promote target to lead: %w", o, err)
		}
	}

	train, trainErr := skill.Train(m, romData, int(o.Level), skill.StatAwareMove(romData), trainSessionBattleBudget)
	result.Train = &train

	// PromoteToLead is a symmetric swap: the original lead is still at the
	// same partner slot. Restore it whenever Train left a normal controllable
	// state. Do not restore retreat/blackout endings; Run intentionally reads
	// the active trained mon's level to decide whether recovery is progressing.
	if promoted && !train.Retreated && !train.BlackedOut {
		if restoreErr := promoteToLeadStable(m, slot); restoreErr != nil {
			if trainErr != nil {
				return result, fmt.Errorf("agent: %s: train failed after %d battles: %v; restore original lead: %w", o, train.Battles, trainErr, restoreErr)
			}
			return result, fmt.Errorf("agent: %s: restore original lead after training: %w", o, restoreErr)
		}
	}

	if trainErr != nil {
		return result, fmt.Errorf("agent: %s: train failed after %d battles: %w", o, train.Battles, trainErr)
	}
	if train.Reached {
		return result, nil
	}
	result.Outcome = OutcomeBlocked
	switch {
	case train.Retreated:
		return result, fmt.Errorf("agent: %s: %w (ended level %d)", o, skill.ErrTrainRetreat, train.EndLevel)
	case train.BlackedOut:
		return result, fmt.Errorf("agent: %s: %w before reaching level %d (ended level %d after %d battles)", o, skill.ErrBlackedOut, o.Level, train.EndLevel, train.Battles)
	case train.EndLevel > train.StartLevel:
		return result, fmt.Errorf("agent: %s: %w (target %d, ended level %d after %d battles)", o, skill.ErrTrainProgress, o.Level, train.EndLevel, train.Battles)
	default:
		return result, fmt.Errorf("agent: %s: target level %d not reached (ended level %d after %d battles)", o, o.Level, train.EndLevel, train.Battles)
	}
}
