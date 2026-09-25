package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestNaturalMoveLearnerUsesProfileLearnerNotActiveBattleMon(t *testing.T) {
	exec := game.BattleExecutionState{
		Learner: game.BattleMoveLearnerState{
			Valid:     true,
			PartySlot: 1,
			Type1:     0x16,
			Type2:     0x03,
			Moves:     [4]uint16{33, 45, 73, 22},
		},
	}
	mon, slot, ok := naturalMoveLearner(exec)
	if !ok {
		t.Fatal("naturalMoveLearner reported no learner")
	}
	if slot != 1 {
		t.Fatalf("slot=%d want 1", slot)
	}
	if mon.Moves != [4]uint16{33, 45, 73, 22} {
		t.Fatalf("moves=%v want profile-projected learner moves", mon.Moves)
	}
	if mon.Type1 != 0x16 || mon.Type2 != 0x03 {
		t.Fatalf("types=(%#02x,%#02x) want learner types", mon.Type1, mon.Type2)
	}
}

func TestNaturalMoveLearnerRejectsInvalidProfileLearner(t *testing.T) {
	exec := game.BattleExecutionState{
		Learner: game.BattleMoveLearnerState{Valid: false, PartySlot: 4},
	}
	if _, slot, ok := naturalMoveLearner(exec); ok || slot != 4 {
		t.Fatalf("slot=%d ok=%v want invalid slot 4", slot, ok)
	}
}

func TestNaturalMoveDecisionRejectsIdsOutsideCurrentGen1Strategy(t *testing.T) {
	exec := game.BattleExecutionState{
		Learner: game.BattleMoveLearnerState{
			Valid: true, PartySlot: 0, Type1: 300, Moves: [4]uint16{1, 2, 3, 4},
		},
	}
	if _, _, ok := naturalMoveDecisionForLearner(exec, nil, 251, nil); ok {
		t.Fatal("Gen-I move policy unexpectedly accepted a Gen-II type id; execution state must stay wide without silently narrowing")
	}
}
