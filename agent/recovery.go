package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
)

// recoverableFailureFingerprint is the one semantic identity used by retry,
// strategic escalation, repeat detection and same-state quarantine. The key is
// derived only from canonical objective identity, normalized failure facts and
// planner-relevant world state; native error prose never participates.
type recoverableFailureFingerprint struct {
	Key          string
	ObjectiveKey string
	StateKey     string
}

type failureQuarantineEntry struct {
	Fingerprint string
	StateKey    string
}

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

func fingerprintRecoverableFailure(obj Objective, result ObjectiveResult) recoverableFailureFingerprint {
	objectiveKey := objectiveStorageKey(obj)
	stateKey := recoveryStateKey(result.Final)
	return recoverableFailureFingerprint{
		ObjectiveKey: objectiveKey,
		StateKey:     stateKey,
		Key:          objectiveKey + "|" + normalizedFailureKey(result) + "|" + stateKey,
	}
}

// recoverableFailureKey remains the compact compatibility helper used by
// diagnostics/tests; all live recovery decisions consume the fingerprint above.
func recoverableFailureKey(obj Objective, result ObjectiveResult) string {
	return fingerprintRecoverableFailure(obj, result).Key
}

// record quarantines the exact objective/failure/world fingerprint after a
// recoverable failure. The entry lives inside runFailurePolicy so quarantine,
// repeat detection and retry budgets cannot drift into separate state machines.
func (f *runFailurePolicy) record(result ObjectiveResult) {
	if f == nil || actionFor(result.Outcome) != actionReplan {
		return
	}
	fingerprint := fingerprintRecoverableFailure(result.Objective, result)
	f.quarantine[fingerprint.ObjectiveKey] = failureQuarantineEntry{
		Fingerprint: fingerprint.Key,
		StateKey:    fingerprint.StateKey,
	}
}

// filter suppresses an exact failed objective while planner-relevant state is
// unchanged and alternatives exist. A material state change naturally expires
// the entry because it changes StateKey; no string/error heuristic is needed.
// If every option is quarantined it fails open, leaving the bounded retry policy
// as the final loop ceiling.
func (f *runFailurePolicy) filter(obs Observation, offered []Objective) []Objective {
	if f == nil || len(f.quarantine) == 0 || len(offered) <= 1 {
		return offered
	}
	stateKey := recoveryStateKey(obs)
	out := make([]Objective, 0, len(offered))
	for _, o := range offered {
		key := objectiveStorageKey(o)
		entry, ok := f.quarantine[key]
		if !ok {
			out = append(out, o)
			continue
		}
		if entry.StateKey != stateKey {
			delete(f.quarantine, key)
			out = append(out, o)
			continue
		}
	}
	if len(out) == 0 {
		return offered
	}
	return out
}

func (f *runFailurePolicy) clear(o Objective) {
	if f != nil {
		delete(f.quarantine, objectiveStorageKey(o))
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
