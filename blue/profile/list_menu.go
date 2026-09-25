package profile

import "github.com/maestroi/pokepilot/game"

func (p *Profile) DecodeListMenu(r game.MemoryReader) game.ListMenuState {
	return p.engine.DecodeListMenu(r)
}
