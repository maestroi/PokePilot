package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
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

// normalizedFailureKey fingerprints transaction phase, portable class, stable
// cause id and structured context. Native error prose is deliberately absent.
func normalizedFailureKey(result ObjectiveResult) string {
	failure := normalizedFailure(result)
	context := append([]string(nil), failure.Context...)
	sort.Strings(context)
	data, _ := json.Marshal(struct {
		Phase   string   `json:"phase,omitempty"`
		Class   string   `json:"class,omitempty"`
		Cause   string   `json:"cause,omitempty"`
		Context []string `json:"context,omitempty"`
	}{
		Phase:   string(failure.Phase),
		Class:   string(failure.Class),
		Cause:   failure.Cause,
		Context: context,
	})
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:8])
}

func recoverableFailureKey(obj Objective, result ObjectiveResult) string {
	return objectiveStorageKey(obj) + "|" + normalizedFailureKey(result) + "|" + recoveryStateKey(result.Final)
}

func (q failureQuarantine) record(result ObjectiveResult) {
	if q == nil || actionFor(result.Outcome) != actionReplan {
		return
	}
	failure := normalizedFailure(result)
	q[objectiveStorageKey(result.Objective)] = failureQuarantineEntry{
		Cause: FailureCauseID(failure.Cause), StateKey: recoveryStateKey(result.Final),
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
		key := objectiveStorageKey(o)
		entry, ok := q[key]
		if !ok {
			out = append(out, o)
			continue
		}
		if entry.StateKey != stateKey {
			delete(q, key)
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
		delete(q, objectiveStorageKey(o))
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
