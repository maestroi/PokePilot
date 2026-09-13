package main

import (
	"encoding/json"
	"sync"
)

// spectatorPresentationPolicy is the tiny, explicitly public-safe slice of a
// wall dashboard row that is useful for presentation but is not part of the
// historical spectatorRun Go shape. Keeping it separate avoids accidentally
// widening the public trust boundary when private wall rows gain new fields.
type spectatorPresentationPolicy struct {
	FPS             int
	PlayStyle       string
	RiskTolerance   string
	WildEncounters  string
}

const spectatorPresentationPolicyLimit = 256

var spectatorPresentationPolicies = struct {
	sync.Mutex
	order  []string
	values map[string]spectatorPresentationPolicy
}{values: make(map[string]spectatorPresentationPolicy)}

func rememberSpectatorPresentationPolicy(runID string, policy spectatorPresentationPolicy) {
	if runID == "" {
		return
	}
	spectatorPresentationPolicies.Lock()
	defer spectatorPresentationPolicies.Unlock()
	if _, exists := spectatorPresentationPolicies.values[runID]; !exists {
		spectatorPresentationPolicies.order = append(spectatorPresentationPolicies.order, runID)
	}
	spectatorPresentationPolicies.values[runID] = policy
	for len(spectatorPresentationPolicies.order) > spectatorPresentationPolicyLimit {
		oldest := spectatorPresentationPolicies.order[0]
		spectatorPresentationPolicies.order = spectatorPresentationPolicies.order[1:]
		delete(spectatorPresentationPolicies.values, oldest)
	}
}

func spectatorPresentationPolicyForRun(runID string) spectatorPresentationPolicy {
	spectatorPresentationPolicies.Lock()
	defer spectatorPresentationPolicies.Unlock()
	return spectatorPresentationPolicies.values[runID]
}

// UnmarshalJSON deliberately reads the presentation fields in a second,
// allowlisted pass. spectatorSourceRun still owns the private fields needed for
// curation, while spectatorRun.MarshalJSON decides exactly what crosses the
// public boundary.
func (run *spectatorSourceRun) UnmarshalJSON(data []byte) error {
	type plain spectatorSourceRun
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var presentation struct {
		FPS            int    `json:"fps"`
		PlayStyle      string `json:"play_style,omitempty"`
		RiskTolerance  string `json:"risk_tolerance,omitempty"`
		WildEncounters string `json:"wild_encounters,omitempty"`
	}
	if err := json.Unmarshal(data, &presentation); err != nil {
		return err
	}
	*run = spectatorSourceRun(decoded)
	rememberSpectatorPresentationPolicy(run.RunID, spectatorPresentationPolicy{
		FPS:            presentation.FPS,
		PlayStyle:      presentation.PlayStyle,
		RiskTolerance:  presentation.RiskTolerance,
		WildEncounters: presentation.WildEncounters,
	})
	return nil
}
