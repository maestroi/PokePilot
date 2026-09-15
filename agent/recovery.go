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
// objective-relevant world state; native error prose never participates.
type recoverableFailureFingerprint struct {
	Key          string
	ObjectiveKey string
	StateKey     string
}

type failureQuarantineEntry struct {
	Fingerprint string
	StateKey    string
}

// recoveryState is the objective-scoped semantic projection used by retry
// identity. It deliberately avoids hashing every observable fact: an incidental
// money/HP change must not reopen a travel failure, while PP, party readiness,
// route capabilities and inventory are included for objectives that depend on
// them.
type recoveryState struct {
	Location     PlaceID                `json:"location,omitempty"`
	X            uint8                  `json:"x,omitempty"`
	Y            uint8                  `json:"y,omitempty"`
	Controllable bool                   `json:"controllable,omitempty"`
	InBattle     bool                   `json:"in_battle,omitempty"`
	Money        uint32                 `json:"money,omitempty"`
	Party        []FailurePartyMember   `json:"party,omitempty"`
	LeadPP       []uint8                `json:"lead_pp,omitempty"`
	Inventory    []FailureInventoryItem `json:"inventory,omitempty"`
	Badges       []string               `json:"badges,omitempty"`
	Capabilities []FailureCapability    `json:"capabilities,omitempty"`
	Progress     []FailureProgressFact  `json:"progress,omitempty"`
}

func recoveryStateFor(o Objective, obs Observation) recoveryState {
	full := FailureStateFor(obs)
	out := recoveryState{
		Location:     full.Location,
		X:            full.X,
		Y:            full.Y,
		Controllable: full.Controllable,
		InBattle:     full.InBattle,
	}

	includeRoute := func() {
		out.Badges = append([]string(nil), full.Badges...)
		out.Capabilities = append([]FailureCapability(nil), full.Capabilities...)
		out.Progress = append([]FailureProgressFact(nil), full.Progress...)
	}
	includeCombat := func() {
		out.Party = append([]FailurePartyMember(nil), full.Party...)
		out.LeadPP = append([]uint8(nil), obs.LeadPP...)
	}
	includeInventory := func() {
		out.Inventory = append([]FailureInventoryItem(nil), full.Inventory...)
	}

	switch o.Kind {
	case KindGoTo, KindProgress:
		includeRoute()
	case KindTalk:
		// Position/boundary state is sufficient for a local interaction retry.
	case KindTrainer:
		includeCombat()
		includeRoute()
	case KindStarter:
		includeCombat()
	case KindTrain:
		includeCombat()
	case KindHeal:
		includeCombat()
		if o.Place != "" {
			includeRoute()
		}
	case KindGym:
		includeCombat()
		includeRoute()
	case KindCatch:
		includeCombat()
		includeInventory()
		includeRoute()
	case KindBuy:
		out.Money = full.Money
		includeInventory()
	case KindPickup:
		includeInventory()
	case KindUseItem:
		includeCombat()
		includeInventory()
	default:
		// Future objective kinds fail safe by using the complete portable state
		// rather than accidentally ignoring a prerequisite they may depend on.
		out.Money = full.Money
		out.Party = append([]FailurePartyMember(nil), full.Party...)
		out.LeadPP = append([]uint8(nil), obs.LeadPP...)
		out.Inventory = append([]FailureInventoryItem(nil), full.Inventory...)
		includeRoute()
	}
	return out
}

func recoveryStateKey(o Objective, obs Observation) string {
	data, _ := json.Marshal(recoveryStateFor(o, obs))
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
	stateKey := recoveryStateKey(obj, result.Final)
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

// filter suppresses an exact failed objective while the state relevant to that
// objective is unchanged and alternatives exist. A material scoped state change
// expires the entry; unrelated drift does not. If every option is quarantined
// it fails open, leaving the bounded retry policy as the final loop ceiling.
func (f *runFailurePolicy) filter(obs Observation, offered []Objective) []Objective {
	if f == nil || len(f.quarantine) == 0 || len(offered) <= 1 {
		return offered
	}
	out := make([]Objective, 0, len(offered))
	for _, o := range offered {
		key := objectiveStorageKey(o)
		entry, ok := f.quarantine[key]
		if !ok {
			out = append(out, o)
			continue
		}
		if entry.StateKey != recoveryStateKey(o, obs) {
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
