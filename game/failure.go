// Package game contains game-agnostic runtime contracts. It must not import
// a concrete game implementation, emulator, ROM decoder, or skill package.
package game

// FailurePhase identifies which transaction phase produced the failure. The
// values are part of the durable runtime/forensics contract and deliberately
// describe lifecycle semantics rather than one game's implementation details.
type FailurePhase string

const (
	FailurePhaseInitialObservation FailurePhase = "initial_observation"
	FailurePhaseValidation         FailurePhase = "validation"
	FailurePhaseStartBoundary      FailurePhase = "start_boundary"
	FailurePhaseExecution          FailurePhase = "execution"
	FailurePhaseSettle             FailurePhase = "settle"
	FailurePhaseFinishBoundary     FailurePhase = "finish_boundary"
	FailurePhaseFinalObservation   FailurePhase = "final_observation"
	FailurePhasePostcondition      FailurePhase = "postcondition"
)

// FailureClass is the portable policy class for one failed transaction. A game
// adapter maps its native typed errors to this vocabulary before generic run
// policy sees the failure.
type FailureClass string

const (
	FailureClassBlocked                  FailureClass = "blocked"
	FailureClassChoiceRequired           FailureClass = "choice_required"
	FailureClassStabilizationFailed      FailureClass = "stabilization_failed"
	FailureClassOwnershipFailure         FailureClass = "ownership_failure"
	FailureClassControllerUncertain      FailureClass = "controller_uncertain"
	FailureClassPostconditionFailed      FailureClass = "postcondition_failed"
	FailureClassPostconditionUnavailable FailureClass = "postcondition_unavailable"
	FailureClassUnknown                  FailureClass = "unknown_failure"
)

// Failure is the normalized game-adapter boundary record. Cause is a stable
// semantic identifier (never error prose). Context contains compact structured
// evidence needed to distinguish otherwise identical causes. The native typed
// error remains outside this record for adapter-owned diagnostics/forensics.
type Failure struct {
	Phase       FailurePhase `json:"phase"`
	Class       FailureClass `json:"class"`
	Cause       string       `json:"cause"`
	Recoverable bool         `json:"recoverable"`
	Context       []string       `json:"context,omitempty"`
	Prerequisites []Prerequisite `json:"prerequisites,omitempty"`
}

func (f Failure) Empty() bool {
	return f.Phase == "" && f.Class == "" && f.Cause == "" && !f.Recoverable && len(f.Context) == 0 && len(f.Prerequisites) == 0
}
