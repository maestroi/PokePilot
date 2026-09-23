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
}

// RequiredBattleError reports a resolved required battle whose outcome did not
// satisfy the required win postcondition. It keeps the concrete battle result
// and encounter identity structured so callers never need to parse error prose.
//
// ResultLost unwraps to ErrTrainerBlackedOut for one-release compatibility with
// existing Gen I recovery callers. The agent consumes RequiredBattleError
// directly and normalizes it to the generic combat_defeat outcome.
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
	if e != nil && e.Outcome.Result == state.ResultLost {
		return ErrTrainerBlackedOut
	}
	return nil
}

// RequireBattleWin turns a resolved mandatory battle into the shared structured
// outcome contract. A win is success; every other result remains data on the
// typed error so the objective adapter can decide how to recover.
func RequireBattleWin(encounter string, result state.BattleResult) error {
	if result == state.ResultWon {
		return nil
	}
	return &RequiredBattleError{Outcome: RequiredBattleOutcome{
		Encounter: encounter,
		Result:    result,
	}}
}
