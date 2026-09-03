package agent

import "strings"

// gymLossFailurePrefix scopes a leader loss to the gym where it happened.
// Ordinary objective failures use Objective.String() as their stable key,
// but KindGym deliberately renders "here" because the same verb is reused in
// every gym. Without a scoped key, losing to Brock would either block every
// later gym or be indistinguishable from a loss to Misty after History scrolls.
const gymLossFailurePrefix = "beat the gym leader at "

// gymLossFailureKey is both the persisted failure identity and the factual
// text the planner sees in Observation.Failures. The place comes from the
// deterministic GymAt table when Offer constructs the objective; it is not a
// model-supplied argument.
func gymLossFailureKey(place string) string {
	if place == "" {
		return ""
	}
	return gymLossFailurePrefix + strings.ToUpper(place)
}

// gymLossFailureName classifies the one KindGym failure that should gate an
// immediate rechallenge: the leader battle was reached and lost. Navigation,
// menu, and control errors inside the gym remain ordinary failures and do not
// pretend that training would fix them.
//
// gymOutcomeErr owns this exact factual phrase. Keeping the classifier here
// avoids changing the public error contract merely to add planner bookkeeping;
// a future typed gym-loss error can replace this helper without changing the
// persisted failure key or Offer semantics.
func gymLossFailureName(o Objective, err error) (string, bool) {
	if o.Kind != KindGym || o.Place == "" || err == nil {
		return "", false
	}
	if !strings.Contains(err.Error(), "lost to the gym leader") {
		return "", false
	}
	return gymLossFailureKey(o.Place), true
}

// clearGymLossFailures marks the party as materially changed after successful
// training. One completed Train rung is enough to make previously lost gyms
// eligible for another attempt; successful travel/healing does not clear the
// evidence, which is what prevents loss -> long walk back -> loss loops.
func (k *Knowledge) clearGymLossFailures() {
	for name := range k.Failures {
		if strings.HasPrefix(name, gymLossFailurePrefix) {
			delete(k.Failures, name)
		}
	}
}
