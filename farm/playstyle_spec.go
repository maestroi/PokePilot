package farm

import (
	"encoding/json"
	"strings"
	"sync"
)

// Optional run-policy fields extend the long-lived Spec wire without changing
// the legacy struct layout used throughout the wall/runner code. The JSON
// methods below round-trip them keyed by run_id; old payloads simply have no
// entry and retain their compatibility defaults in the agent.
//
// This is intentionally wire policy, not planner policy: farm cannot import
// agent. The runner normalizes values before constructing planner behavior.
var (
	playStyleByRun      sync.Map // map[string]string
	riskToleranceByRun  sync.Map // map[string]string
	wildEncountersByRun sync.Map // map[string]string
	goalProvidedByRun   sync.Map // map[string]bool; keeps explicit goal:"" distinct from omission
)

var currentRunPolicy struct {
	sync.RWMutex
	playStyle      string
	riskTolerance  string
	wildEncounters string
}

func rememberRunPolicyValue(store *sync.Map, runID, value string) {
	runID = strings.TrimSpace(runID)
	value = strings.ToLower(strings.TrimSpace(value))
	if runID == "" {
		return
	}
	if value == "" {
		store.Delete(runID)
		return
	}
	store.Store(runID, value)
}

func runPolicyValue(store *sync.Map, runID string) string {
	if v, ok := store.Load(strings.TrimSpace(runID)); ok {
		return v.(string)
	}
	return ""
}

func rememberRunGoalProvided(runID string, provided bool) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return
	}
	if !provided {
		goalProvidedByRun.Delete(runID)
		return
	}
	goalProvidedByRun.Store(runID, true)
}

func runGoalProvided(runID string) bool {
	_, ok := goalProvidedByRun.Load(strings.TrimSpace(runID))
	return ok
}

// RememberPlayStyle records the wire value for a run. Empty style removes the
// extension, preserving the historical no-field encoding for legacy specs.
func RememberPlayStyle(runID, style string) {
	rememberRunPolicyValue(&playStyleByRun, runID, style)
}

func RememberRiskTolerance(runID, risk string) {
	rememberRunPolicyValue(&riskToleranceByRun, runID, risk)
}

func RememberWildEncounters(runID, policy string) {
	rememberRunPolicyValue(&wildEncountersByRun, runID, policy)
}

// PlayStyleForRun returns the selected wire profile, or empty for a legacy
// spec. Empty deliberately means "use the agent's backwards-compatible
// default", not Adventure.
func PlayStyleForRun(runID string) string { return runPolicyValue(&playStyleByRun, runID) }

func RiskToleranceForRun(runID string) string { return runPolicyValue(&riskToleranceByRun, runID) }

func WildEncountersForRun(runID string) string { return runPolicyValue(&wildEncountersByRun, runID) }

func PlayStyleForSpec(s Spec) string { return PlayStyleForRun(s.RunID) }

func RiskToleranceForSpec(s Spec) string { return RiskToleranceForRun(s.RunID) }

func WildEncountersForSpec(s Spec) string { return WildEncountersForRun(s.RunID) }

// Current* values come from the most recently decoded lease spec in this
// process. A pokepilot worker leases and runs one spec at a time, so the
// existing planner construction path can consume them without growing every
// historical function signature. Decoding a legacy spec explicitly resets
// the fields to their empty compatibility defaults.
func CurrentPlayStyle() string {
	currentRunPolicy.RLock()
	defer currentRunPolicy.RUnlock()
	return currentRunPolicy.playStyle
}

func CurrentRiskTolerance() string {
	currentRunPolicy.RLock()
	defer currentRunPolicy.RUnlock()
	return currentRunPolicy.riskTolerance
}

func CurrentWildEncounters() string {
	currentRunPolicy.RLock()
	defer currentRunPolicy.RUnlock()
	return currentRunPolicy.wildEncounters
}

func setCurrentRunPolicy(playStyle, riskTolerance, wildEncounters string) {
	currentRunPolicy.Lock()
	currentRunPolicy.playStyle = strings.ToLower(strings.TrimSpace(playStyle))
	currentRunPolicy.riskTolerance = strings.ToLower(strings.TrimSpace(riskTolerance))
	currentRunPolicy.wildEncounters = strings.ToLower(strings.TrimSpace(wildEncounters))
	currentRunPolicy.Unlock()
}

// CopyPlayStyle is retained for source compatibility with the play-style
// feature. New callers that create a logical child should use CopyRunPolicy.
func CopyPlayStyle(fromRunID, toRunID string) {
	if style := PlayStyleForRun(fromRunID); style != "" {
		RememberPlayStyle(toRunID, style)
	}
}

// CopyRunPolicy inherits only fields the child did not explicitly set. This
// makes resumed/endless children retain the parent's gameplay behavior while
// still allowing a caller to override one orthogonal knob.
func CopyRunPolicy(fromRunID, toRunID string) {
	if PlayStyleForRun(toRunID) == "" {
		CopyPlayStyle(fromRunID, toRunID)
	}
	if RiskToleranceForRun(toRunID) == "" {
		if risk := RiskToleranceForRun(fromRunID); risk != "" {
			RememberRiskTolerance(toRunID, risk)
		}
	}
	if WildEncountersForRun(toRunID) == "" {
		if policy := WildEncountersForRun(fromRunID); policy != "" {
			RememberWildEncounters(toRunID, policy)
		}
	}
	if !runGoalProvided(toRunID) && runGoalProvided(fromRunID) {
		rememberRunGoalProvided(toRunID, true)
	}
}

// MarshalJSON adds optional run-policy fields to Spec without forcing every
// existing Spec literal in the repository to grow fields immediately. An
// explicitly supplied empty goal is re-inserted after normal omitempty
// encoding so Free play survives wall -> runner leases.
func (s Spec) MarshalJSON() ([]byte, error) {
	type plain Spec
	data, err := json.Marshal(struct {
		plain
		PlayStyle      string `json:"play_style,omitempty"`
		RiskTolerance  string `json:"risk_tolerance,omitempty"`
		WildEncounters string `json:"wild_encounters,omitempty"`
	}{
		plain:          plain(s),
		PlayStyle:      PlayStyleForRun(s.RunID),
		RiskTolerance:  RiskToleranceForRun(s.RunID),
		WildEncounters: WildEncountersForRun(s.RunID),
	})
	if err != nil || s.Goal != "" || !runGoalProvided(s.RunID) {
		return data, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	fields["goal"] = json.RawMessage(`""`)
	return json.Marshal(fields)
}

// UnmarshalJSON accepts both old specs and the extended run-policy wire and
// remembers extensions by run_id for subsequent lease/recording use.
func (s *Spec) UnmarshalJSON(data []byte) error {
	type plain Spec
	var in struct {
		plain
		PlayStyle      string `json:"play_style,omitempty"`
		RiskTolerance  string `json:"risk_tolerance,omitempty"`
		WildEncounters string `json:"wild_encounters,omitempty"`
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	_, goalProvided := fields["goal"]

	*s = Spec(in.plain)
	RememberPlayStyle(s.RunID, in.PlayStyle)
	RememberRiskTolerance(s.RunID, in.RiskTolerance)
	RememberWildEncounters(s.RunID, in.WildEncounters)
	rememberRunGoalProvided(s.RunID, goalProvided)
	setCurrentRunPolicy(in.PlayStyle, in.RiskTolerance, in.WildEncounters)
	if !goalProvided {
		ApplyPlayStyleDefaultGoal(s)
	}
	return nil
}
