package agent

import (
	"errors"
	"fmt"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/skill"
)

// snorlaxStaticCaptureFailure reproduces the exact #2939 farm failure: a
// Route 16 Snorlax static capture whose objective boundary did not stabilize,
// joined with the typed "one-time static source unavailable" fault. The dirty
// boundary dominates classification, so the outcome is terminal
// (OutcomeStabilizationFailed) even though the underlying fault is a plain
// planning boundary.
func snorlaxStaticCaptureFailure() (Objective, ObjectiveResult, error) {
	obj := Objective{Kind: KindCatch, Species: SpeciesID("snorlax"), Place: "route 16"}
	err := fmt.Errorf("agent: %s: %w: %w", obj.String(), ErrObjectiveBoundaryDirty, skill.ErrStaticCaptureUnavailable)
	final := Observation{
		Map:          0x1b,
		Location:     "route 16",
		X:            12,
		Y:            4,
		Controllable: false,
		InBattle:     true,
		GameID:       "pokemon-gold",
		Party:        []PartyMon{{Species: SpeciesID("cyndaquil"), Level: 30}},
	}
	outcome := classifyObjectiveOutcome(obj, err, final)
	cause, context := failureCauseFor(err)
	result := ObjectiveResult{
		Objective: obj,
		Outcome:   outcome,
		Summary:   fmt.Sprintf("stabilization_failed at route 16 (12,4): %v", err),
		Failure: &gameruntime.Failure{
			Class:       failureClassForOutcome(outcome),
			Cause:       string(cause),
			Context:     context,
			Recoverable: actionFor(outcome) == actionReplan,
		},
		Cause:        FailureCauseID(cause),
		CauseContext: append([]string(nil), context...),
		Final:        final,
	}
	return obj, result, err
}

func dirtyJoinedResult(obj Objective, fault error) ObjectiveResult {
	err := fmt.Errorf("agent: %s: %w: %w", obj.String(), ErrObjectiveBoundaryDirty, fault)
	final := Observation{Location: "route 16", Controllable: false, InBattle: true, GameID: "pokemon-gold"}
	outcome := classifyObjectiveOutcome(obj, err, final)
	cause, context := failureCauseFor(err)
	return ObjectiveResult{
		Objective: obj,
		Outcome:   outcome,
		Failure: &gameruntime.Failure{
			Class:   failureClassForOutcome(outcome),
			Cause:   string(cause),
			Context: context,
		},
		Cause:        FailureCauseID(cause),
		CauseContext: append([]string(nil), context...),
		Final:        final,
	}
}

// TestTerminalRunErrorReproClassifiesTerminal documents the pre-fix behavior
// the issue reports: the run-loop action for the #2939 failure is actionStop,
// i.e. the whole farm run halts on the first such error.
func TestTerminalRunErrorReproClassifiesTerminal(t *testing.T) {
	obj, result, err := snorlaxStaticCaptureFailure()
	if !errors.Is(err, skill.ErrStaticCaptureUnavailable) || !errors.Is(err, ErrObjectiveBoundaryDirty) {
		t.Fatalf("repro error lost its typed faults: %v", err)
	}
	if result.Outcome != OutcomeStabilizationFailed {
		t.Fatalf("outcome = %q, want %q", result.Outcome, OutcomeStabilizationFailed)
	}
	if got := string(result.Cause); got != "objective_boundary_dirty" {
		t.Fatalf("cause = %q, want objective_boundary_dirty", got)
	}
	if len(result.CauseContext) != 1 || result.CauseContext[0] != "primary:static_capture_unavailable" {
		t.Fatalf("cause context = %v, want [primary:static_capture_unavailable]", result.CauseContext)
	}
	if actionFor(result.Outcome) != actionStop {
		t.Fatalf("actionFor(%q) = %v, want actionStop (the fatal classification #2939 hit)", result.Outcome, actionFor(result.Outcome))
	}
	if obj.Kind != KindCatch || obj.Species != SpeciesID("snorlax") {
		t.Fatalf("repro objective = %+v", obj)
	}
}

