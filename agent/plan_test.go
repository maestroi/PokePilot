package agent

import (
	"errors"
	"testing"
)

func TestResolvePlanStepSkipsStaleSentences(t *testing.T) {
	offered := []Objective{
		{Kind: KindGoTo, Place: "pallet town"},
		{Kind: KindGoTo, Place: "route 1"},
	}
	plan := Plan{Goal: "go north", Steps: []string{"heal the party here", "go to route 1"}}
	obj, skipped, ok := resolvePlanStep(&plan, offered)
	if !ok || skipped != 1 || obj.String() != "go to route 1" || plan.Step != 1 {
		t.Fatalf("resolve = obj %q skipped %d ok %v step %d", obj.String(), skipped, ok, plan.Step)
	}
}

func TestValidateStrategicPlanRequiresObjectiveSentences(t *testing.T) {
	offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
	if _, err := validateStrategicPlan(Plan{Goal: "north", Steps: []string{"1"}}, offered, 4); err == nil {
		t.Fatal("menu index was accepted as a durable plan step")
	}
	got, err := validateStrategicPlan(Plan{Goal: "north", Steps: []string{"GO TO ROUTE 1"}}, offered, 4)
	if err != nil {
		t.Fatalf("sentence plan: %v", err)
	}
	if got.Steps[0] != "go to route 1" || got.Round != 4 || got.Step != 0 {
		t.Fatalf("canonical plan = %+v", got)
	}
}

type planningTestPlanner struct {
	strategic int
	fast      int
	plans     []Plan
}

func (p *planningTestPlanner) Next(_ Observation, offered []Objective) (Objective, error) {
	p.fast++
	if len(offered) == 0 {
		return Objective{}, ErrDone
	}
	return offered[0], nil
}

func (p *planningTestPlanner) Strategize(_ Observation, offered []Objective, _ string) (Plan, error) {
	p.strategic++
	if len(p.plans) == 0 {
		return Plan{}, errors.New("no test plan")
	}
	plan := p.plans[0]
	p.plans = p.plans[1:]
	return validateStrategicPlan(plan, offered, 1)
}

func TestRunPlanningExecutesPlanWithoutFastCalls(t *testing.T) {
	offered := []Objective{
		{Kind: KindGoTo, Place: "pallet town"},
		{Kind: KindGoTo, Place: "route 1"},
	}
	p := &planningTestPlanner{plans: []Plan{{Goal: "north", Steps: []string{"go to pallet town", "go to route 1"}}}}
	r := newRunPlanning(Plan{})

	first, fromPlan, err, _ := r.choose(nil, 1, p, Observation{Round: 1}, offered)
	if err != nil || !fromPlan || first.String() != "go to pallet town" {
		t.Fatalf("first = %q fromPlan=%v err=%v", first.String(), fromPlan, err)
	}
	r.success(true)
	second, fromPlan, err, _ := r.choose(nil, 2, p, Observation{Round: 2}, offered)
	if err != nil || !fromPlan || second.String() != "go to route 1" {
		t.Fatalf("second = %q fromPlan=%v err=%v", second.String(), fromPlan, err)
	}
	if p.strategic != 1 || p.fast != 0 {
		t.Fatalf("calls strategic=%d fast=%d, want 1/0", p.strategic, p.fast)
	}
	if r.Stats.PlanExecutions != 2 {
		t.Fatalf("plan executions = %d, want 2", r.Stats.PlanExecutions)
	}
}

func TestRunPlanningReplansWhenRequested(t *testing.T) {
	offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
	p := &planningTestPlanner{plans: []Plan{
		{Goal: "first", Steps: []string{"go to route 1"}},
		{Goal: "recover", Steps: []string{"go to route 1"}},
	}}
	r := newRunPlanning(Plan{})
	if _, _, err, _ := r.choose(nil, 1, p, Observation{Round: 1}, offered); err != nil {
		t.Fatal(err)
	}
	r.request("objective_failed")
	if _, _, err, _ := r.choose(nil, 2, p, Observation{Round: 2}, offered); err != nil {
		t.Fatal(err)
	}
	if p.strategic != 2 || r.Stats.ReplanReasons["objective_failed"] != 1 || r.Plan.Goal != "recover" {
		t.Fatalf("planner=%d stats=%+v plan=%+v", p.strategic, r.Stats, r.Plan)
	}
}
