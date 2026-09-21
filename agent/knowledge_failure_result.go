package agent

import (
	"strings"
)

// FailedResult records planner-facing failure knowledge from the normalized
// objective result. nativeErr is retained only for the bounded human-readable
// diagnostic text; failure identity and recovery gates use semantic evidence.
func (k *Knowledge) FailedResult(result ObjectiveResult, nativeErr error) {
	if k == nil || nativeErr == nil {
		return
	}
	o := result.Objective
	storage := objectiveStorageKey(o)
	if result.Battle != nil && !result.Battle.Won && o.Kind == KindGym && o.Place != "" {
		storage = gymLossFailureKey(o.Place)
	}
	if failureCauseIs(result, "trainer_blacked_out") {
		storage = trainerLossFailureKey(o)
	}
	f := k.Failures[storage]
	k.bumpFailureTimes(&f)
	f.Objective, f.Last = o.String(), conciseObjectiveError(o, nativeErr)
	k.Failures[storage] = f
}

// noteMachineUnusable records that this objective left the backing machine
// unsafe to keep using. The mode survives a checkpoint resume so a later
// process can refuse the same class of action without parsing the error text.
// The record is stamped with the current build. A newer build does not keep
// the gate, so a fix for the stall can try the action again.
func (k *Knowledge) noteMachineUnusable(o Objective, err error) {
	if k == nil || err == nil {
		return
	}
	storage := failureStorageKey(o.Key(), failureModeMachineUnusable)
	f := k.Failures[storage]
	k.bumpFailureTimes(&f)
	f.Objective, f.Last = o.String(), conciseObjectiveError(o, err)
	k.Failures[storage] = f
}

func virtualTradeMachineUnusable(k *Knowledge) bool {
	if k == nil {
		return false
	}
	prefix := failureModeMachineUnusable + ":"
	build := strings.TrimSpace(k.Build)
	for storage, failure := range k.Failures {
		if !strings.HasPrefix(storage, prefix) {
			continue
		}
		if build != "" && failure.Build != build {
			continue
		}
		return true
	}
	return false
}

// notePartyCombatResult releases combat-loss gates only from semantic party
// progress or normalized training outcomes. Generic run policy never needs to
// inspect a concrete skill/controller error identity.
func (k *Knowledge) notePartyCombatResult(before, after Observation, result ObjectiveResult) {
	if k == nil {
		return
	}
	if partyCombatAdvanced(before, after) ||
		failureCauseIs(result, "train_progress_shortfall") ||
		failureCauseIs(result, "training_inefficient_area") {
		k.releaseCombatLossGates()
	}
}
