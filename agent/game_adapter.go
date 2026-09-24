package agent

import (
	"errors"
	"fmt"

	gameruntime "github.com/maestroi/pokepilot/game"
)

// ObjectiveGameAdapter is the seam between the game-agnostic objective
// transaction runtime and one concrete game implementation. The runtime owns
// lifecycle ordering; the adapter owns game facts, state inspection,
// controller mechanics, positive postcondition evidence, and translation of
// native errors into the portable failure vocabulary.
type ObjectiveGameAdapter interface {
	gameruntime.Adapter[Objective, Observation, ObjectiveResult]

	// NormalizeFailure translates one native adapter/runtime error at a known
	// transaction phase. Generic recovery policy must depend only on the
	// returned record, never on the native error identity.
	NormalizeFailure(gameruntime.FailurePhase, error, Observation) gameruntime.Failure

	// CaptureFailure persists game-specific forensic evidence. It is diagnostic
	// only: a capture error must never replace the gameplay/runtime error.
	CaptureFailure(Objective, error) error
}

// ExecuteWithAdapter is the game-agnostic objective transaction entrypoint.
// Callers bind the active game's adapter outside the generic runtime; no
// emulator, ROM, native map ids, or game-owned skills are needed here.
func ExecuteWithAdapter(a ObjectiveGameAdapter, o Objective) (ObjectiveResult, error) {
	return executeObjectiveWithAdapter(a, o)
}

// executeObjectiveWithAdapter is the internal transaction boundary. The
// portable game package owns lifecycle ordering; this layer attaches the game
// adapter's normalized failure record while preserving the native error for
// diagnostics and forensics.
func executeObjectiveWithAdapter(a ObjectiveGameAdapter, o Objective) (ObjectiveResult, error) {
	tx := gameruntime.ExecuteTransaction[Objective, Observation, ObjectiveResult](a, o)
	result := tx.Result
	result.Objective = o
	if tx.InitialObservationErr == nil {
		initial := FailureStateFor(tx.Initial)
		result.Initial = &initial
	}

	if tx.InitialObservationErr != nil {
		retErr := fmt.Errorf("agent: %s: initial observation unavailable: %w", o, tx.InitialObservationErr)
		attachNormalizedFailure(a, &result, gameruntime.FailurePhaseInitialObservation, tx.InitialObservationErr, tx.Final)
		result = finalizeObjectiveResult(o, result, tx.Final, retErr)
		reportObjectiveCaptureFailure(a, o, retErr)
		return result, retErr
	}

	if tx.ValidationErr != nil {
		attachNormalizedFailure(a, &result, gameruntime.FailurePhaseValidation, tx.ValidationErr, tx.Final)
		result = finalizeObjectiveResult(o, result, tx.Final, tx.ValidationErr)
		return result, tx.ValidationErr
	}

	if tx.StartBoundaryErr != nil {
		retErr := fmt.Errorf("agent: %s: objective start invariant: %w", o, tx.StartBoundaryErr)
		phase, native := gameruntime.FailurePhaseStartBoundary, tx.StartBoundaryErr
		if tx.FinalObservationErr != nil {
			phase, native = gameruntime.FailurePhaseFinalObservation, tx.FinalObservationErr
			retErr = errors.Join(retErr, fmt.Errorf("agent: %s: observation after start-boundary failure unavailable: %w", o, tx.FinalObservationErr))
		}
		attachNormalizedFailure(a, &result, phase, native, tx.Final)
		result = finalizeObjectiveResult(o, result, tx.Final, retErr)
		reportObjectiveCaptureFailure(a, o, retErr)
		return result, retErr
	}

	primary := tx.ExecutionErr
	failurePhase := gameruntime.FailurePhaseExecution
	failureNative := tx.ExecutionErr
	if primary == nil && tx.SettleErr != nil {
		failurePhase = gameruntime.FailurePhaseSettle
		primary = fmt.Errorf("agent: %s: postcondition settle: %w",
			o, errors.Join(ErrObjectivePostconditionUnavailable, tx.SettleErr))
		failureNative = primary
	}
	if primary == nil && tx.PostconditionErr != nil {
		failurePhase = gameruntime.FailurePhasePostcondition
		primary = fmt.Errorf("agent: %s: %w", o, tx.PostconditionErr)
		failureNative = primary
	}

	retErr := objectiveBoundaryError(o, primary, tx.FinishBoundaryErr)
	if tx.FinishBoundaryErr != nil {
		// A dirty finish dominates the owned action's semantic failure. Preserve
		// the joined native error for diagnostics but normalize at the finish
		// boundary phase so generic policy cannot treat it as ordinary blockage.
		failurePhase = gameruntime.FailurePhaseFinishBoundary
		failureNative = retErr
		result.Outcome = ""
	}
	if tx.FinalObservationErr != nil {
		// Without a trustworthy final observation, generic policy cannot safely
		// reason from an otherwise recoverable failure.
		failurePhase = gameruntime.FailurePhaseFinalObservation
		failureNative = tx.FinalObservationErr
		obsErr := fmt.Errorf("agent: %s: final observation unavailable: %w", o, tx.FinalObservationErr)
		if retErr == nil {
			retErr = obsErr
		} else {
			retErr = errors.Join(retErr, obsErr)
		}
	}
	if retErr != nil {
		attachNormalizedFailure(a, &result, failurePhase, failureNative, tx.Final)
		// A story/compound objective may own a battle without returning battle
		// evidence directly. If an otherwise unknown failure visibly ended in a
		// Pokemon Center respawn, recover that gameplay outcome here so every
		// objective kind gets the same combat-loss policy.
		promoteDefeatRespawnFailure(&result, tx.Initial, tx.Final)
	}
	result = finalizeObjectiveResult(o, result, tx.Final, retErr)
	// CaptureFailure saves emulator state. A poisoned machine still holds the
	// frame lock inside the stalled step, so a save here deadlocks the worker.
	if retErr != nil && !errors.Is(retErr, gameruntime.ErrMachineUnusable) {
		reportObjectiveCaptureFailure(a, o, retErr)
	}
	return result, retErr
}

