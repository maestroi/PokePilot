package bench

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/boxxle"
	"github.com/maestroi/pokepilot/boxxle/policy"
	"github.com/maestroi/pokepilot/boxxle/solver"
)

// policyProposer adapts the greedy deterministic policy to the Proposer
// seam. It stands in for a model planner in the harness tests.
func policyProposer(state boxxle.State) (boxxle.Pos, boxxle.Direction, PlannerCall, error) {
	dec, err := policy.Choose(state)
	if err != nil {
		return boxxle.Pos{}, boxxle.DirUp, PlannerCall{Backend: "policy"}, err
	}
	p := dec.Candidate.Push
	return p.Crate, p.Dir, PlannerCall{Backend: "policy", Latency: time.Millisecond}, nil
}

// TestPolicySolvesFixtures drives the greedy policy over every fixture with
// the fallback disabled: the policy is a real (if weak) planner, and the
// harness must measure it end to end. Solvability is checked against the
// solver oracle, so the unsolvable fixtures are expected to stay unsolved.
func TestPolicySolvesFixtures(t *testing.T) {
	puzzles, err := solver.Fixtures()
	if err != nil {
		t.Fatalf("Fixtures: %v", err)
	}
	solved := 0
	for _, p := range puzzles {
		_, solvable, err := solver.Solve(p.State)
		if err != nil {
			t.Fatalf("%s: Solve: %v", p.Name, err)
		}
		run := Drive(p, policyProposer, FallbackConfig{})
		if run.Solved != solvable {
			t.Errorf("%s: policy run solved=%v, oracle says solvable=%v (pushes=%d invalid=%d deadlocks=%d failures=%d)",
				p.Name, run.Solved, solvable, run.Pushes, run.InvalidPushes, run.Deadlocks, run.CallFailures)
			continue
		}
		if !solvable {
			continue
		}
		solved++
		if run.UsedFallback {
			t.Errorf("%s: solved with fallback disabled", p.Name)
		}
		if len(run.Calls) == 0 {
			t.Errorf("%s: no planner calls recorded", p.Name)
		}
	}
	if solved == 0 {
		t.Fatal("no fixture solved by the policy")
	}
}

// TestReferenceRunSolves checks the solver reference run: solved, minimum
// pushes, no planner calls.
func TestReferenceRunSolves(t *testing.T) {
	p, err := fixtureByName(t, "trivial_1")
	if err != nil {
		t.Fatal(err)
	}
	run := ReferenceRun(p)
	if !run.Solved {
		t.Fatal("reference run not solved")
	}
	if run.Pushes != 1 {
		t.Errorf("reference pushes = %d, want 1", run.Pushes)
	}
	if len(run.Calls) != 0 {
		t.Errorf("reference run recorded %d planner calls, want 0", len(run.Calls))
	}
	if run.UsedFallback {
		t.Error("reference run marked as fallback")
	}
}

// TestFallbackRescuesStuckPlanner checks the configurable fallback seam: a
// proposer that only proposes illegal pushes must trip the invalid-push
// threshold, hand off to the solver, and finish solved with the fallback
// pushes counted separately.
func TestFallbackRescuesStuckPlanner(t *testing.T) {
	p, err := fixtureByName(t, "trivial_1")
	if err != nil {
		t.Fatal(err)
	}
	dumb := func(state boxxle.State) (boxxle.Pos, boxxle.Direction, PlannerCall, error) {
		// Propose pushing a crate that is not there: always invalid.
		return boxxle.Pos{X: 0, Y: 0}, boxxle.DirUp, PlannerCall{Backend: "dumb"}, nil
	}
	fb := FallbackConfig{Enabled: true, MaxInvalid: 3}
	run := Drive(p, dumb, fb)
	if !run.Solved {
		t.Fatalf("fallback run not solved (invalid=%d)", run.InvalidPushes)
	}
	if !run.UsedFallback {
		t.Fatal("fallback not marked as used")
	}
	if run.FallbackPushes != 1 {
		t.Errorf("fallback pushes = %d, want 1", run.FallbackPushes)
	}
	if run.InvalidPushes != 3 {
		t.Errorf("invalid pushes = %d, want 3 (the threshold)", run.InvalidPushes)
	}
}

