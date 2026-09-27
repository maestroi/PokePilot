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

// scriptedStarterObjective resolves a scripted run's starter. A named request
// keeps the existing Red/Blue starter path. An empty request may use the
// loaded game's only opening starter, which is how Yellow selects Pikachu
// without pretending it has Red's three-ball choice.
func scriptedStarterObjective(m *emu.Emu, request string, seed int64) (agent.Objective, error) {
	obs, err := agent.ObserveChecked(m, m.ROM())
	if err != nil {
		return agent.Objective{}, err
	}
	if o, ok := agent.DefaultStarterObjective(obs); ok {
		req := strings.ToLower(strings.TrimSpace(request))
		if req != "" && req != string(o.Species) {
			return agent.Objective{}, fmt.Errorf("%s uses the scripted %s starter, got %q", obs.GameID, o.Species, request)
		}
		return o, nil
	}
	req := strings.ToLower(strings.TrimSpace(request))
	if req == "" {
		return agent.Objective{}, fmt.Errorf("%s offers a starter choice; name one", obs.GameID)
	}
	if o, ok := agent.StarterObjectiveForSpecies(obs, agent.SpeciesID(req)); ok {
		return o, nil
	}
	return starterObjectiveForRequest(request, seed)
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
