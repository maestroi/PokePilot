package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/red/state"
)

// RequiredBattleOutcome is the semantic result of a battle that must be won
// before the owning skill can complete. Encounter is an adapter-owned stable
// identity; generic recovery treats it as opaque evidence.
type RequiredBattleOutcome struct {
	Encounter string
	Result    state.BattleResult
	Trainer   bool
}

// RequiredBattleError reports a resolved required battle whose outcome did not
// satisfy the required win postcondition. It keeps the concrete battle result
// and encounter identity structured so callers never need to parse error prose.
//
// ResultLost unwraps to the appropriate legacy blackout sentinel for
// compatibility with older direct callers. The agent consumes
// RequiredBattleError directly and normalizes every actual loss to the generic
// combat_defeat outcome.
type RequiredBattleError struct {
	Outcome RequiredBattleOutcome
}

func (e *RequiredBattleError) Error() string {
	if e == nil {
		return "skill: required battle did not complete"
	}
	return fmt.Sprintf("skill: required battle %q ended with outcome %d, want won", e.Outcome.Encounter, e.Outcome.Result)
}

func (e *RequiredBattleError) Unwrap() error {
	if e == nil || e.Outcome.Result != state.ResultLost {
		return nil
	}
	if e.Outcome.Trainer {
		return ErrTrainerBlackedOut
	}
	return ErrBlackedOut
}

// RequireBattleWin turns a resolved mandatory non-trainer battle into the
// shared structured outcome contract. A win is success; every other result
// remains data on the typed error so the objective adapter can decide how to
// recover.
func RequireBattleWin(encounter string, result state.BattleResult) error {
	return requireBattleWin(encounter, result, false)
}

// RequireTrainerBattleWin is the trainer counterpart to RequireBattleWin. The
// structured outcome is identical for the agent; Trainer exists only so legacy
// errors.Is callers keep seeing ErrTrainerBlackedOut rather than the broader
// ErrBlackedOut sentinel.
func RequireTrainerBattleWin(encounter string, result state.BattleResult) error {
	return requireBattleWin(encounter, result, true)
}

func requireBattleWin(encounter string, result state.BattleResult, trainer bool) error {
	if result == state.ResultWon {
		return nil
	}
	return &RequiredBattleError{Outcome: RequiredBattleOutcome{
		Encounter: encounter,
		Result:    result,
		Trainer:   trainer,
	}}
}
