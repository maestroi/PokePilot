package agent

import "strings"

// gymLossFailurePrefix scopes a leader loss to the gym where it happened.
// Ordinary objective failures use Objective.String() as their stable key,
// but KindGym deliberately renders "here" because the same verb is reused in
// every gym. Without a scoped key, losing to Brock would either block every
// later gym or be indistinguishable from a loss to Misty after History scrolls.
const gymLossFailurePrefix = "beat the gym leader at "

// gymRetryReadyKey is deliberately the ordinary KindGym objective identity.
// Knowledge.Done deletes an objective's ordinary failure key when it succeeds,
// so a successful retry automatically consumes this marker without another
// persistence path. A fresh leader loss still gets the scoped key below; while
// that scoped loss exists gymRetryPending is false and another Train rung is
// allowed.
func gymRetryReadyKey() string { return Objective{Kind: KindGym}.String() }

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

// clearGymLossFailures marks a lost gym as READY TO RETRY after successful
// training instead of simply forgetting the loss. The old behavior deleted
// the only evidence that linked training to a rechallenge, so the planner was
// free to pick Train again and again (observed live: zero badges, Ivysaur L22,
// 141 repeated decisions). The recovery cycle is now bounded:
//
//   leader loss -> Train one rung -> retry due -> leader attempt
//
// If the retry loses, Knowledge.Failed writes the scoped loss key again;
// gymRetryPending then becomes false and one more Train rung is allowed. If
// the retry wins, Knowledge.Done deletes gymRetryReadyKey automatically.
func (k *Knowledge) clearGymLossFailures() {
	var retry Failure
	for name, f := range k.Failures {
		if !strings.HasPrefix(name, gymLossFailurePrefix) {
			continue
		}
		place := strings.TrimPrefix(name, gymLossFailurePrefix)
		retry = Failure{
			Objective: gymRetryReadyKey(),
			Times:     f.Times,
			Last:      "party trained after losing at " + place + "; retry is due",
		}
		delete(k.Failures, name)
	}
	if retry.Objective != "" {
		k.Failures[retry.Objective] = retry
	}
}

// gymRetryPending reports the recovery state created by clearGymLossFailures.
// A new scoped leader loss always takes precedence over the older ready marker:
// that means the retry happened and lost, so training must become available
// again before another attempt.
func gymRetryPending(k *Knowledge) (place string, ok bool) {
	for name := range k.Failures {
		if strings.HasPrefix(name, gymLossFailurePrefix) {
			return "", false
		}
	}
	f, ok := k.Failures[gymRetryReadyKey()]
	if !ok {
		return "", false
	}
	const prefix = "party trained after losing at "
	const suffix = "; retry is due"
	if strings.HasPrefix(f.Last, prefix) && strings.HasSuffix(f.Last, suffix) {
		place = strings.TrimSuffix(strings.TrimPrefix(f.Last, prefix), suffix)
		place = strings.ToLower(place)
	}
	return place, true
}

func appendObjectiveNote(o Objective, note string) Objective {
	if note == "" {
		return o
	}
	if o.Note == "" {
		o.Note = note
	} else {
		o.Note += " " + note
	}
	return o
}
