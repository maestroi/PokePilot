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

type recoveryStateScope uint8

const (
	recoveryStateScopeObjective recoveryStateScope = iota
	recoveryStateScopeRoutePrerequisite
	recoveryStateScopeCombatLoss
	recoveryStateScopeFieldRoster
)

type failureQuarantineEntry struct {
	Fingerprint string
	StateKey    string
	StateScope  recoveryStateScope
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
	case KindRepairFieldCapability:
		includeCombat()
		includeInventory()
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

// routePrerequisiteStateKey deliberately excludes position and local boundary
// state. A missing portable route capability is not repaired by walking to a
// different tile or completing an unrelated local objective; retry becomes
// meaningful only after badges, field capabilities, or story progress change.
//
// This scope is intentionally narrower than the ordinary KindGoTo recovery
// state. Geometry/navigation failures still include Location/X/Y, because a
// different component or approach can make those succeed.
func routePrerequisiteStateKey(obs Observation) string {
	full := FailureStateFor(obs)
	data, _ := json.Marshal(struct {
		Badges       []string              `json:"badges,omitempty"`
		Capabilities []FailureCapability   `json:"capabilities,omitempty"`
		Progress     []FailureProgressFact `json:"progress,omitempty"`
	}{
		Badges:       full.Badges,
		Capabilities: full.Capabilities,
		Progress:     full.Progress,
	})
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:8])
}

// combatLossStateKey treats every battle defeat as a combat-state failure.
// Ordinary GoTo/progression failures intentionally ignore party drift, but a
// blackout can become retryable after training, evolution, PP recovery, or
// other material party progress even when the destination and route state are
// unchanged.
func combatLossStateKey(obs Observation) string {
	full := FailureStateFor(obs)
	data, _ := json.Marshal(struct {
		Party        []FailurePartyMember  `json:"party,omitempty"`
		LeadPP       []uint8               `json:"lead_pp,omitempty"`
		Badges       []string              `json:"badges,omitempty"`
		Capabilities []FailureCapability   `json:"capabilities,omitempty"`
		Progress     []FailureProgressFact `json:"progress,omitempty"`
	}{
		Party:        full.Party,
		LeadPP:       append([]uint8(nil), obs.LeadPP...),
		Badges:       full.Badges,
		Capabilities: full.Capabilities,
		Progress:     full.Progress,
	})
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:8])
}

// fieldRosterStateKey treats a field-roster capability gap (no party or box
// member can learn the required HM) as a roster-state failure, not a
// position failure. Walking to a different tile or map does not change which
// Pokémon can learn FLY; the quarantine must survive position drift until
// party composition, inventory, badges, capabilities, or story progress
// change.
func fieldRosterStateKey(obs Observation) string {
	full := FailureStateFor(obs)
	data, _ := json.Marshal(struct {
		Party        []FailurePartyMember   `json:"party,omitempty"`
		LeadPP       []uint8                `json:"lead_pp,omitempty"`
		Inventory    []FailureInventoryItem `json:"inventory,omitempty"`
		Badges       []string               `json:"badges,omitempty"`
		Capabilities []FailureCapability    `json:"capabilities,omitempty"`
		Progress     []FailureProgressFact  `json:"progress,omitempty"`
	}{
		Party:        full.Party,
		LeadPP:       append([]uint8(nil), obs.LeadPP...),
		Inventory:    full.Inventory,
		Badges:       full.Badges,
		Capabilities: full.Capabilities,
		Progress:     full.Progress,
	})
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:8])
}