func attachNormalizedFailure(a ObjectiveGameAdapter, result *ObjectiveResult, phase gameruntime.FailurePhase, err error, final Observation) {
	if result == nil || err == nil {
		return
	}
	failure := a.NormalizeFailure(phase, err, final)
	if result.Battle != nil && !result.Battle.Won {
		failure.Class = gameruntime.FailureClassBlocked
		failure.Recoverable = true
		failure.Context = nil
		if result.Battle.Encounter != "" {
			failure.Context = []string{result.Battle.Encounter}
		}
		if result.Battle.Result == "lost" {
			failure.Cause = failureCauseCombatDefeat
		} else {
			failure.Cause = failureCauseCombatNotWon
		}
		result.Outcome = OutcomeBlocked
	}
	// Some adapter-owned actions can provide a stronger semantic outcome than
	// the native error type alone (for example a resolved-but-lost gym battle).
	// Keep that evidence when normalization has only an unknown fallback.
	if result.Outcome != "" && result.Outcome != OutcomeCompleted && failure.Class == gameruntime.FailureClassUnknown {
		failure.Class = failureClassForOutcome(result.Outcome)
		failure.Recoverable = actionFor(result.Outcome) == actionReplan
		if failure.Cause == "" || failure.Cause == "unknown_error" {
			failure.Cause = "outcome:" + string(result.Outcome)
		}
	}
	result.Failure = &failure
}

func reportObjectiveCaptureFailure(a ObjectiveGameAdapter, o Objective, err error) {
	if err == nil {
		return
	}
	if captureErr := a.CaptureFailure(o, err); captureErr != nil {
		fmt.Printf("  objective forensics: %v\n", captureErr)
	}
}

// objectiveBoundaryError combines execution and finish-boundary failures
// without losing either typed error identity. If execution itself succeeded, a
// dirty finish is a postcondition failure owned by the objective that just ran
// — never by whatever the planner might choose next.
func objectiveBoundaryError(o Objective, primary, boundary error) error {
	if boundary == nil {
		return primary
	}
	if primary != nil {
		combined := errors.Join(primary, fmt.Errorf("objective left invalid boundary: %w", boundary))
		return fmt.Errorf("agent: %s: %w", o, combined)
	}
	return fmt.Errorf("agent: %s: objective postcondition: %w", o, boundary)
}
