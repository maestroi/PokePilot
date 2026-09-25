package profile

import "github.com/maestroi/pokepilot/game"

func (p *Profile) DecodeBattleResources(r game.MemoryReader) game.BattleResourcesState {
	return p.engine.DecodeBattleResources(r)
}
