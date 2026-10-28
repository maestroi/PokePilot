package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
)

// trainHealResumeSegments bounds how many Center heal trips one Train
// objective may take after a retreat. Oddish grinding weak Route 22 grass
// (triage 0bba5e027eaeb5c0) needs at least one heal to finish L17→19 without
// leaving a productive shortfall for the planner to re-select.
const trainHealResumeSegments = 3

// trainHealTravelBattles is the battle allowance for the Center commute and
// return-to-grass legs. It is separate from the grind session budget so a
// heal-resume cannot steal the encounters needed to finish the level target.
const trainHealTravelBattles = 40

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

// dropUnviableTargetedTraining removes targeted training (such as a distant
// dex-evolution threshold) that one bounded Train session cannot reach here.
// The executor rejects those without input, so offering them only lets
// combat preparation or the planner pick the same dead objective per map.
func dropUnviableTargetedTraining(out []Objective, estimate partyTrainingEstimator) []Objective {
	kept := out[:0]
	for _, o := range out {
		if o.Kind == KindTrain && o.Species != "" {
			if est, err := estimate(o.Slot, int(o.Level)); err == nil && est.Viability == TrainingOutsideBudget {
				continue
			}
		}
		kept = append(kept, o)
	}
	return kept
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
	estimate, err := estimateTraining(romData, mon.Species, currentXP, mon.Level, slots, targetLevel, budget)
	if err != nil {
		return TrainingEstimate{}, err
	}
	return applyPartyTrainingMethod(party, slot, slots, estimate), nil
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

func trainingEvidenceFromRed(train skill.TrainResult) *TrainingEvidence {
	return &TrainingEvidence{
		StartLevel: train.StartLevel,
		EndLevel:   train.EndLevel,
		Battles:    train.Battles,
		BlackedOut: train.BlackedOut,
		Reached:    train.Reached,
		Retreated:  train.Retreated,
		Method:     string(train.Mode),
	}
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

// trainingSessionExecutable reports whether a bounded Train session may start in
// the measured habitat. A session is executable only if it can make progress:
// it must award wild XP and be able to reach at least the next level within its
// budget. Two classes make zero progress and are typed strategic blocks: a band
// that awards no XP at all (unsafe encounter band with no usable carry, or no
// encounter table), and a band whose XP rate cannot deliver even one level in
// the budget. A target that needs more than one session is still executable:
// it reaches the next level, and Train reports the remainder as a productive
// shortfall (ErrTrainProgress) instead of failing. Whether training here is
// worth starting is the provider's campaign-aware decision, not the executor's.
func trainingSessionExecutable(estimate TrainingEstimate, estimateErr error) bool {
	if estimateErr != nil {
		return true
	}
	if estimate.Viability == TrainingSatisfied {
		return true
	}
	if estimate.XPPerEncounter == 0 {
		return false
	}
	return estimate.NextLevelEncounters <= trainSessionBattles(estimate)
}

// leadTrainingSessionOfferable reports whether the default lead Train choice
// should appear in the menu. Combat-readiness may keep an expensive habitat on
// the table, but never an unexecutable OutsideBudget band: the executor would
// refuse every attempt and the run would stagnate on the same typed block.
func leadTrainingSessionOfferable(obs Observation, known *Knowledge) bool {
	if obs.Training == nil {
		return true
	}
	if !trainingSessionExecutable(*obs.Training, nil) {
		return false
	}
	if !trainingUnviableHere(obs) {
		return true
	}
	return trainingYieldsXP(obs) && owesCombatReadiness(known, obs)
}

// executeTrainingObjective temporarily promotes the requested party member to
// slot 0 because Red only awards battle XP to participating slots. A clean or
// progress-making session swaps the original lead back afterward. Retreats and
// blackouts deliberately keep the trained member active: Run's established
// recovery bookkeeping reads the active mon's ending level before replanning.
//
// When a session retreats short of the target, the objective owns a bounded
// heal-resume: travel to the nearest Pokemon Center, heal, return to the grind
// tile, and continue with the remaining battle budget. Leaving that commute to
// a later planner round is how productive shortfalls stagnated
// (triage 0bba5e027eaeb5c0: Oddish reached L18 then retreated on Route 22).
func executeTrainingObjective(m *emu.Emu, romData []byte, o Objective, result ObjectiveResult) (ObjectiveResult, error) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if redSafariTrainingMap(m.Peek8(sym.CurMap)) {
		result.Outcome = OutcomeBlocked
		return result, fmt.Errorf("agent: %s: %w: Safari Game habitats use capture-only encounters and cannot award ordinary training XP", o, ErrTrainingInefficient)
	}
	slot, err := resolveTrainingPartySlot(&mem, o)
	if err != nil {
		return result, err
	}
	estimate, estimateErr := currentPartyTrainingEstimate(&mem, romData, m.Peek8(sym.CurMap), slot, int(o.Level), trainSessionBattleBudget)
	if !trainingSessionExecutable(estimate, estimateErr) {
		result.Outcome = OutcomeBlocked
		return result, fmt.Errorf("agent: %s: %w", o, &TrainingInefficientError{Estimate: estimate})
	}

	promoted := slot > 0
	if promoted {
		if err := promoteToLeadStable(m, slot); err != nil {
			return result, fmt.Errorf("agent: %s: promote target to lead: %w", o, err)
		}
	}

	trainOptions := skill.TrainOptions{Mode: skill.TrainDirect}
	if estimateErr == nil && estimate.Method == TrainingSwitch {
		trainOptions.Mode = skill.TrainSwitch
		trainOptions.MinCarryLevel = estimate.MinCarryLevel
	}
	battles := trainSessionBattleBudget
	if estimateErr == nil {
		battles = trainSessionBattles(estimate)
	}
	policy := skill.StatAwareMove(romData)
	grindReturn := skill.Destination{Map: m.Peek8(sym.CurMap), X: m.Peek8(sym.XCoord), Y: m.Peek8(sym.YCoord)}

	var (
		train    skill.TrainResult
		trainErr error
		spent    int
	)
	for segment := 0; segment < trainHealResumeSegments; segment++ {
		remaining := battles - spent
		if remaining <= 0 {
			break
		}
		seg, segErr := skill.TrainWithOptions(m, romData, int(o.Level), policy, remaining, trainOptions)
		if segment == 0 {
			train = seg
		} else {
			train.Battles += seg.Battles
			train.Fled += seg.Fled
			train.EndLevel = seg.EndLevel
			train.Reached = seg.Reached
			train.Retreated = seg.Retreated
			train.BlackedOut = train.BlackedOut || seg.BlackedOut
			train.Mode = seg.Mode
		}
		spent += seg.Battles + seg.Fled
		trainErr = segErr
		if segErr != nil || seg.Reached || seg.BlackedOut || !seg.Retreated {
			break
		}
		// Retreat with budget left: heal at the nearest Center and resume on
		// the same grass. Failure to heal/return keeps the retreat outcome so
		// Run's ordinary train_retreat recovery still applies.
		if _, _, healTravelErr := skill.ReachNearestPokemonCenter(m, romData, policy, trainHealTravelBattles); healTravelErr != nil {
			break
		}
		if healErr := skill.Heal(m); healErr != nil {
			trainErr = fmt.Errorf("heal after training retreat: %w", healErr)
			break
		}
		if _, returnErr := skill.Travel(m, romData, grindReturn, policy, trainHealTravelBattles); returnErr != nil {
			trainErr = fmt.Errorf("return to training grass after heal: %w", returnErr)
			break
		}
		grindReturn = skill.Destination{Map: m.Peek8(sym.CurMap), X: m.Peek8(sym.XCoord), Y: m.Peek8(sym.YCoord)}
	}
	result.Train = trainingEvidenceFromRed(train)

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
