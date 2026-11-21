package agent

import "strings"

// A terminal run error used to stop the whole farm run on the first
// objective whose execution error classified as actionStop — in practice
// OutcomeStabilizationFailed, because a dirty objective boundary
// (ErrObjectiveBoundaryDirty) dominates every other classification. When the
// fault the boundary wrapped is really a recoverable planning boundary (a
// one-time static capture that is gone, an exhausted hunt budget, a missing
// route capability, ...), that is objective-local feedback: the run must
// quarantine the objective, advance to the next one and keep going, while the
// bounded failure/watchdog budgets stay the outer guard. Only genuine
// terminal controller faults may stop the run and let the wall retry the
// attempt (#2939).

// terminalDivertPrimaryCauses lists the underlying ("primary:<cause>") faults
// that make a terminal-classified objective failure safe to recover from
// in-loop. The list mirrors the planning-boundary and productive-session
// causes runFailurePolicy.recoverable already treats as non-fatal when the
// same fault arrives as a plain blocked outcome; keeping them in one place
// lets both paths agree on what "recoverable" means.
var terminalDivertPrimaryCauses = []string{
	// Productive bounded sessions whose terminal condition was not reached.
	"train_progress_shortfall",
	"catch_hunt_exhausted",
	"fishing_hunt_exhausted",
	"catch_attempt_missed",
	"static_capture_exhausted",
	// Route/planning/resource boundaries.
	"route_prerequisite_missing",
	"no_route",
	"no_path",
	"route_replan_exhausted",
	"navigation_stalled",
	"transition_execution_failed",
	"push_puzzle_search_exhausted",
	"progression_prerequisite_missing",
	"training_inefficient_area",
	"static_capture_unavailable",
	"cant_afford",
	"not_in_stock",
	"bag_not_risen",
	// Expected gameplay outcomes with dedicated recovery campaigns.
	"combat_defeat",
}

// primaryFailureCause returns the underlying fault that a dirty objective
// boundary wrapped, carried in the failure context as "primary:<cause>" (see
// dirtyBoundaryPrimaryCause). It returns "" when the failure was not a
// joined dirty-boundary error, in which case the terminal classification is
// taken at face value and stays fail-closed.
func primaryFailureCause(result ObjectiveResult) string {
	for _, ctx := range result.CauseContext {
		if strings.HasPrefix(ctx, "primary:") {
			return strings.TrimPrefix(ctx, "primary:")
		}
	}
	return ""
}

// terminalOutcomeRecoverable reports whether an outcome classified as a fatal
// stop (actionStop) actually wraps one of the recoverable causes above. Only
// the joined dirty-boundary form qualifies: the executor explicitly recorded
// both the unstable boundary and the underlying fault, and that fault is a
// planning boundary rather than a controller defect. Plain terminal outcomes
// without a primary cause (unknown_failure, ownership_failure, an unsafe
// boundary around a route blockage, ...) remain fail-closed.
func terminalOutcomeRecoverable(obj Objective, result ObjectiveResult) bool {
	if actionFor(result.Outcome) != actionStop {
		return false
	}
	primary := primaryFailureCause(result)
	if primary == "" {
		return false
	}
	// A local Talk that reached no dialogue is only a planning boundary for
	// Talk objectives; for anything else the cause is not meaningful.
	if primary == "no_dialogue" && obj.Kind != KindTalk {
		return false
	}
	for _, cause := range terminalDivertPrimaryCauses {
		if primary == cause {
			return true
		}
	}
	return false
}

// execErrorRunAction maps one failed objective execution onto the run-loop
// action. Fatal terminal errors keep actionStop, so the run stops, the outcome
// is marked terminal and the wall retries the attempt. A terminal-classified
// outcome that wraps a recoverable primary cause is diverted to actionReplan:
// the run loop quarantines the objective and advances to the next task
// instead of stopping the farm run (#2939).
func (f *runFailurePolicy) execErrorRunAction(obj Objective, result ObjectiveResult) runAction {
	action := actionFor(result.Outcome)
	if action == actionStop && terminalOutcomeRecoverable(obj, result) {
		return actionReplan
	}
	return action
}

// unwrapDirtyBoundaryCause replaces a joined dirty-boundary failure's
// aggregate cause ("objective_boundary_dirty") with the underlying primary
// fault, so the bounded failure policy classifies it exactly like the same
// fault arriving without the dirty boundary. The objective result stored on
// the run's outcome history is intentionally left untouched: telemetry keeps
// the terminal classification and the primary-cause context as evidence.
func unwrapDirtyBoundaryCause(result ObjectiveResult) ObjectiveResult {
	primary := primaryFailureCause(result)
	if primary == "" {
		return result
	}
	out := result
	out.Cause = FailureCauseID(primary)
	if out.Failure != nil {
		failure := *out.Failure
		failure.Cause = primary
		failure.Recoverable = true
		out.Failure = &failure
	}
	if len(result.CauseContext) > 0 {
		ctx := make([]string, 0, len(result.CauseContext))
		for _, c := range result.CauseContext {
			if c == "primary:"+primary {
				continue
			}
			ctx = append(ctx, c)
		}
		out.CauseContext = ctx
	}
	return out
}
