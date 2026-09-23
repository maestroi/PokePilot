package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

func TestRequireBattleWinCarriesStructuredLoss(t *testing.T) {
	if err := RequireBattleWin("story:boss", state.ResultWon); err != nil {
		t.Fatalf("win = %v, want nil", err)
	}

	err := RequireBattleWin("story:boss", state.ResultLost)
	var required *RequiredBattleError
	if !errors.As(err, &required) {
		t.Fatalf("loss = %T %v, want *RequiredBattleError", err, err)
	}
	if required.Outcome.Encounter != "story:boss" || required.Outcome.Result != state.ResultLost {
		t.Fatalf("outcome = %+v", required.Outcome)
	}
	if !errors.Is(err, ErrTrainerBlackedOut) {
		t.Fatal("required battle loss must preserve legacy trainer-blackout compatibility")
	}
}

func TestRequireBattleWinKeepsDrawDistinctFromBlackout(t *testing.T) {
	err := RequireBattleWin("story:boss", state.ResultDraw)
	var required *RequiredBattleError
	if !errors.As(err, &required) || required.Outcome.Result != state.ResultDraw {
		t.Fatalf("draw = %T %v, want structured ResultDraw", err, err)
	}
	if errors.Is(err, ErrTrainerBlackedOut) {
		t.Fatal("draw must not claim the party blacked out")
	}
}
