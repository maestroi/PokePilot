package agent

import (
	"errors"
	"fmt"

	gameruntime "github.com/maestroi/pokepilot/game"
)

// ObjectiveGameAdapter is the seam between the game-agnostic objective
// transaction runtime and one concrete game implementation. The runtime owns
// lifecycle and normalized ObjectiveResult policy; the adapter owns game facts,
// state inspection, controller mechanics, and positive postcondition evidence.
//
// Objective/Observation/ObjectiveResult are still the existing agent types in
// this first migration step. Moving their remaining Red-shaped identifiers to a
// portable semantic vocabulary is intentionally a later change; the important
// invariant established here is that transaction ordering no longer depends on
// an emulator or Pokémon Red implementation.
type ObjectiveGameAdapter interface {
	gameruntime.Adapter[Objective, Observation, ObjectiveResult]

	// CaptureFailure persists game-specific forensic evidence. It is diagnostic
	// only: a capture error must never replace the gameplay/runtime error.
	CaptureFailure(Objective, error) error
}

// executeObjectiveWithAdapter is the agent-facing transaction boundary. The
// portable game package owns lifecycle ordering; this layer maps that evidence
// into PokePilot's stable ObjectiveResult/Outcome/error contract.
func executeObjectiveWithAdapter(a ObjectiveGameAdapter, o Objective) (ObjectiveResult, error) {
	tx := gameruntime.ExecuteTransaction[Objective, Observation, ObjectiveResult](a, o)
	result := tx.Result
	result.Objective = o

	if tx.ValidationErr != nil {
		result = finalizeObjectiveResult(o, result, tx.Final, tx.ValidationErr)
		return result, tx.ValidationErr
	}

	if tx.StartBoundaryErr != nil {
		retErr := fmt.Errorf("agent: %s: objective start invariant: %w", o, tx.StartBoundaryErr)
		result = finalizeObjectiveResult(o, result, tx.Final, retErr)
		reportObjectiveCaptureFailure(a, o, retErr)
		return result, retErr
	}

	primary := tx.ExecutionErr
	if primary == nil && tx.PostconditionErr != nil {
		primary = fmt.Errorf("agent: %s: %w", o, tx.PostconditionErr)
	}

	retErr := objectiveBoundaryError(o, primary, tx.FinishBoundaryErr)
	if tx.FinishBoundaryErr != nil {
		// A dirty finish is stronger than any semantic blocked result produced
		// by the owned action. The planner must never receive an unsettled game
		// as ordinary gameplay blockage.
		result.Outcome = ""
	}
	result = finalizeObjectiveResult(o, result, tx.Final, retErr)
	if retErr != nil {
		reportObjectiveCaptureFailure(a, o, retErr)
	}
	return result, retErr
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
