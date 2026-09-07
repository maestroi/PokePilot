package agent_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
)

type countingScript struct {
	objs  []agent.Objective
	next  int
	calls int
}

func (p *countingScript) Next(agent.Observation, []agent.Objective) (agent.Objective, error) {
	p.calls++
	if p.next >= len(p.objs) {
		return agent.Objective{}, agent.ErrDone
	}
	o := p.objs[p.next]
	p.next++
	return o, nil
}

func TestRunRejectsPlannerDoneBeforeDeterministicGoal(t *testing.T) {
	e := loadFixture(t)
	p := agent.NewScriptedPlanner()
	res := agent.Run(e, e.ROM(), p, agent.Budget{
		MaxFrames: 10_000_000,
		Goal:      "badges:1",
	})

	if res.Stop != agent.StopError {
		t.Fatalf("Stop = %d, want StopError for premature deterministic completion", res.Stop)
	}
	if !errors.Is(res.Err, agent.ErrGoalIncomplete) {
		t.Fatalf("Err = %v, want ErrGoalIncomplete", res.Err)
	}
	if res.Rounds != 0 {
		t.Fatalf("Rounds = %d, want 0 (planner stopped before an objective ran)", res.Rounds)
	}
	if res.GoalStatus == nil || res.GoalStatus.Complete {
		t.Fatalf("GoalStatus = %+v, want present and incomplete", res.GoalStatus)
	}
}

func TestRunStopsOnObservedGoalBeforeAnotherPlannerCall(t *testing.T) {
	e := loadFixture(t)
	p := &countingScript{objs: []agent.Objective{
		{Kind: agent.KindStarter},
		{Kind: agent.KindGoTo, Place: "pallet town"},
	}}
	res := agent.Run(e, e.ROM(), p, agent.Budget{
		MaxFrames: 10_000_000,
		Goal:      "reach:pallet town",
	})

	if res.Stop != agent.StopDone {
		t.Fatalf("Stop = %d, want StopDone", res.Stop)
	}
	if p.calls != 2 {
		t.Fatalf("planner calls = %d, want 2; completion must stop before a third ask", p.calls)
	}
	if res.Rounds != 2 {
		t.Fatalf("Rounds = %d, want 2", res.Rounds)
	}
	if res.GoalStatus == nil || !res.GoalStatus.Complete {
		t.Fatalf("GoalStatus = %+v, want complete", res.GoalStatus)
	}
}

func TestRunPromptOnlyGoalKeepsPlannerDoneSemantics(t *testing.T) {
	e := loadFixture(t)
	res := agent.Run(e, e.ROM(), agent.NewScriptedPlanner(), agent.Budget{
		MaxFrames: 10_000_000,
		Goal:      "Explore Kanto and see how far you get.",
	})

	if res.Stop != agent.StopDone {
		t.Fatalf("Stop = %d, want StopDone for prompt-only planner completion", res.Stop)
	}
	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if res.GoalStatus != nil {
		t.Fatalf("GoalStatus = %+v, want nil for prompt-only goal", res.GoalStatus)
	}
}

func TestRunRejectsMalformedStructuredGoalBeforeROMWork(t *testing.T) {
	res := agent.Run(nil, nil, agent.NewScriptedPlanner(), agent.Budget{
		MaxFrames: 1,
		Goal:      "badges:99",
	})

	if res.Stop != agent.StopError {
		t.Fatalf("Stop = %d, want StopError", res.Stop)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "run goal") {
		t.Fatalf("Err = %v, want run-goal validation error", res.Err)
	}
	if res.ProgressEarly != nil || res.ProgressFinal != nil {
		t.Fatalf("malformed goal must fail before gameplay: early=%v final=%v", res.ProgressEarly, res.ProgressFinal)
	}
}
