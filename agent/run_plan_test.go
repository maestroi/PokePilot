package agent_test

import (
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

type strategicScriptPlanner struct {
	plans     []agent.Plan
	fastCalls int
	planCalls int
}

func (p *strategicScriptPlanner) Next(_ agent.Observation, offered []agent.Objective) (agent.Objective, error) {
	p.fastCalls++
	if len(offered) == 0 {
		return agent.Objective{}, agent.ErrDone
	}
	return offered[0], nil
}

func (p *strategicScriptPlanner) Strategize(_ agent.Observation, offered []agent.Objective, _ string) (agent.Plan, error) {
	p.planCalls++
	if len(p.plans) == 0 {
		return agent.Plan{}, agent.ErrDone
	}
	plan := p.plans[0]
	p.plans = p.plans[1:]
	return plan, nil
}

// This fixture-backed integration pins the zero-call tier through Run itself:
// one strategist call creates two legal starter-room steps, and the second
// round must not ask the cheap chooser again. It naturally skips with the
// rest of run_test when no prepared ROM fixture is available.
func TestRunExecutesPersistentPlanWithoutChooserCall(t *testing.T) {
	e := loadFixture(t)
	p := &strategicScriptPlanner{plans: []agent.Plan{{
		Goal:  "leave the bedroom",
		Steps: []string{"take the charmander starter", "go to pallet town"},
		Round: 1,
	}}}
	res := agent.Run(e, e.ROM(), p, agent.Budget{MaxRounds: 2, MaxFrames: 10_000_000})
	if res.Stop != agent.StopBudget && res.Stop != agent.StopDone {
		t.Fatalf("stop=%v err=%v", res.Stop, res.Err)
	}
	if p.planCalls != 1 || p.fastCalls != 0 {
		t.Fatalf("calls strategist=%d chooser=%d, want 1/0", p.planCalls, p.fastCalls)
	}
	if res.Planning.PlanExecutions != 2 {
		t.Fatalf("plan executions=%d, want 2", res.Planning.PlanExecutions)
	}
}