// TestFallbackDisabledStaysStuck checks that with the fallback disabled the
// harness does not hand off: the run ends unsolved at the safety cap with
// the failures counted.
func TestFallbackDisabledStaysStuck(t *testing.T) {
	p, err := fixtureByName(t, "trivial_1")
	if err != nil {
		t.Fatal(err)
	}
	dumb := func(state boxxle.State) (boxxle.Pos, boxxle.Direction, PlannerCall, error) {
		return boxxle.Pos{X: 0, Y: 0}, boxxle.DirUp, PlannerCall{Backend: "dumb"}, nil
	}
	run := Drive(p, dumb, FallbackConfig{})
	if run.Solved {
		t.Fatal("stuck run reported solved without fallback")
	}
	if run.UsedFallback {
		t.Fatal("fallback used although disabled")
	}
	if run.InvalidPushes == 0 {
		t.Fatal("no invalid pushes counted")
	}
}

// TestLatencyPercentiles checks p50/p95 computation on synthetic planner
// calls with known latencies: one valid push, then two invalid ones that
// trip the fallback threshold, so exactly three calls (1, 2, 100 ms) are
// recorded.
func TestLatencyPercentiles(t *testing.T) {
	p, err := fixtureByName(t, "cross_2")
	if err != nil {
		t.Fatal(err)
	}
	latencies := []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		100 * time.Millisecond,
	}
	i := 0
	proposer := func(state boxxle.State) (boxxle.Pos, boxxle.Direction, PlannerCall, error) {
		call := PlannerCall{Backend: "synthetic", Latency: latencies[i%len(latencies)]}
		i++
		if i == 1 {
			// First call: push the left crate up onto its goal.
			return boxxle.Pos{X: 3, Y: 3}, boxxle.DirUp, call, nil
		}
		// Then: propose a push that is not there.
		return boxxle.Pos{X: 0, Y: 0}, boxxle.DirUp, call, nil
	}
	fb := FallbackConfig{Enabled: true, MaxInvalid: 2}
	run := Drive(p, proposer, fb)
	if !run.Solved {
		t.Fatal("run not solved")
	}
	if !run.UsedFallback {
		t.Fatal("fallback not used")
	}
	if len(run.Calls) != 3 {
		t.Fatalf("calls = %d, want 3", len(run.Calls))
	}
	rep := Aggregate("synthetic", []Run{run}, map[string]int{"cross_2": 2})
	if len(rep.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rep.Rows))
	}
	m := rep.Rows[0]
	// p50 of {1,2,100} is 2; p95 interpolates 0.9 of the way to 100.
	if m.LatencyP50MS != 2 {
		t.Errorf("p50 = %v ms, want 2", m.LatencyP50MS)
	}
	if m.LatencyP95MS < 90 || m.LatencyP95MS > 100 {
		t.Errorf("p95 = %v ms, want in [90,100]", m.LatencyP95MS)
	}
	if m.InferenceTimeMS != 103 {
		t.Errorf("inference time = %v ms, want 103", m.InferenceTimeMS)
	}
}

// TestAggregateAndCompareText smoke-checks the report: rates, distributions,
// and the text comparison.
func TestAggregateAndCompareText(t *testing.T) {
	p, err := fixtureByName(t, "trivial_1")
	if err != nil {
		t.Fatal(err)
	}
	modelRun := Drive(p, policyProposer, FallbackConfig{})
	referenceRun := ReferenceRun(p)
	reference := map[string]int{"trivial_1": 1}

	model := Aggregate("policy", []Run{modelRun}, reference)
	referenceReport := Aggregate("solver", []Run{referenceRun}, reference)

	if model.Puzzles != 1 || model.Solved != 1 {
		t.Fatalf("model report: puzzles=%d solved=%d, want 1/1", model.Puzzles, model.Solved)
	}
	if model.SolveRate != 1 {
		t.Errorf("solve rate = %v, want 1", model.SolveRate)
	}
	if model.SolvedWithoutFallback != 1 {
		t.Errorf("solved without fallback = %d, want 1", model.SolvedWithoutFallback)
	}
	if model.Rows[0].SolutionRatio <= 0 {
		t.Errorf("solution ratio = %v, want > 0", model.Rows[0].SolutionRatio)
	}
	if model.Rows[0].ReferencePushes != 1 {
		t.Errorf("reference pushes = %d, want 1", model.Rows[0].ReferencePushes)
	}

	text := CompareText(model, referenceReport)
	for _, want := range []string{"boxxle benchmark", "solve rate", "solution ratio", "latency"} {
		if !strings.Contains(text, want) {
			t.Errorf("CompareText missing %q:\n%s", want, text)
		}
	}
}

func fixtureByName(t *testing.T, name string) (solver.Puzzle, error) {
	t.Helper()
	puzzles, err := solver.Fixtures()
	if err != nil {
		return solver.Puzzle{}, err
	}
	for _, p := range puzzles {
		if p.Name == name {
			return p, nil
		}
	}
	return solver.Puzzle{}, fmt.Errorf("fixture %s not found", name)
}
