package agent

// FailedResult records planner-facing failure knowledge from the normalized
// objective result. nativeErr is retained only for the bounded human-readable
// diagnostic text; failure identity and recovery gates use semantic evidence.
func (k *Knowledge) FailedResult(result ObjectiveResult, nativeErr error) {
	if k == nil || nativeErr == nil {
		return
	}
	o := result.Objective
	name := o.String()
	if result.Battle != nil && !result.Battle.Won && o.Kind == KindGym && o.Place != "" {
		name = gymLossFailureKey(o.Place)
	}
	if failureCauseIs(result, "trainer_blacked_out") {
		name = trainerLossFailureKey(o)
	}
	f := k.Failures[name]
	f.Objective, f.Times, f.Last = name, f.Times+1, conciseObjectiveError(o, nativeErr)
	k.Failures[name] = f
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