// TestTerminalRunErrorWithRecoverablePrimaryCauseIsDiverted is the core
// regression test: after the fix the same terminal-classified failure is
// diverted into the replan flow, so the run advances to the next task instead
// of stopping permanently.
func TestTerminalRunErrorWithRecoverablePrimaryCauseIsDiverted(t *testing.T) {
	obj, result, _ := snorlaxStaticCaptureFailure()
	policy := newRunFailurePolicy(3)

	if !terminalOutcomeRecoverable(obj, result) {
		t.Fatalf("terminalOutcomeRecoverable = false for primary:static_capture_unavailable, want true")
	}
	if got := policy.execErrorRunAction(obj, result); got != actionReplan {
		t.Fatalf("execErrorRunAction = %v, want actionReplan (run continues with the next objective)", got)
	}

	// The diverted failure must quarantine the exact objective so the next
	// round's planner offers a different task.
	policy.record(result)
	other := Objective{Kind: KindGoTo, Place: "route 17"}
	offered := policy.filter(result.Final, []Objective{obj, other})
	if len(offered) != 1 || offered[0] != other {
		t.Fatalf("filter after diverted terminal error = %v, want only the next task %v", offered, other)
	}

	// The bounded failure policy must classify the unwrapped failure as
	// recovered planning feedback, not spend the fatal consecutive budget.
	unwrapped := unwrapDirtyBoundaryCause(result)
	if string(unwrapped.Cause) != "static_capture_unavailable" {
		t.Fatalf("unwrapped cause = %q, want static_capture_unavailable", unwrapped.Cause)
	}
	if len(unwrapped.CauseContext) != 0 {
		t.Fatalf("unwrapped context = %v, want empty", unwrapped.CauseContext)
	}
	if unwrapped.Failure == nil || unwrapped.Failure.Cause != "static_capture_unavailable" || !unwrapped.Failure.Recoverable {
		t.Fatalf("unwrapped failure = %+v", unwrapped.Failure)
	}
	for i := 0; i < 5; i++ {
		decision := policy.recoverable(obj, unwrapped, false, 30)
		if decision.Stop != StopUnset || !decision.Recovered {
			t.Fatalf("recoverable() miss %d = %+v, want recovered without stopping", i+1, decision)
		}
	}
	// The original result keeps its terminal classification for telemetry.
	if result.Outcome != OutcomeStabilizationFailed || string(result.Cause) != "objective_boundary_dirty" {
		t.Fatalf("original result mutated: outcome=%q cause=%q", result.Outcome, result.Cause)
	}
}

// TestTerminalRunErrorGenuineFaultsStillStopRun keeps the fail-closed contract:
// terminal errors whose primary cause is a real controller defect (or that
// carry no primary cause at all) must still stop the run so the wall can
// retry the attempt.
func TestTerminalRunErrorGenuineFaultsStillStopRun(t *testing.T) {
	policy := newRunFailurePolicy(3)
	catch := Objective{Kind: KindCatch, Species: SpeciesID("snorlax")}

	cases := []struct {
		name   string
		result ObjectiveResult
	}{
		{"menu_stuck", dirtyJoinedResult(catch, skill.ErrMenuStuck)},
		{"battle_ownership", dirtyJoinedResult(catch, skill.ErrBattle)},
		{"unknown_failure", dirtyJoinedResult(catch, errors.New("boom"))},
		{"no_primary_cause", ObjectiveResult{
			Objective: catch,
			Outcome:   OutcomeStabilizationFailed,
			Cause:     "no_path",
			Final:     Observation{Controllable: false},
		}},
		{"ownership_outcome", ObjectiveResult{
			Objective: catch,
			Outcome:   OutcomeOwnershipFailure,
			Cause:     "battle_escaped_owner",
			Final:     Observation{Controllable: true},
		}},
		{"controller_uncertain", ObjectiveResult{
			Objective: catch,
			Outcome:   OutcomeControllerUncertain,
			Cause:     "menu_stuck",
			Final:     Observation{Controllable: false},
		}},
	}
	for _, tc := range cases {
		if terminalOutcomeRecoverable(tc.result.Objective, tc.result) {
			t.Fatalf("%s: terminalOutcomeRecoverable = true, want false (fail-closed)", tc.name)
		}
		if got := policy.execErrorRunAction(tc.result.Objective, tc.result); got != actionStop {
			t.Fatalf("%s: execErrorRunAction = %v, want actionStop", tc.name, got)
		}
	}

	// A dirty boundary around a route blockage (no primary marker) stays fatal.
	blocked := ObjectiveResult{
		Objective: Objective{Kind: KindGoTo, Place: "route 16"},
		Outcome:   OutcomeStabilizationFailed,
		Cause:     "no_path",
		Final:     Observation{Controllable: false, InBattle: true},
	}
	if got := policy.execErrorRunAction(blocked.Objective, blocked); got != actionStop {
		t.Fatalf("unsafe-boundary no_path: execErrorRunAction = %v, want actionStop", got)
	}
}

