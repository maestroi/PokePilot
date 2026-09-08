// Package game contains game-agnostic runtime contracts. It must not import
// a concrete game implementation, emulator, ROM decoder, or skill package.
package game

// Adapter is the game-owned half of one objective transaction. The generic
// runtime owns lifecycle and ordering; an adapter owns how its game observes,
// validates, executes, stabilizes, and proves a semantic postcondition.
//
// The type parameters deliberately let the current Red-facing agent migrate
// incrementally without forcing its Objective/Observation/ObjectiveResult
// types into this package in the same change. A later multi-game vocabulary
// migration can replace those concrete types without changing transaction
// ownership.
type Adapter[Objective any, Observation any, Result any] interface {
	// Observe returns a stable semantic observation. It must not expose raw
	// addresses or require the generic runtime to understand game memory.
	Observe() Observation

	// Validate rejects malformed objective arguments or unsatisfied adapter-
	// visible prerequisites before gameplay input is sent. The initial semantic
	// observation is supplied so a future game need not reach behind the seam to
	// inspect its own state during validation.
	Validate(Objective, Observation) error

	// NormalizeBoundary returns the game to a safe objective boundary using
	// only semantically reversible cleanup. It must not answer gameplay/story
	// choices or spend resources.
	NormalizeBoundary() error

	// ExecuteOwned performs the game-specific action owned by this objective.
	// It may return a structured non-completion result together with a typed
	// diagnostic error.
	ExecuteOwned(Objective) (Result, error)

	// WithinObjectiveBudget executes fn under the game's bounded-controller
	// watchdog. The adapter chooses how the budget is enforced (frames, ticks,
	// etc.); the generic runtime only requires that one objective cannot run
	// forever.
	WithinObjectiveBudget(Objective, func() error) error

	// SettlePostcondition may passively wait for a successful action's state to
	// become readable before the finish boundary is normalized. It must not
	// send gameplay input. This is where an emulator-backed adapter may step
	// frames through a warp/fade without making a decision.
	SettlePostcondition(Objective)

	// VerifyPostcondition checks the positive semantic success contract against
	// the final settled observation. A nil error means the objective's claimed
	// success is true in the world now.
	VerifyPostcondition(Objective, Observation, Result) error
}

// Transaction is the raw lifecycle evidence for one objective. It deliberately
// keeps validation, start-boundary, execution, finish-boundary, and semantic
// postcondition failures separate. The caller can preserve its own typed error
// identities and normalized Outcome vocabulary without teaching this package
// game-specific error classes.
type Transaction[Result any, Observation any] struct {
	Result  Result
	Initial Observation
	Final   Observation

	ValidationErr     error
	StartBoundaryErr  error
	ExecutionErr      error
	FinishBoundaryErr error
	PostconditionErr  error
}

// ExecuteTransaction runs one objective through the portable lifecycle:
//
//	observe -> validate -> normalize start -> bounded owned execution
//	-> passive settle -> normalize finish -> observe -> verify postcondition
//
// Finish normalization still runs after an execution failure so the objective
// owns the state it leaves behind. Postcondition verification runs only after a
// successful action and a clean finish boundary.
func ExecuteTransaction[Objective any, Observation any, Result any](
	a Adapter[Objective, Observation, Result],
	o Objective,
) Transaction[Result, Observation] {
	var tx Transaction[Result, Observation]
	tx.Initial = a.Observe()

	if err := a.Validate(o, tx.Initial); err != nil {
		tx.ValidationErr = err
		tx.Final = tx.Initial
		return tx
	}

	if err := a.NormalizeBoundary(); err != nil {
		tx.StartBoundaryErr = err
		tx.Final = a.Observe()
		return tx
	}

	tx.ExecutionErr = a.WithinObjectiveBudget(o, func() error {
		var err error
		tx.Result, err = a.ExecuteOwned(o)
		return err
	})

	if tx.ExecutionErr == nil {
		a.SettlePostcondition(o)
	}

	tx.FinishBoundaryErr = a.NormalizeBoundary()
	tx.Final = a.Observe()

	if tx.ExecutionErr == nil && tx.FinishBoundaryErr == nil {
		tx.PostconditionErr = a.VerifyPostcondition(o, tx.Final, tx.Result)
	}

	return tx
}
