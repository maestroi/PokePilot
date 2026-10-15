package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

const (
	gsCombatRecoveryMaxLevel         = 60
	gsCombatRecoverySessionLevelGain = 2
)

// gsProgressionCombatRecovery offers a *real*, adapter-executable preparation
// action when the generic combat-loss gate has withheld a story fight. Gold
// previously offered only progress objectives: losing Morty left no legal
// actions, so the farm replayed the same pre-fight checkpoint indefinitely.
// Only a typed failure of the CURRENT unfinished story step can unlock this
// detour; old successes or unrelated script errors never start a grind.
func gsProgressionCombatRecovery(obs Observation, progress Objective) []Objective {
	if progress.Kind != KindProgress || obs.PartyCount == 0 || len(obs.Party) == 0 {
		return nil
	}
	if !obs.Story.Has(gsprofile.ProgressSlowpokeWellCleared) {
		return nil // no proven native Gen2 wild-grind corridor yet
	}
	lead := obs.Party[0]
	if lead.IsEgg || lead.Level == 0 || lead.Level >= gsCombatRecoveryMaxLevel {
		return nil
	}
	var loss *Failure
	for i := range obs.Failures {
		f := &obs.Failures[i]
		if f.Times <= 0 {
			continue
		}
		if f.Key != nil && *f.Key == progress.Key() {
			loss = f
			break
		}
		if f.Objective == progress.String() && (f.ReadinessTarget > 0 || f.ReadinessBaseline > 0) {
			loss = f
			break
		}
	}
	if loss == nil {
		return nil
	}

	target := gsCombatRecoveryLeadTarget(obs, *loss)
	if target <= int(lead.Level) {
		return nil
	}
	return []Objective{{
		Kind:  KindTrain,
		Slot:  0,
		Level: uint8(target),
		Note:  fmt.Sprintf("(Gold/Silver recovery after %d loss(es) at %s: train the lead in a proven wild habitat before retrying)", loss.Times, progress),
	}}
}

// Generic combat readiness counts the strongest three members with weights
// 4/2/1. A single-lead Gen2 recovery can increase at least four readiness
// points per level, but never invents a new species or a distant training map.
// The stored quantified target remains authoritative. Legacy losses without a
// target use a conservative bounded five-level training session instead.
//
// Levels at or above failure.ReadinessCeiling do not raise failureReadiness
// (see partyCombatReadinessUnder). Gen2 recovery only trains the lead, so once
// the carry is at that ceiling any remaining deficit belongs to the bench —
// the same invariant as preparationTrainingObjective. Offering more lead XP
// would grind forever under a target that lead levels cannot pay
// (run-3v7t9gqr8gnks2efu8haxz0j2c: Jasmine ceiling 40, lead L53→L60).
func gsCombatRecoveryLeadTarget(obs Observation, failure Failure) int {
	if len(obs.Party) == 0 {
		return 0
	}
	current := int(obs.Party[0].Level)
	target := current + 5
	if failure.ReadinessTarget > 0 {
		deficit := failure.ReadinessTarget - failureReadiness(obs, failure)
		if deficit <= 0 {
			return 0 // quantified goal met: retry the fight, do not grind
		}
		if c := failure.ReadinessCeiling; c > 0 && current >= c {
			return 0 // lead grind cannot raise readiness past the ceiling
		}
		target = current + (deficit+3)/4
		if c := failure.ReadinessCeiling; c > 0 && target > c {
			target = c
		}
	}
	if target <= current {
		return 0
	}
	// The same lead grind is proven over a short two-level Burned Tower
	// preparation, not an unbounded five-to-ten-level atomic objective.
	// Split a larger quantified campaign into verified rungs so the
	// objective watchdog and restart checkpoints remain useful.
	if target > current+gsCombatRecoverySessionLevelGain {
		target = current + gsCombatRecoverySessionLevelGain
	}
	if target > gsCombatRecoveryMaxLevel {
		target = gsCombatRecoveryMaxLevel
	}
	return target
}

