package profile

import "github.com/maestroi/pokepilot/game"

func (p *Profile) DecodeBattleState(r game.MemoryReader) (game.BattleState, bool) {
	return p.engine.DecodeBattleState(r)
}

func (p *Profile) DecodeBattleResult(r game.MemoryReader) game.BattleResult {
	return p.engine.DecodeBattleResult(r)
}
