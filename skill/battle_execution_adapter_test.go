package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

const fakeBattleExecutionPhase uint16 = 48

type fakeGen2BattleExecutionDecoder struct{}

func (fakeGen2BattleExecutionDecoder) DecodeBattleExecution(r game.MemoryReader) game.BattleExecutionState {
	phase := game.BattleExecutionNone
	if r.Peek8(fakeBattleExecutionPhase) != 0 {
		phase = game.BattleExecutionMoveMenu
	}
	return game.BattleExecutionState{
		InBattle: true,
		Phase:    phase,
		Learner: game.BattleMoveLearnerState{
			Valid:     true,
			PartySlot: 2,
			Type1:     300,
			Type2:     301,
			Moves:     [4]uint16{1, 200, 251, 0},
		},
		PartyMoves: [][4]uint16{{1, 2, 3, 4}, {5, 6, 7, 8}, {1, 200, 251, 0}},
	}
}

func TestGenericBattleExecutionUsesProfileState(t *testing.T) {
	m := &fakeBattleMenuMachine{}
	m.mem[fakeBattleExecutionPhase] = 1
	decoder := fakeGen2BattleExecutionDecoder{}
	got := decoder.DecodeBattleExecution(m)
	if !got.InBattle || !got.Learner.Valid || got.Learner.PartySlot != 2 {
		t.Fatalf("state = %#v", got)
	}
	if got.Learner.Type1 != 300 || got.Learner.Moves[2] != 251 {
		t.Fatalf("Gen-II-shaped learner was narrowed: %#v", got.Learner)
	}
	if !got.MoveLearned(2, 2, 251) {
		t.Fatal("portable move-learning verification did not observe party move")
	}
}
