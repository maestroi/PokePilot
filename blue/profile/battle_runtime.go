package profile

import "github.com/maestroi/pokepilot/game"

func (p *Profile) DecodeBattleRuntime(r game.MemoryReader) game.BattleRuntimeState {
	return p.engine.DecodeBattleRuntime(r)
}
