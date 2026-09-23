package agent

import (
	"errors"
	"strings"

	"github.com/maestroi/pokepilot/skill"
)

func combatRecoveryObjective(o Objective) Objective {
	base := o
	base.Note = ""
	if base.Kind == KindGoTo || (base.Kind == KindHeal && base.Place != "") {
		// Flee changes only wild-encounter policy. A mandatory trainer cannot
		// be fled, so both journey variants share one combat recovery identity.
		base.Flee = false
	}
	return base
}

func combatLossFailureKey(o Objective) string {
	return failureStorageKey(combatRecoveryObjective(o).Key(), failureModeCombatLoss)
}

func combatRetryReadyKey(o Objective) string {
	return failureStorageKey(combatRecoveryObjective(o).Key(), failureModeCombatRetry)
}

// combatLossFailureName recognizes only typed combat evidence. It deliberately
// does not classify a plain ErrBlackedOut: wild losses and poison wipes are
// logistics outcomes, not proof that the objective is blocked by combat.
func combatLossFailureName(o Objective, err error) (string, bool) {
	if err == nil {
		return "", false
	}
	var required *skill.RequiredBattleError
	if errors.As(err, &required) && errors.Is(err, skill.ErrBlackedOut) {
		return combatLossFailureKey(o), true
	}
	if errors.Is(err, skill.ErrTrainerBlackedOut) {
		return combatLossFailureKey(o), true
	}
	return "", false
}

func combatLossRecorded(k *Knowledge, o Objective) bool {
	if k == nil {
		return false
	}
	base := combatRecoveryObjective(o)
	if _, ok := k.Failures[combatLossFailureKey(base)]; ok {
		return true
	}
	// Read-only migration support for structured pre-generic modes.
	if _, ok := k.Failures[legacyTrainerLossStorageKey(base)]; ok {
		return true
	}
	if _, ok := k.Failures[legacyTrainerLossStringKey(base)]; ok {
		return true
	}
	if base.Kind == KindGym && base.Place != "" {
		if _, ok := k.Failures[legacyGymLossStorageKey(string(base.Place))]; ok {
			return true
		}
		if _, ok := k.Failures[legacyGymLossStringKey(string(base.Place))]; ok {
			return true
		}
	}
	return false
}

func mergeCombatRetryFailure(a, b Failure) Failure {
	if b.Times > a.Times || a.Objective == "" {
		return b
	}
	return a
}

// promoteCombatLossesToRetry is the only live loss->retry state transition.
// It converts generic combat losses, and any readable historical trainer/gym
// modes, into generic combat_retry records keyed by ObjectiveKey.
func (k *Knowledge) promoteCombatLossesToRetry() {
	if k == nil {
		return
	}
	retries := map[ObjectiveKey]Failure{}
	for storage, f := range k.Failures {
		if key, mode, ok := parseFailureStorageKey(storage); ok {
			switch mode {
			case failureModeCombatLoss, legacyFailureModeTrainerLoss, legacyFailureModeGymLoss:
				key = combatRecoveryObjective(key.Objective()).Key()
				retries[key] = mergeCombatRetryFailure(retries[key], f)
				delete(k.Failures, storage)
			case legacyFailureModeGymRetry:
				// Legacy retry-ready state is migrated on sight.
				key = combatRecoveryObjective(key.Objective()).Key()
				retries[key] = mergeCombatRetryFailure(retries[key], f)
				delete(k.Failures, storage)
			}
			continue
		}
		if strings.HasPrefix(storage, legacyGymLossFailurePrefix) {
			place := strings.ToLower(strings.TrimPrefix(storage, legacyGymLossFailurePrefix))
			if place != "" {
				key := combatRecoveryObjective(Objective{Kind: KindGym, Place: PlaceID(place)}).Key()
				retries[key] = mergeCombatRetryFailure(retries[key], f)
			}
			delete(k.Failures, storage)
			continue
		}
		if strings.HasPrefix(storage, legacyTrainerLossFailurePrefix) {
			// v4 trainer-loss strings did not persist a structural key. Their
			// historical post-training behavior was simply to release the gate,
			// so preserve that behavior rather than parse display prose.
			delete(k.Failures, storage)
			continue
		}
		// v4 checkpoints used the generic gym display sentence as the retry key
		// and kept the place only in Last.
		if storage == (Objective{Kind: KindGym}).String() {
			const prefix = "party trained after losing at "
			const suffix = "; retry is due"
			if strings.HasPrefix(f.Last, prefix) && strings.HasSuffix(f.Last, suffix) {
				place := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(f.Last, prefix), suffix))
				if place != "" {
					key := combatRecoveryObjective(Objective{Kind: KindGym, Place: PlaceID(place)}).Key()
					retries[key] = mergeCombatRetryFailure(retries[key], f)
					delete(k.Failures, storage)
				}
			}
		}
	}
	for key, retry := range retries {
		o := key.Objective()
		retry.Objective = o.String()
		retry.Last = "combat readiness improved after defeat; retry is due"
		storage := combatRetryReadyKey(o)
		retry = mergeCombatRetryFailure(k.Failures[storage], retry)
		k.Failures[storage] = retry
	}
}

