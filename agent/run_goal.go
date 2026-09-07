package agent

import (
	"errors"
	"fmt"
	"strings"
)

// ErrGoalIncomplete is returned when a planner reports ErrDone while an
// opted-in deterministic run goal is still observably incomplete. A planner
// is allowed to run out of ideas; it is not allowed to turn that into a
// successful run when the run has an independent success predicate.
var ErrGoalIncomplete = errors.New("agent: deterministic run goal incomplete")

// RunGoalProvider lets a planner or planner decorator expose the same raw
// run goal it already uses for prompt context. Run uses this only when
// Budget.Goal is empty, so command/farm wrappers do not need their own goal
// evaluator and explicit callers can still configure the run directly.
type RunGoalProvider interface {
	RunGoal() string
}

// RunGoalStatusObserver is an optional diagnostics hook. Run owns the
// completion decision; observers may mirror the status into planner prompts,
// live stats, or operator UIs, but cannot change whether the run is done.
type RunGoalStatusObserver interface {
	ObserveRunGoal(obs Observation, status GoalStatus, deterministic bool)
}

// RunGoal exposes the raw goal on a direct LLM planner. This makes the
// runtime invariant apply even when a caller does not use cmd/pokepilot's
// stats decorator.
func (p *LLMPlanner) RunGoal() string {
	if p == nil {
		return ""
	}
	return p.Goal
}

// RunGoal forwards the primary planner's goal through transport failover.
// The endpoint may change; the run goal must not.
func (p *FailoverPlanner) RunGoal() string {
	if p == nil || p.Primary == nil {
		return ""
	}
	return p.Primary.Goal
}

// resolveRunGoal parses the one run-owned goal contract. Free text remains
// prompt-only (deterministic=false); documented structured syntax and known
// presets become pure observation predicates. Budget.Goal wins over a planner
// provider when both are present.
func resolveRunGoal(p Planner, raw string) (Goal, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if gp, ok := p.(RunGoalProvider); ok {
			raw = strings.TrimSpace(gp.RunGoal())
		}
	}
	g, deterministic, err := PlannerGoal(raw)
	if err != nil {
		return Goal{}, deterministic, fmt.Errorf("agent: run goal: %w", err)
	}
	return g, deterministic, nil
}

// evaluateRunGoal evaluates one settled observation and mirrors the exact
// same authoritative status to diagnostics. The contextual round/intent
// fields are populated on a copy so Result.Final stays a plain game-state
// observation rather than inheriting planner-only metadata.
func evaluateRunGoal(p Planner, g Goal, obs Observation, round, maxRounds int, intent string, intentAge int) GoalStatus {
	goalObs := obs
	goalObs.Intent = intent
	goalObs.IntentAge = intentAge
	goalObs.Round = round
	goalObs.RoundsLeft = roundsLeft(round, maxRounds)
	status := EvaluateGoal(g, goalObs)
	publishRunGoalStatus(p, goalObs, status, true)
	return status
}

func publishRunGoalStatus(p Planner, obs Observation, status GoalStatus, deterministic bool) {
	if sink, ok := p.(RunGoalStatusObserver); ok {
		sink.ObserveRunGoal(obs, status, deterministic)
	}
}

// classifyPlannerDone translates a planner's ErrDone into the run outcome.
// Prompt-only runs keep the historical meaning. Deterministic runs can only
// reach this function while incomplete: Run checks completion before asking
// the planner, so the same signal is a typed false-completion error instead.
func classifyPlannerDone(deterministic bool, status GoalStatus) (Stop, error) {
	if deterministic {
		return StopError, incompleteGoalError(status)
	}
	return StopDone, nil
}

func incompleteGoalError(status GoalStatus) error {
	if status.Summary == "" {
		return ErrGoalIncomplete
	}
	return fmt.Errorf("%w: planner reported done while %s", ErrGoalIncomplete, status.Summary)
}
