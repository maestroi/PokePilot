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

func TestRunPlanningUsesCheapChooserWithoutStrategist(t *testing.T) {
	offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
	cheap := &cheapPlanningTestPlanner{}
	r := newRunPlanning(Plan{})
	obj, fromPlan, err, _ := r.choose(nil, 1, cheap, Observation{Round: 1}, offered)
	if err != nil || fromPlan || obj.String() != "go to route 1" {
		t.Fatalf("obj=%q fromPlan=%v err=%v", obj.String(), fromPlan, err)
	}
	if cheap.calls != 1 || r.Stats.FastCalls != 1 {
		t.Fatalf("cheap calls=%d stats=%+v", cheap.calls, r.Stats)
	}
}

type cheapPlanningTestPlanner struct{ calls int }

func (p *cheapPlanningTestPlanner) Next(_ Observation, offered []Objective) (Objective, error) {
	p.calls++
	return offered[0], nil
}

func TestRecoverableFailureReplansOnceThenStopsOnSameStructuredCause(t *testing.T) {
	seen := map[string]bool{}
	obj := Objective{Kind: KindGoTo, Place: "route 1"}
	result := ObjectiveResult{Outcome: OutcomeBlocked, Cause: FailureCauseID("route_prerequisite_missing")}
	reason, key, terminal := recoverableFailureReplan(seen, obj, result, false, false, 1, 3)
	if terminal || reason != "objective_failed" || key == "" {
		t.Fatalf("first = reason %q key %q terminal %v", reason, key, terminal)
	}
	seen[key] = true
	_, _, terminal = recoverableFailureReplan(seen, obj, result, false, false, 2, 3)
	if !terminal {
		t.Fatal("same objective/cause after a strategic replan did not become terminal")
	}
}

func TestRecoverableFailureNamesBlackoutAndRetreatReplans(t *testing.T) {
	obj := Objective{Kind: KindTrain, Level: 12}
	result := ObjectiveResult{Outcome: OutcomeBlocked, Cause: FailureCauseID("battle_blackout")}
	reason, _, terminal := recoverableFailureReplan(map[string]bool{}, obj, result, true, false, 1, 3)
	if terminal || reason != "blackout" {
		t.Fatalf("blackout = %q terminal=%v", reason, terminal)
	}
	reason, _, terminal = recoverableFailureReplan(map[string]bool{}, obj, result, false, true, 1, 3)
	if terminal || reason != "train_retreat" {
		t.Fatalf("retreat = %q terminal=%v", reason, terminal)
	}
}

func TestReplanOnceStopsSecondWatchdogEdgeUntilProgressReset(t *testing.T) {
	escalated := false
	if !replanOnce(&escalated) {
		t.Fatal("first watchdog edge did not allow replan")
	}
	if replanOnce(&escalated) {
		t.Fatal("second watchdog edge allowed another replan without progress")
	}
	escalated = false
	if !replanOnce(&escalated) {
		t.Fatal("progress reset did not restore one replan opportunity")
	}
}

func TestRunPlanningValidatesCustomStrategistOutput(t *testing.T) {
	offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
	p := &rawStrategist{plan: Plan{Goal: "cheat", Steps: []string{"1"}}}
	r := newRunPlanning(Plan{})
	if _, _, err, _ := r.choose(nil, 1, p, Observation{Round: 1}, offered); err == nil {
		t.Fatal("Run accepted a custom strategist plan made of a menu index")
	}
}

type rawStrategist struct{ plan Plan }

func (p *rawStrategist) Next(_ Observation, offered []Objective) (Objective, error) {
	return offered[0], nil
}
func (p *rawStrategist) Strategize(_ Observation, _ []Objective, _ string) (Plan, error) {
	return p.plan, nil
}

// TestRunPlanningFallsBackToCheapChooserOnUnresolvedPlanStep covers the farm
// failure where a strategist kept naming a plan step that was never on the
// offered menu (stale, invented, or a formatting near-miss) until its retry
// budget ran out — MEASURED across run-permx767dvp9scvb1rs158xz,
// run-11dgixnaxktf1uax2xvdmdc5z, run-2isumqfgwygzx19ovjpobmorlw and
// run-1dvuxv760j2as3rac0c68gcm2m, all of which ended the run with
// reason:error instead of degrading. Because hallucinatingStrategist has no
// StrategizeRetry (not a StrategicFeedbackPlanner), strategizeWithRetries
// cannot even re-ask; choose must still recognize ErrPlanStepUnresolved and
// fall back to the plain chooser for this round rather than propagating the
// error and killing the run.
type hallucinatingStrategist struct{ fast int }

func (p *hallucinatingStrategist) Next(_ Observation, offered []Objective) (Objective, error) {
	p.fast++
	return offered[0], nil
}
func (p *hallucinatingStrategist) Strategize(_ Observation, _ []Objective, _ string) (Plan, error) {
	return Plan{Goal: "reach pewter", Steps: []string{"go to pewter gym, fleeing wild battles"}}, nil
}

func TestRunPlanningFallsBackToCheapChooserOnUnresolvedPlanStep(t *testing.T) {
	offered := []Objective{{Kind: KindGoTo, Place: "pewter city"}}
	p := &hallucinatingStrategist{}
	r := newRunPlanning(Plan{})

	obj, fromPlan, err, _ := r.choose(nil, 1, p, Observation{Round: 1}, offered)
	if err != nil {
		t.Fatalf("choose returned an error instead of degrading: %v", err)
	}
	if fromPlan {
		t.Fatal("fallback objective reported as coming from the plan")
	}
	if obj.String() != "go to pewter city" {
		t.Fatalf("obj = %q, want the offered fallback objective", obj.String())
	}
	if p.fast != 1 {
		t.Fatalf("cheap chooser calls = %d, want 1", p.fast)
	}
	if r.Stats.FastCalls != 1 {
		t.Fatalf("FastCalls = %d, want 1", r.Stats.FastCalls)
	}
	// The strategist is still installed as nothing (r.Plan never became
	// active), so the next round asks the strategist again rather than
	// being stuck on the rejected plan.
	if r.Plan.Active() {
		t.Fatalf("plan should not be active after a rejected strategize: %+v", r.Plan)
	}
}

// TestRunPlanningKeepsHardStopForStructurallyInvalidPlans confirms the
// fallback is scoped to ErrPlanStepUnresolved only: a plan that is
// structurally broken (here, a menu index used as a step, which
// validateStrategicPlan rejects before ever calling Chosen) still stops the
// run, because that failure means the strategist's OWN output contract is
// broken, not that it named something outside this round's menu.
func TestRunPlanningKeepsHardStopForStructurallyInvalidPlans(t *testing.T) {
	offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
	p := &rawStrategist{plan: Plan{Goal: "cheat", Steps: []string{"1"}}}
	r := newRunPlanning(Plan{})
	if _, _, err, _ := r.choose(nil, 1, p, Observation{Round: 1}, offered); err == nil {
		t.Fatal("Run accepted a custom strategist plan made of a menu index")
	}
}