// gsLeadOnlyRecoveryCanAdvance reports whether Gen2's lead-only wild grind can
// still move an open quantified combat-prep campaign. When every unmet
// readiness target already has the lead at its ceiling, more lead XP is a
// no-op under failureReadiness and the portable no-corridor escape must
// promote the loss to retry instead of locking the fight.
func gsLeadOnlyRecoveryCanAdvance(obs Observation) bool {
	if len(obs.Party) == 0 {
		return false
	}
	lead := int(obs.Party[0].Level)
	sawUnmet := false
	for _, f := range obs.Failures {
		if f.Times <= 0 || f.ReadinessTarget <= 0 {
			continue
		}
		if failureReadiness(obs, f) >= f.ReadinessTarget {
			continue
		}
		sawUnmet = true
		if f.ReadinessCeiling <= 0 || lead < f.ReadinessCeiling {
			return true
		}
	}
	// No quantified unmet campaign: a legacy (target-less) loss can still use
	// the lead grind, and a satisfied campaign needs no corridor.
	return !sawUnmet
}

func gsRecoveryGrindSite(obs Observation) (string, uint8, uint8, error) {
	switch {
	case obs.Story.Has(gsprofile.ProgressSudowoodoCleared):
		// Route 37 was proven by the Gold Burned Tower rival reproduction
		// and remains a legal wild-encounter tile through the coastal frontier.
		return "ROUTE_37", 15, 14, nil
	case obs.Story.Has(gsprofile.ProgressSlowpokeWellCleared):
		// The B1F encounter corridor is already used by Bugsy preparation.
		return "SLOWPOKE_WELL_B1F", 5, 3, nil
	default:
		return "", 0, 0, fmt.Errorf("%w: no qualified Gen2 grind site at this story boundary", errGSControllerUnavailable)
	}
}

func executeGSCombatRecoveryTrain(m *emu.Emu, romData []byte, o Objective) error {
	if m == nil {
		return fmt.Errorf("gen2 training: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	obs, err := ObserveChecked(m, romData)
	if err != nil {
		return err
	}
	if o.Kind != KindTrain || o.Slot != 0 || o.Species != "" || o.Level > gsCombatRecoveryMaxLevel {
		return fmt.Errorf("%w: Gen2 combat recovery only supports the lead, to level %d or below", errGSControllerUnavailable, gsCombatRecoveryMaxLevel)
	}
	if len(obs.Party) == 0 || obs.Party[0].IsEgg {
		return fmt.Errorf("%w: Gen2 combat recovery requires a non-egg lead", errGSSecondBadgeUnexpectedState)
	}
	if obs.Party[0].Level >= o.Level {
		return nil
	}
	mapName, x, y, err := gsRecoveryGrindSite(obs)
	if err != nil {
		return err
	}
	return gsEnsureLeadLevel(m, romData, profile, "combat recovery", int(o.Level), mapName, x, y)
}

func executeGSCombatRecoveryHeal(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("gen2 heal: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	return gsEnsurePartyRecovered(m, romData, profile)
}

// A Gold/Silver egg is not healable. The generic heal verifier deliberately
// treats MaxHP==0 as failure (valid for Gen I), but Gen II's ordinary egg slot
// must be skipped while every actual battler is checked for full HP/status.
func verifyGSCombatRecoveryHeal(obs Observation) error {
	if !stableObjectiveBoundary(obs) {
		return fmt.Errorf("%w: Gen2 heal returned outside stable overworld", ErrObjectivePostconditionUnavailable)
	}
	members := 0
	for i, mon := range obs.Party {
		if mon.IsEgg {
			continue
		}
		members++
		if mon.MaxHP == 0 || mon.HP != mon.MaxHP || mon.Status != "" {
			return fmt.Errorf("%w: Gen2 heal left party slot %d at %d/%d HP status=%q",
				ErrObjectivePostconditionFailed, i, mon.HP, mon.MaxHP, mon.Status)
		}
	}
	if members == 0 {
		return fmt.Errorf("%w: Gen2 heal has no hatchable party members", ErrObjectivePostconditionFailed)
	}
	return nil
}
