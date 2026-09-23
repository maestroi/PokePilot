package agent

import (
	"errors"
	"strings"

	"github.com/maestroi/pokepilot/skill"
)

// legacyTrainerLossFailurePrefix is retained only for v4/string-keyed
// checkpoint migration. New state uses failureModeTrainerLoss + ObjectiveKey.
const legacyTrainerLossFailurePrefix = "trainer loss while attempting "

func trainerLossObjective(o Objective) Objective {
	base := o
	base.Note = ""
	if base.Kind == KindGoTo || (base.Kind == KindHeal && base.Place != "") {
		base.Flee = false
	}
	return base
}

// trainerLossFailureKey gives a trainer loss a stable logical objective
// identity. Travel's fight/flee variants intentionally collapse to the same
// key: fleeing changes only wild encounters, while a trainer cannot be fled.
func trainerLossFailureKey(o Objective) string {
	return failureStorageKey(trainerLossObjective(o).Key(), failureModeTrainerLoss)
}

func legacyTrainerLossFailureKey(o Objective) string {
	base := trainerLossObjective(o)
	if base.Kind == KindGym && base.Place != "" {
		return legacyTrainerLossFailurePrefix + "beat the gym leader at " + strings.ToUpper(base.Place)
	}
	return legacyTrainerLossFailurePrefix + base.String()
}

func trainerLossFailureName(o Objective, err error) (string, bool) {
	if err == nil || !errors.Is(err, skill.ErrTrainerBlackedOut) {
		return "", false
	}
	return trainerLossFailureKey(o), true
}

func trainerLossRecorded(k *Knowledge, o Objective) bool {
	if k == nil {
		return false
	}
	if _, ok := k.Failures[trainerLossFailureKey(o)]; ok {
		return true
	}
	_, ok := k.Failures[legacyTrainerLossFailureKey(o)]
	return ok
}

func (k *Knowledge) clearTrainerLossFailures() {
	if k == nil {
		return
	}
	for name := range k.Failures {
		if _, mode, ok := parseFailureStorageKey(name); ok && mode == failureModeTrainerLoss {
			delete(k.Failures, name)
			continue
		}
		if strings.HasPrefix(name, legacyTrainerLossFailurePrefix) {
			delete(k.Failures, name)
		}
	}
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
	k.clearGymLossFailures()
	k.clearTrainerLossFailures()
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

func filterTrainerLossBlocked(out []Objective, known *Knowledge) []Objective {
	retryPlaces := gymRetryPlaces(known)
	retryDue := len(retryPlaces) > 0
	ppDue := ppRecoveryDue(out)
	filtered := make([]Objective, 0, len(out))
	for _, o := range out {
		if trainerLossRecorded(known, o) {
			continue
		}
		if o.Kind == KindGym && o.Place != "" && gymLossRecorded(known, o.Place) {
			continue
		}
		if ppDue && (o.Kind == KindTrain || o.Kind == KindGym) {
			continue
		}
		if retryDue && o.Kind == KindTrain {
			continue
		}
		if retryDue {
			place := strings.ToLower(string(o.Place))
			switch {
			case o.Kind == KindGym && retryPlaces[place]:
				o = appendObjectiveNote(o, "(retry due after successful training; test the stronger party now)")
			case o.Kind == KindGoTo && retryPlaces[place]:
				o = appendObjectiveNote(o, "(return for gym retry after successful training)")
			}
		}
		filtered = append(filtered, o)
	}
	return filtered
}