func recoveryStateScopeFor(result ObjectiveResult) recoveryStateScope {
	if failureCauseIs(result, "route_prerequisite_missing") {
		// Capability-only quarantine is safe for direct travel objectives: moving
		// elsewhere does not satisfy a missing badge/HM/story gate, and the
		// plain/flee sibling would otherwise immediately retry the same route.
		//
		// Compound objectives are different. Progress, catch, gym, gift/trade
		// execution and similar transactions can own multiple internal journeys,
		// so a route prerequisite may describe only the approach that failed.
		// Their ordinary objective state includes position and lets a materially
		// different route reopen the transaction. #1109 exposed this for
		// progression; #1084 exposed the same permanent quarantine for a Route 2
		// catch after recovery had moved Red elsewhere.
		if _, direct := routePolicySibling(result.Objective); direct {
			return recoveryStateScopeRoutePrerequisite
		}
	}
	if failureCauseIs(result, "trainer_blacked_out") ||
		failureCauseIs(result, failureCauseCombatDefeat) {
		return recoveryStateScopeCombatLoss
	}
	if failureCauseIs(result, "field_roster_no_recovery") {
		return recoveryStateScopeFieldRoster
	}
	return recoveryStateScopeObjective
}

func recoveryStateKeyForScope(o Objective, obs Observation, scope recoveryStateScope) string {
	switch scope {
	case recoveryStateScopeRoutePrerequisite:
		return routePrerequisiteStateKey(obs)
	case recoveryStateScopeCombatLoss:
		return combatLossStateKey(obs)
	case recoveryStateScopeFieldRoster:
		return fieldRosterStateKey(obs)
	default:
		return recoveryStateKey(o, obs)
	}
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
	scope := recoveryStateScopeFor(result)
	stateKey := recoveryStateKeyForScope(obj, result.Final, scope)
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

func routePolicySibling(o Objective) (Objective, bool) {
	switch o.Kind {
	case KindGoTo:
	case KindHeal:
		if o.Place == "" {
			return Objective{}, false
		}
	default:
		return Objective{}, false
	}
	sibling := o
	sibling.Flee = !o.Flee
	return sibling, true
}

// record quarantines the exact objective/failure/world fingerprint after a
// recoverable failure. The entry lives inside runFailurePolicy so quarantine,
// repeat detection and retry budgets cannot drift into separate state machines.
func (f *runFailurePolicy) record(result ObjectiveResult) {
	if f == nil || actionFor(result.Outcome) != actionReplan {
		return
	}
	fingerprint := fingerprintRecoverableFailure(result.Objective, result)
	scope := recoveryStateScopeFor(result)
	f.pendingPrerequisites = nil
	failure := normalizedFailure(result)
	switch {
	case failureCauseIs(result, "route_prerequisite_missing"):
		seen := map[CapabilityID]bool{}
		for _, raw := range failure.Context {
			capability := CapabilityID(raw)
			if capability == "" || seen[capability] {
				continue
			}
			seen[capability] = true
			f.pendingPrerequisites = append(f.pendingPrerequisites, Prerequisite{Capability: capability})
		}
	case failureCauseIs(result, "progression_prerequisite_missing"):
		seen := map[ProgressID]bool{}
		for _, raw := range failure.Context {
			progress := ProgressID(raw)
			if progress == "" || seen[progress] {
				continue
			}
			seen[progress] = true
			f.pendingPrerequisites = append(f.pendingPrerequisites, Prerequisite{Progress: progress})
		}
	}
	f.quarantine[fingerprint.ObjectiveKey] = failureQuarantineEntry{
		Fingerprint: fingerprint.Key,
		StateKey:    fingerprint.StateKey,
		StateScope:  scope,
	}

	// A missing route prerequisite is a property of the destination and the
	// current semantic route state, not of position or how wild encounters are
	// handled while walking. Quarantine the plain/flee sibling together so
	// recovery cannot immediately retry the same impossible route under the
	// other travel policy, and retain both entries across unrelated movement.
	if failureCauseIs(result, "route_prerequisite_missing") {
		if sibling, ok := routePolicySibling(result.Objective); ok {
			siblingResult := result
			siblingResult.Objective = sibling
			siblingFingerprint := fingerprintRecoverableFailure(sibling, siblingResult)
			f.quarantine[siblingFingerprint.ObjectiveKey] = failureQuarantineEntry{
				Fingerprint: siblingFingerprint.Key,
				StateKey:    siblingFingerprint.StateKey,
				StateScope:  scope,
			}
		}
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
		if entry.StateKey != recoveryStateKeyForScope(o, obs, entry.StateScope) {
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
