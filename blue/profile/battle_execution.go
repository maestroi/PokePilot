package profile

import "github.com/maestroi/pokepilot/game"

func (p *Profile) DecodeBattleExecution(r game.MemoryReader) game.BattleExecutionState {
	return p.engine.DecodeBattleExecution(r)
}
