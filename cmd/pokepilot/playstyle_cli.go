package main

import "flag"

// Gameplay policy is intentionally separate from -llm-profile: the latter
// chooses inference hardware, while these flags choose how the run behaves.
// Empty values preserve historical compatibility for old scripts/commands.
var (
	localPlayStyle      = flag.String("play-style", "", "gameplay priorities for llm runs: speedrun, adventure, completionist, or team_builder")
	localRiskTolerance  = flag.String("risk-tolerance", "", "recovery policy for llm runs: aggressive, balanced, or cautious")
	localWildEncounters = flag.String("wild-encounters", "", "wild encounter policy for llm runs: planner or fight")
)

func localPlayStyleName() string {
	if localPlayStyle == nil {
		return ""
	}
	return *localPlayStyle
}

func localRiskToleranceName() string {
	if localRiskTolerance == nil {
		return ""
	}
	return *localRiskTolerance
}

func localWildEncountersName() string {
	if localWildEncounters == nil {
		return ""
	}
	return *localWildEncounters
}
