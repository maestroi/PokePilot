package agent

import (
	"fmt"
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
	f := k.Failures[storage]
	combatLoss := (result.Battle != nil && result.Battle.Result == "lost") ||
		failureCauseIs(result, failureCauseCombatDefeat) ||
		failureCauseIs(result, "trainer_blacked_out")
	if combatLoss {
		// All new combat recovery state is generic. Carry the retry record
		// forward before deleting it so repeated defeats escalate instead of
		// starting over at "first loss" after every preparation cycle.
		storage = combatLossFailureKey(o)
		f = mergeCombatRetryFailure(k.Failures[storage], k.Failures[combatRetryReadyKey(o)])
		delete(k.Failures, combatRetryReadyKey(o))
	}
	k.bumpFailureTimes(&f)
	f.Objective, f.Last = o.String(), conciseObjectiveError(o, nativeErr)
	if combatLoss {
		stampCombatPreparation(&f, result.Final)
		if f.ReadinessTarget > 0 {
			f.Last += fmt.Sprintf("; combat preparation readiness %d -> %d before retry",
				f.ReadinessBaseline, f.ReadinessTarget)
		}
	}
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
	progress := partyCombatAdvanced(before, after) ||
		failureCauseIs(result, "train_progress_shortfall") ||
		failureCauseIs(result, "training_inefficient_area")
	k.releaseSatisfiedCombatLossGates(after, progress)
}
