package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

type failureQuarantineEntry struct {
	Cause    FailureCauseID
	StateKey string
}

type failureQuarantine map[string]failureQuarantineEntry

func newFailureQuarantine() failureQuarantine { return failureQuarantine{} }

// recoveryStateKey hashes only FailureState's semantic, planner-relevant
// fields. Raw RAM/map encodings and diagnostic prose never decide retry policy.
func recoveryStateKey(obs Observation) string {
	data, _ := json.Marshal(FailureStateFor(obs))
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:8])
}

func recoverableFailureKey(obj Objective, result ObjectiveResult) string {
	cause := result.Cause
	if cause == "" {
		cause = FailureCauseID("outcome:" + string(result.Outcome))
	}
	return obj.String() + "|" + string(cause) + "|" + recoveryStateKey(result.Final)
}

func (q failureQuarantine) record(result ObjectiveResult) {
	if q == nil || actionFor(result.Outcome) != actionReplan {
		return
	}
	q[result.Objective.String()] = failureQuarantineEntry{
		Cause: result.Cause, StateKey: recoveryStateKey(result.Final),
	}
}

// filter suppresses an exact failed objective while the relevant semantic
// state is unchanged and alternatives exist. A material state change expires
// the quarantine automatically. If every option is quarantined it fails open;
// the repeated-failure budget then provides the hard loop ceiling.
func (q failureQuarantine) filter(obs Observation, offered []Objective) []Objective {
	if q == nil || len(q) == 0 || len(offered) <= 1 {
		return offered
	}
	stateKey := recoveryStateKey(obs)
	out := make([]Objective, 0, len(offered))
	for _, o := range offered {
		entry, ok := q[o.String()]
		if !ok {
			out = append(out, o)
			continue
		}
		if entry.StateKey != stateKey {
			delete(q, o.String())
			out = append(out, o)
			continue
		}
	}
	if len(out) == 0 {
		return offered
	}
	return out
}

func (q failureQuarantine) clear(o Objective) {
	if q != nil {
		delete(q, o.String())
	}
}

func markLastOutcomeRecovered(res *Result) {
	if res == nil || len(res.Outcomes) == 0 {
		return
	}
	i := len(res.Outcomes) - 1
	res.Outcomes[i].Recovered = true
	res.Outcomes[i].Terminal = false
}

func markLastOutcomeTerminal(res *Result) {
	if res == nil || len(res.Outcomes) == 0 {
		return
	}
	i := len(res.Outcomes) - 1
	res.Outcomes[i].Recovered = false
	res.Outcomes[i].Terminal = true
}
