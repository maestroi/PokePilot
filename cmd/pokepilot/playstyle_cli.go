package main

import (
	"flag"

	"github.com/maestroi/pokepilot/farm"
)

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

// goalFlagProvided reports whether -goal was supplied explicitly. An explicit
// empty -goal means Free play and must not gain a play-style default.
func goalFlagProvided() bool {
	provided := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "goal" {
			provided = true
		}
	})
	return provided
}

// resolveLocalGoal keeps an explicitly supplied -goal authoritative. Without
// one, an explicit play style supplies its own terminal default; a legacy
// command with no play style retains main.go's historical elite-four default.
func resolveLocalGoal(current string) string {
	if goalFlagProvided() {
		return current
	}
	if goal := farm.DefaultGoalForPlayStyle(localPlayStyleName()); goal != "" {
		return goal
	}
	return current
}

// localRunPolicy assembles the CLI-selected gameplay policy as the same value
// type a leased Spec produces, so local runs feed the planner through one path
// instead of reading process-global flag state inside it.
//
// The goal is already resolved: resolveLocalGoal applies the play-style default
// unless -goal was supplied, and an explicit empty -goal is Free play. Marking
// it provided here keeps that decision from being applied twice.
func localRunPolicy(goal string) farm.RunPolicy {
	spec := farm.Spec{
		Planner:        "llm",
		Goal:           farm.GoalFrom(goal),
		PlayStyle:      localPlayStyleName(),
		RiskTolerance:  localRiskToleranceName(),
		WildEncounters: localWildEncountersName(),
	}
	return farm.RunPolicyFor(spec)
}