// combatRetryKeys returns retry-ready objectives while honoring legacy
// checkpoint modes. A fresh loss for the same normalized key always wins over
// an older ready marker.
func combatRetryKeys(k *Knowledge) map[ObjectiveKey]bool {
	ready := map[ObjectiveKey]bool{}
	lost := map[ObjectiveKey]bool{}
	if k == nil {
		return ready
	}
	for storage, f := range k.Failures {
		if key, mode, ok := parseFailureStorageKey(storage); ok {
			key = combatRecoveryObjective(key.Objective()).Key()
			switch mode {
			case failureModeCombatRetry, legacyFailureModeGymRetry:
				ready[key] = true
			case failureModeCombatLoss, legacyFailureModeTrainerLoss, legacyFailureModeGymLoss:
				lost[key] = true
			}
			continue
		}
		if strings.HasPrefix(storage, legacyGymLossFailurePrefix) {
			place := strings.ToLower(strings.TrimPrefix(storage, legacyGymLossFailurePrefix))
			if place != "" {
				lost[combatRecoveryObjective(Objective{Kind: KindGym, Place: PlaceID(place)}).Key()] = true
			}
			continue
		}
		if storage == (Objective{Kind: KindGym}).String() {
			const prefix = "party trained after losing at "
			const suffix = "; retry is due"
			if strings.HasPrefix(f.Last, prefix) && strings.HasSuffix(f.Last, suffix) {
				place := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(f.Last, prefix), suffix))
				if place != "" {
					ready[combatRecoveryObjective(Objective{Kind: KindGym, Place: PlaceID(place)}).Key()] = true
				}
			}
		}
	}
	for key := range lost {
		delete(ready, key)
	}
	return ready
}

// notePartyCombatChange is retained for direct legacy callers/tests; live Run
// uses notePartyCombatResult. Both release the same structured recovery gates.
func (k *Knowledge) notePartyCombatChange(before, after Observation, execErr error) {
	if k == nil {
		return
	}
	if partyCombatAdvanced(before, after) ||
		errors.Is(execErr, skill.ErrTrainProgress) ||
		errors.Is(execErr, ErrTrainingInefficient) {
		k.releaseCombatLossGates()
	}
}

func (k *Knowledge) releaseCombatLossGates() {
	k.promoteCombatLossesToRetry()
}

func partyCombatAdvanced(before, after Observation) bool {
	if after.PartyCount > before.PartyCount {
		return true
	}
	return partyMaxLevel(after) > partyMaxLevel(before)
}

func partyMaxLevel(obs Observation) uint8 {
	var max uint8
	for _, mon := range obs.Party {
		if mon.Level > max {
			max = mon.Level
		}
	}
	return max
}

func trainingUnviableHere(obs Observation) bool {
	return obs.Training != nil && obs.Training.Viability == TrainingOutsideBudget
}

// ppRecoveryDue reports whether Offer already proved that attacking PP needs
// recovery by constructing a recovery objective.
func ppRecoveryDue(out []Objective) bool {
	for _, o := range out {
		if o.Kind == KindHeal && strings.Contains(o.Note, "lead has no PP") {
			return true
		}
		if o.Kind != KindUseItem {
			continue
		}
		for _, id := range ppRestoreItems {
			if o.Item == id {
				return true
			}
		}
	}
	return false
}

func combatRetryMatchesObjective(ready map[ObjectiveKey]bool, o Objective) bool {
	if ready[combatRecoveryObjective(o).Key()] {
		return true
	}
	// A gym retry may require an ordinary journey back to the gym before the
	// challenge itself is locally offerable.
	if o.Kind == KindGoTo && o.Place != "" {
		return ready[combatRecoveryObjective(Objective{Kind: KindGym, Place: o.Place}).Key()]
	}
	return false
}

func filterCombatRecoveryBlocked(out []Objective, known *Knowledge) []Objective {
	retryKeys := combatRetryKeys(known)
	retryDue := len(retryKeys) > 0
	ppDue := ppRecoveryDue(out)
	filtered := make([]Objective, 0, len(out))
	for _, o := range out {
		if combatLossRecorded(known, o) {
			continue
		}
		if ppDue && (o.Kind == KindTrain || o.Kind == KindGym) {
			continue
		}
		if retryDue && o.Kind == KindTrain {
			continue
		}
		if combatRetryMatchesObjective(retryKeys, o) {
			o = appendObjectiveNote(o, "(retry due after combat-readiness progress; test the stronger party now)")
		}
		filtered = append(filtered, o)
	}
	return filtered
}
