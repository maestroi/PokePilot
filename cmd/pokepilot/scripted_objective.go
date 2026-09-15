package main

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	redstarter "github.com/maestroi/pokepilot/red/starter"
	"github.com/maestroi/pokepilot/skill"
)

// starterObjectiveForRequest keeps the physical Oak-ball selector separate
// from the semantic species the transaction must verify. Random/fixed starter
// experiments patch the middle ball, so Starter remains Squirtle while Species
// records the actual Pokemon selected for this run.
func starterObjectiveForRequest(request string, seed int64) (agent.Objective, error) {
	selection, err := redstarter.Resolve(request, seed)
	if err != nil {
		return agent.Objective{}, err
	}
	starter, ok := skill.StarterFromRequest(request)
	if !ok {
		return agent.Objective{}, fmt.Errorf("unknown starter %q", request)
	}
	return agent.Objective{
		Kind:    agent.KindStarter,
		Starter: starter,
		Species: agent.SpeciesID(selection.Species),
	}, nil
}

func executeScriptedObjective(m *emu.Emu, o agent.Objective) (agent.ObjectiveResult, error) {
	return agent.Execute(m, m.ROM(), o)
}

// scriptedObjectiveDetail makes farm/CLI failures useful without falling back
// to raw skill errors as the only contract. Summary remains human-readable,
// while Outcome/Cause/Context are the same normalized fields used by agent.Run.
func scriptedObjectiveDetail(result agent.ObjectiveResult, err error) string {
	parts := []string{fmt.Sprintf("objective=%q", result.Objective.String())}
	if result.Outcome != "" {
		parts = append(parts, "outcome="+string(result.Outcome))
	}
	if result.Cause != "" {
		parts = append(parts, "cause="+string(result.Cause))
	}
	if len(result.CauseContext) > 0 {
		parts = append(parts, "context="+strings.Join(result.CauseContext, ","))
	}
	if result.Summary != "" {
		parts = append(parts, "summary="+result.Summary)
	} else if err != nil {
		parts = append(parts, "error="+err.Error())
	}
	return strings.Join(parts, "; ")
}

func captureScriptedObjectiveTelemetry(stop agent.Stop, results []agent.ObjectiveResult, err error) {
	captureObjectiveFailureTelemetry(agent.Result{
		Stop:     stop,
		Outcomes: append([]agent.ObjectiveResult(nil), results...),
		Err:      err,
	})
}
