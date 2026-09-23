package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestRequireBattleWinCarriesStructuredLoss(t *testing.T) {
	if err := RequireBattleWin("static:snorlax", state.ResultWon); err != nil {
		t.Fatalf("win = %v, want nil", err)
	}

	err := RequireBattleWin("static:snorlax", state.ResultLost)
	var required *RequiredBattleError
	if !errors.As(err, &required) {
		t.Fatalf("loss = %T %v, want *RequiredBattleError", err, err)
	}
	if required.Outcome.Encounter != "static:snorlax" || required.Outcome.Result != state.ResultLost || required.Outcome.Trainer {
		t.Fatalf("outcome = %+v", required.Outcome)
	}
	if !errors.Is(err, ErrBlackedOut) {
		t.Fatal("required non-trainer loss must preserve generic blackout compatibility")
	}
	if errors.Is(err, ErrTrainerBlackedOut) {
		t.Fatal("required non-trainer loss must not claim a trainer blackout")
	}
}

func TestRequireTrainerBattleWinCarriesTrainerCompatibility(t *testing.T) {
	err := RequireTrainerBattleWin("gym:brock", state.ResultLost)
	var required *RequiredBattleError
	if !errors.As(err, &required) || !required.Outcome.Trainer {
		t.Fatalf("trainer loss = %T %v, want structured trainer outcome", err, err)
	}
	if !errors.Is(err, ErrTrainerBlackedOut) || !errors.Is(err, ErrBlackedOut) {
		t.Fatal("trainer loss must unwrap to trainer and generic blackout compatibility")
	}
}

func TestRequireBattleWinKeepsDrawDistinctFromBlackout(t *testing.T) {
	err := RequireTrainerBattleWin("story:boss", state.ResultDraw)
	var required *RequiredBattleError
	if !errors.As(err, &required) || required.Outcome.Result != state.ResultDraw {
		t.Fatalf("draw = %T %v, want structured ResultDraw", err, err)
	}
	if errors.Is(err, ErrTrainerBlackedOut) {
		t.Fatal("draw must not claim the party blacked out")
	}
}
