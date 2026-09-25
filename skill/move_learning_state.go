package skill

import "github.com/maestroi/pokepilot/game"

// naturalMoveLearner returns the party member the active profile says is
// currently processing a natural move-learning episode. It is not necessarily
// the active battle Pokémon: experience can be awarded to a different party
// member after a KO.
func naturalMoveLearner(exec game.BattleExecutionState) (game.BattleMoveLearnerState, int, bool) {
	learner := exec.Learner
	return learner, learner.PartySlot, learner.Valid
}

// naturalMoveDecisionForLearner bridges the portable execution projection to
// the existing Gen-I move-set strategy. Execution ids remain uint16 so the
// profile boundary is Gen-II-safe; this strategy adapter refuses ids it cannot
// represent rather than silently truncating them.
func naturalMoveDecisionForLearner(exec game.BattleExecutionState, romData []byte, offered uint16, blocked map[uint8]bool) (MoveLearningDecision, int, bool) {
	learner, slot, ok := naturalMoveLearner(exec)
	if !ok {
		return MoveLearningDecision{}, slot, false
	}
	if offered == 0 || offered > 0xff || learner.Type1 > 0xff || learner.Type2 > 0xff {
		return MoveLearningDecision{}, slot, false
	}
	var moves [4]uint8
	for i, move := range learner.Moves {
		if move > 0xff {
			return MoveLearningDecision{}, slot, false
		}
		moves[i] = uint8(move)
	}
	return decideNaturalMove(
		romData,
		uint8(learner.Type1),
		uint8(learner.Type2),
		moves,
		uint8(offered),
		blocked,
	), slot, true
}
