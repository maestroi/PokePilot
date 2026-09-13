package main

import (
	"encoding/json"

	"github.com/maestroi/pokepilot/farm"
)

type wallRunPolicy struct {
	PlayStyle      string
	RiskTolerance  string
	WildEncounters string
}

func runPolicyForWallRun(runID, resumeFrom string) wallRunPolicy {
	if resumeFrom != "" {
		farm.CopyRunPolicy(resumeFrom, runID)
	}
	return wallRunPolicy{
		PlayStyle:      farm.PlayStyleForRun(runID),
		RiskTolerance:  farm.RiskToleranceForRun(runID),
		WildEncounters: farm.WildEncountersForRun(runID),
	}
}

func playStyleForWallRun(runID, resumeFrom string) string {
	return runPolicyForWallRun(runID, resumeFrom).PlayStyle
}

// MarshalJSON adds optional gameplay policy to dashboard rows while keeping
// tileRow's historical Go shape source-compatible.
func (r tileRow) MarshalJSON() ([]byte, error) {
	type plain tileRow
	policy := runPolicyForWallRun(r.RunID, r.ResumeFromRunID)
	return json.Marshal(struct {
		plain
		PlayStyle      string `json:"play_style,omitempty"`
		RiskTolerance  string `json:"risk_tolerance,omitempty"`
		WildEncounters string `json:"wild_encounters,omitempty"`
	}{
		plain:          plain(r),
		PlayStyle:      policy.PlayStyle,
		RiskTolerance:  policy.RiskTolerance,
		WildEncounters: policy.WildEncounters,
	})
}

// persistedTile carries gameplay policy through a wall restart. Old state
// files simply omit it and retain legacy compatibility behavior.
func (p persistedTile) MarshalJSON() ([]byte, error) {
	type plain persistedTile
	policy := runPolicyForWallRun(p.RunID, p.ResumeFromRunID)
	return json.Marshal(struct {
		plain
		PlayStyle      string `json:"play_style,omitempty"`
		RiskTolerance  string `json:"risk_tolerance,omitempty"`
		WildEncounters string `json:"wild_encounters,omitempty"`
	}{
		plain:          plain(p),
		PlayStyle:      policy.PlayStyle,
		RiskTolerance:  policy.RiskTolerance,
		WildEncounters: policy.WildEncounters,
	})
}

func (p *persistedTile) UnmarshalJSON(data []byte) error {
	type plain persistedTile
	var in struct {
		plain
		PlayStyle      string `json:"play_style,omitempty"`
		RiskTolerance  string `json:"risk_tolerance,omitempty"`
		WildEncounters string `json:"wild_encounters,omitempty"`
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	*p = persistedTile(in.plain)
	farm.RememberPlayStyle(p.RunID, in.PlayStyle)
	farm.RememberRiskTolerance(p.RunID, in.RiskTolerance)
	farm.RememberWildEncounters(p.RunID, in.WildEncounters)
	return nil
}
