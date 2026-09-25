package agent

import (
	"errors"
	"fmt"
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
	if boundary, dropped := r.success(true, first); boundary || dropped != 0 {
		t.Fatalf("ordinary cached step unexpectedly ended leg: boundary=%v dropped=%d", boundary, dropped)
	}
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

// truncatingStrategist always gets cut off mid-reply, the failure mode a
// reasoning-heavy recovery escalation (blackout, stuck, ...) produces on a
// model that burns its completion budget on a <think> block before ever
// emitting the closing JSON. It has no StrategizeRetry, so
// strategizeWithRetries cannot re-ask and returns the initial error as-is.
type truncatingStrategist struct{ fast int }

func (p *truncatingStrategist) Next(_ Observation, offered []Objective) (Objective, error) {
	p.fast++
	return offered[0], nil
}
func (p *truncatingStrategist) Strategize(_ Observation, _ []Objective, _ string) (Plan, error) {
	return Plan{}, fmt.Errorf("%w: finish_reason %q", ErrNotFinished, "length")
}

// TestRunPlanningFallsBackToCheapChooserOnLengthTruncation covers the same
// class of farm failure as the unresolved-step case above, but for a
// strategist that never even produces a parseable plan: MEASURED on
// run-1pkm1en1hog5a0, where a blackout replan escalated reasoning effort,
// every retry hit finish_reason "length", and because only
// ErrPlanStepUnresolved degraded to the cheap chooser, the run re-entered the
// same expensive strategist call forever instead of falling back.
func TestRunPlanningFallsBackToCheapChooserOnLengthTruncation(t *testing.T) {
	offered := []Objective{{Kind: KindGoTo, Place: "pewter city"}}
	p := &truncatingStrategist{}
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

func TestValidateStrategicPlanStopsAtFirstWorldBoundary(t *testing.T) {
	offered := []Objective{
		{Kind: KindTrain, Level: 12},
		{Kind: KindGoTo, Place: "route 3", Note: "(unvisited adjacent map)"},
		{Kind: KindGoTo, Place: "pallet town"},
	}
	got, err := validateStrategicPlan(Plan{
		Goal:  "reach mt moon",
		Steps: []string{"train the lead to level 12", "go to route 3", "go to pallet town"},
	}, offered, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Boundary {
		t.Fatalf("plan = %+v; want discovery boundary", got)
	}
	if len(got.Steps) != 2 || got.Steps[1] != "go to route 3" {
		t.Fatalf("steps = %v; speculative tail was not removed", got.Steps)
	}
	if got.TailDropped != 1 {
		t.Fatalf("tail dropped = %d; want 1", got.TailDropped)
	}
	if len(got.StepKeys) != 2 {
		t.Fatalf("step keys = %d; want 2", len(got.StepKeys))
	}
}

func TestRunPlanningAutoContinuesBoundaryLegIntoSingleProgression(t *testing.T) {
	progress := Objective{Kind: KindProgress, Progress: "mt_moon_fossil_acquired"}
	p := &planningTestPlanner{}
	r := newRunPlanning(Plan{
		Goal:     "cross mt moon and reach cerulean",
		Steps:    []string{"go to route 3"},
		StepKeys: []ObjectiveKey{{Kind: KindGoTo, Place: "route 3"}},
		Step:     1,
		Boundary: true,
	})

	obj, fromPlan, err, _ := r.choose(nil, 2, p, Observation{Round: 2}, []Objective{progress})
	if err != nil {
		t.Fatal(err)
	}
	if fromPlan {
		t.Fatal("automatic leg continuation should not advance an exhausted cached step")
	}
	if obj != progress {
		t.Fatalf("obj = %+v; want %+v", obj, progress)
	}
	if p.strategic != 0 || p.fast != 0 {
		t.Fatalf("planner calls strategic=%d fast=%d; want zero-call continuation", p.strategic, p.fast)
	}
	if r.Stats.LegAutoExecutions != 1 || r.Stats.LastLegDecision != "single_progression" {
		t.Fatalf("stats = %+v", r.Stats)
	}
}

func TestRunPlanningUsesCheapChooserForSingleFrontierVariants(t *testing.T) {
	offered := []Objective{
		{Kind: KindGoTo, Place: "route 3", Note: "(unvisited adjacent map)"},
		{Kind: KindGoTo, Place: "route 3", Flee: true, Note: "(unvisited adjacent map)"},
		{Kind: KindGoTo, Place: "pewter city"},
	}
	p := &planningTestPlanner{}
	r := newRunPlanning(Plan{Goal: "reach mt moon", Step: 0, Boundary: true})

	obj, fromPlan, err, _ := r.choose(nil, 2, p, Observation{Round: 2}, offered)
	if err != nil {
		t.Fatal(err)
	}
	if fromPlan {
		t.Fatal("cheap frontier continuation reported as cached plan execution")
	}
	if obj.Place != "route 3" {
		t.Fatalf("obj = %s; want route 3 frontier", obj)
	}
	if p.strategic != 0 || p.fast != 1 {
		t.Fatalf("planner calls strategic=%d fast=%d; want 0/1", p.strategic, p.fast)
	}
	if r.Stats.LegFastExecutions != 1 || r.Stats.FastCalls != 1 {
		t.Fatalf("stats = %+v", r.Stats)
	}
}

func TestRunPlanningReplansWhenBoundaryRevealsRealBranch(t *testing.T) {
	offered := []Objective{
		{Kind: KindGoTo, Place: "route 3", Note: "(unvisited adjacent map)"},
		{Kind: KindGoTo, Place: "route 22", Note: "(unvisited adjacent map)"},
	}
	p := &planningTestPlanner{plans: []Plan{{Goal: "take the story route", Steps: []string{"go to route 3"}}}}
	r := newRunPlanning(Plan{Goal: "keep advancing", Boundary: true})

	obj, fromPlan, err, _ := r.choose(nil, 2, p, Observation{Round: 2}, offered)
	if err != nil {
		t.Fatal(err)
	}
	if !fromPlan || obj.Place != "route 3" {
		t.Fatalf("obj=%s fromPlan=%v; want strategist-selected route 3", obj, fromPlan)
	}
	if p.strategic != 1 {
		t.Fatalf("strategic calls = %d; want 1 for a real branch", p.strategic)
	}
}

func TestRunPlanningDropsLegacyTailWhenBoundaryExecutes(t *testing.T) {
	offered := []Objective{
		{Kind: KindGoTo, Place: "route 3", Note: "(unvisited adjacent map)"},
		{Kind: KindGoTo, Place: "pallet town"},
	}
	r := newRunPlanning(Plan{
		Goal:  "reach mt moon",
		Steps: []string{"go to route 3", "go to pallet town"},
	})
	p := &planningTestPlanner{}

	obj, fromPlan, err, _ := r.choose(nil, 2, p, Observation{Round: 2}, offered)
	if err != nil || !fromPlan {
		t.Fatalf("choose obj=%s fromPlan=%v err=%v", obj, fromPlan, err)
	}
	boundary, dropped := r.success(true, obj)
	if !boundary || dropped != 1 {
		t.Fatalf("boundary=%v dropped=%d; want true/1", boundary, dropped)
	}
	if len(r.Plan.Steps) != 1 || r.Plan.Step != 1 || !r.Plan.Boundary {
		t.Fatalf("plan after boundary = %+v", r.Plan)
	}
	if r.Stats.LegTailStepsDropped != 1 || r.Stats.LegBoundaries != 1 {
		t.Fatalf("stats = %+v", r.Stats)
	}
}


type transientTransportStrategist struct {
	strategic int
}

func (p *transientTransportStrategist) Next(_ Observation, offered []Objective) (Objective, error) {
	return offered[0], nil
}

func (p *transientTransportStrategist) Strategize(_ Observation, _ []Objective, _ string) (Plan, error) {
	p.strategic++
	if p.strategic == 1 {
		return Plan{}, fmt.Errorf("%w: backend timed out", ErrTransport)
	}
	return Plan{Goal: "continue north", Steps: []string{"go to route 1"}}, nil
}

func TestRunPlanningRetriesOneTransientTransportFailure(t *testing.T) {
	offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
	p := &transientTransportStrategist{}
	r := newRunPlanning(Plan{})

	obj, fromPlan, err, retries := r.chooseWithTransportRecovery(nil, 7, p, Observation{Round: 7}, offered)
	if err != nil {
		t.Fatalf("transient transport remained terminal: %v", err)
	}
	if !fromPlan || obj.String() != "go to route 1" {
		t.Fatalf("obj=%q fromPlan=%v, want recovered strategist step", obj.String(), fromPlan)
	}
	if p.strategic != 2 {
		t.Fatalf("strategist calls=%d, want exactly 2", p.strategic)
	}
	if retries != 0 {
		t.Fatalf("reply retries=%d, transport retry must stay separate", retries)
	}
}

type permanentTransportStrategist struct {
	strategic int
}

func (p *permanentTransportStrategist) Next(_ Observation, offered []Objective) (Objective, error) {
	return offered[0], nil
}

func (p *permanentTransportStrategist) Strategize(_ Observation, _ []Objective, _ string) (Plan, error) {
	p.strategic++
	return Plan{}, fmt.Errorf("%w: backend still unavailable", ErrTransport)
}

func TestRunPlanningBoundsTransportRecovery(t *testing.T) {
	offered := []Objective{{Kind: KindGoTo, Place: "route 1"}}
	p := &permanentTransportStrategist{}
	r := newRunPlanning(Plan{})

	_, _, err, _ := r.chooseWithTransportRecovery(nil, 7, p, Observation{Round: 7}, offered)
	if !errors.Is(err, ErrTransport) {
		t.Fatalf("err=%v, want ErrTransport after bounded retries", err)
	}
	if p.strategic != maxPlannerTransportAttempts {
		t.Fatalf("strategist calls=%d, want bound %d", p.strategic, maxPlannerTransportAttempts)
	}
}