// TestMultipleConsecutiveDivertedTerminalErrorsKeepRunAlive simulates a run
// that hits the terminal error repeatedly (the deterministic Snorlax capture
// is unavailable on every attempt): the farm run must survive any number of
// consecutive occurrences because each one is quarantined and replanned.
func TestMultipleConsecutiveDivertedTerminalErrorsKeepRunAlive(t *testing.T) {
	obj, result, _ := snorlaxStaticCaptureFailure()
	policy := newRunFailurePolicy(3)

	for i := 1; i <= 8; i++ {
		if got := policy.execErrorRunAction(obj, result); got != actionReplan {
			t.Fatalf("error %d: execErrorRunAction = %v, want actionReplan", i, got)
		}
		policy.record(result)
		decision := policy.recoverable(obj, unwrapDirtyBoundaryCause(result), false, 30)
		if decision.Stop != StopUnset {
			t.Fatalf("error %d: recoverable() stopped the run (%v) after %d consecutive terminal errors; the farm run must keep going", i, decision.Stop, i)
		}
		if !decision.Recovered {
			t.Fatalf("error %d: decision = %+v, want recovered", i, decision)
		}
	}

	// Every occurrence quarantined the same objective, so the planner keeps
	// being offered the remaining tasks; fail-open only applies when nothing
	// else is left.
	other := Objective{Kind: KindHeal, Place: "route 16 pokemon center"}
	offered := policy.filter(result.Final, []Objective{obj, other})
	if len(offered) != 1 || offered[0] != other {
		t.Fatalf("offered after 8 consecutive terminal errors = %v, want [%v]", offered, other)
	}
}

// TestRecoverySucceedsAfterDivertedTerminalError covers the happy path: after
// the diverted terminal error the next objective succeeds, which clears the
// quarantine and the failure streak, and the outcome is reported as recovered
// (not terminal) so the farm does not file a progression blocker.
func TestRecoverySucceedsAfterDivertedTerminalError(t *testing.T) {
	obj, result, _ := snorlaxStaticCaptureFailure()
	policy := newRunFailurePolicy(3)

	if got := policy.execErrorRunAction(obj, result); got != actionReplan {
		t.Fatalf("execErrorRunAction = %v, want actionReplan", got)
	}
	policy.record(result)
	decision := policy.recoverable(obj, unwrapDirtyBoundaryCause(result), true, 30)
	if decision.Stop != StopUnset || !decision.Recovered || decision.ReplanReason == "" {
		t.Fatalf("diverted decision = %+v, want recovered with a replan reason", decision)
	}

	// The next task succeeds: quarantine and streak clear, the run continues.
	next := Objective{Kind: KindGoTo, Place: "route 17"}
	policy.success()
	policy.clear(obj)
	offered := policy.filter(result.Final, []Objective{obj, next})
	if len(offered) != 2 {
		t.Fatalf("offered after recovery = %v, want both objectives back", offered)
	}

	// Telemetry marking: the run loop marks the diverted outcome recovered.
	res := Result{Stop: StopUnset, Outcomes: []ObjectiveResult{result}}
	markLastOutcomeRecovered(&res)
	if !res.Outcomes[0].Recovered || res.Outcomes[0].Terminal {
		t.Fatalf("marked outcome = recovered=%t terminal=%t, want true/false", res.Outcomes[0].Recovered, res.Outcomes[0].Terminal)
	}
}

// TestConsecutiveGenuineTerminalErrorsStopRunOnce documents the other half of
// the contract: a genuine terminal error stops the run immediately (exactly
// once), which is the signal for the wall to mark the attempt failed and
// requeue it.
func TestConsecutiveGenuineTerminalErrorsStopRunOnce(t *testing.T) {
	policy := newRunFailurePolicy(3)
	obj := Objective{Kind: KindCatch, Species: SpeciesID("snorlax")}
	result := dirtyJoinedResult(obj, skill.ErrMenuStuck)

	for i := 1; i <= 3; i++ {
		if got := policy.execErrorRunAction(obj, result); got != actionStop {
			t.Fatalf("genuine terminal error %d: execErrorRunAction = %v, want actionStop", i, got)
		}
		if terminalOutcomeRecoverable(obj, result) {
			t.Fatalf("genuine terminal error %d classified recoverable", i)
		}
		// A genuine stop is not quarantined as a planning boundary.
		policy.record(result)
		if len(policy.quarantine) != 0 {
			t.Fatalf("genuine terminal error %d quarantined %v", i, policy.quarantine)
		}
	}
}

// TestUnwrapDirtyBoundaryCauseKeepsNonDirtyResultsUntouched guards the
// unwrap helper's no-op path.
func TestUnwrapDirtyBoundaryCauseKeepsNonDirtyResultsUntouched(t *testing.T) {
	plain := ObjectiveResult{
		Objective:    Objective{Kind: KindGoTo, Place: "route 1"},
		Outcome:      OutcomeBlocked,
		Cause:        "no_path",
		CauseContext: []string{"surf"},
	}
	got := unwrapDirtyBoundaryCause(plain)
	if got.Cause != plain.Cause || len(got.CauseContext) != 1 || got.Outcome != plain.Outcome {
		t.Fatalf("unwrap changed a non-dirty result: %+v", got)
	}
}
