package skill

import "github.com/maestroi/pokepilot/game"

// MovePolicy chooses which move slot to use from portable live battle state.
// Returning a slot that is not in BattleState.Usable is a programming error
// and Battle rejects it before pressing anything.
type MovePolicy func(game.BattleState) int

// FirstUsableMove is the deterministic fallback policy.
func FirstUsableMove(b game.BattleState) int {
	usable := b.Usable()
	if len(usable) == 0 {
		return -1
	}
	return usable[0]
}
